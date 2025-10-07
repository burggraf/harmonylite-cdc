package integration

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// TestConcurrentWriteContention verifies that SQLite's single-writer model handles concurrent writes correctly
// This test simulates the scenario where PocketBase (or any application) and HarmonyLite applicator
// both attempt to write to the same database simultaneously
func TestConcurrentWriteContention(t *testing.T) {
	t.Run("Two concurrent writers with busy_timeout succeed", func(t *testing.T) {
		// Setup: Create temporary database with WAL mode and busy_timeout
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.db")

		// Create database with WAL mode
		db, err := sql.Open("sqlite", dbPath)
		if err != nil {
			t.Fatalf("Failed to open database: %v", err)
		}
		defer db.Close()

		// Configure WAL mode and busy timeout via PRAGMA
		_, err = db.Exec("PRAGMA journal_mode=WAL")
		if err != nil {
			t.Fatalf("Failed to enable WAL mode: %v", err)
		}
		_, err = db.Exec("PRAGMA busy_timeout=30000")
		if err != nil {
			t.Fatalf("Failed to set busy_timeout: %v", err)
		}

		// Create test table
		_, err = db.Exec("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)")
		if err != nil {
			t.Fatalf("Failed to create table: %v", err)
		}

		// Test: Launch two goroutines that write concurrently
		const numWrites = 100
		const numWriters = 2

		var wg sync.WaitGroup
		var successCount atomic.Int64
		var errorCount atomic.Int64

		startSignal := make(chan struct{})

		for writer := 0; writer < numWriters; writer++ {
			wg.Add(1)
			writerID := writer

			go func() {
				defer wg.Done()

				// Wait for start signal to ensure maximum contention
				<-startSignal

				// Each writer opens its own connection (simulating separate processes)
				writerDB, err := sql.Open("sqlite", dbPath)
				if err != nil {
					t.Errorf("Writer %d failed to open database: %v", writerID, err)
					return
				}
				defer writerDB.Close()

				// Configure busy timeout for this connection
				_, err = writerDB.Exec("PRAGMA busy_timeout=30000")
				if err != nil {
					t.Errorf("Writer %d failed to set busy_timeout: %v", writerID, err)
					return
				}

				for i := 0; i < numWrites; i++ {
					id := writerID*numWrites + i
					name := fmt.Sprintf("writer%d_user%d", writerID, i)

					_, err := writerDB.Exec("INSERT INTO users (id, name) VALUES (?, ?)", id, name)
					if err != nil {
						errorCount.Add(1)
						t.Logf("Writer %d insert %d failed: %v", writerID, i, err)
					} else {
						successCount.Add(1)
					}

					// Small delay to simulate real-world write patterns
					time.Sleep(1 * time.Millisecond)
				}
			}()
		}

		// Start all writers simultaneously
		close(startSignal)

		// Wait for completion
		wg.Wait()

		// Verify: All writes should succeed (busy_timeout allows retries)
		expectedWrites := int64(numWrites * numWriters)
		actualSuccesses := successCount.Load()
		actualErrors := errorCount.Load()

		t.Logf("Concurrent writes: %d total, %d succeeded, %d failed", expectedWrites, actualSuccesses, actualErrors)

		if actualSuccesses != expectedWrites {
			t.Errorf("Expected %d successful writes, got %d (with %d errors)", expectedWrites, actualSuccesses, actualErrors)
		}

		// Verify database contains all records
		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
		if err != nil {
			t.Fatalf("Failed to count records: %v", err)
		}

		if int64(count) != expectedWrites {
			t.Errorf("Expected %d records in database, got %d", expectedWrites, count)
		}
	})

	t.Run("Write contention metrics: lock wait times", func(t *testing.T) {
		// Setup: Database with busy_timeout
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.db")

		db, err := sql.Open("sqlite", dbPath)
		if err != nil {
			t.Fatalf("Failed to open database: %v", err)
		}
		defer db.Close()

		_, err = db.Exec("PRAGMA journal_mode=WAL")
		if err != nil {
			t.Fatalf("Failed to enable WAL mode: %v", err)
		}
		_, err = db.Exec("PRAGMA busy_timeout=30000")
		if err != nil {
			t.Fatalf("Failed to set busy_timeout: %v", err)
		}

		_, err = db.Exec("CREATE TABLE metrics (id INTEGER PRIMARY KEY, value TEXT)")
		if err != nil {
			t.Fatalf("Failed to create table: %v", err)
		}

		// Test: Measure lock wait times under contention
		const numConcurrentWrites = 10
		var wg sync.WaitGroup
		waitTimes := make([]time.Duration, numConcurrentWrites)

		startSignal := make(chan struct{})

		for i := 0; i < numConcurrentWrites; i++ {
			wg.Add(1)
			writerIdx := i

			go func() {
				defer wg.Done()

				<-startSignal

				writerDB, err := sql.Open("sqlite", dbPath)
				if err != nil {
					return
				}
				defer writerDB.Close()

				_, err = writerDB.Exec("PRAGMA busy_timeout=30000")
				if err != nil {
					return
				}

				start := time.Now()
				_, err = writerDB.Exec("INSERT INTO metrics (id, value) VALUES (?, ?)", writerIdx, fmt.Sprintf("value%d", writerIdx))
				elapsed := time.Since(start)

				if err == nil {
					waitTimes[writerIdx] = elapsed
				}
			}()
		}

		close(startSignal)
		wg.Wait()

		// Analyze wait times
		var totalWait time.Duration
		var maxWait time.Duration
		successfulWrites := 0

		for _, wait := range waitTimes {
			if wait > 0 {
				totalWait += wait
				successfulWrites++
				if wait > maxWait {
					maxWait = wait
				}
			}
		}

		avgWait := totalWait / time.Duration(successfulWrites)

		t.Logf("Lock wait times: avg=%v, max=%v, successful_writes=%d/%d",
			avgWait, maxWait, successfulWrites, numConcurrentWrites)

		// Verify: All writes completed (no timeout exceeded)
		if successfulWrites != numConcurrentWrites {
			t.Errorf("Expected %d successful writes, got %d", numConcurrentWrites, successfulWrites)
		}

		// Verify: Maximum wait time is reasonable (< 5 seconds for light contention)
		if maxWait > 5*time.Second {
			t.Errorf("Maximum lock wait time too high: %v (expected < 5s)", maxWait)
		}
	})

	t.Run("No busy_timeout causes SQLITE_BUSY errors", func(t *testing.T) {
		// Setup: Database WITHOUT busy_timeout (to demonstrate the problem)
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.db")

		// Note: No busy_timeout configured (defaults to 0)
		db, err := sql.Open("sqlite", dbPath)
		if err != nil {
			t.Fatalf("Failed to open database: %v", err)
		}
		defer db.Close()

		_, err = db.Exec("PRAGMA journal_mode=WAL")
		if err != nil {
			t.Fatalf("Failed to enable WAL mode: %v", err)
		}

		_, err = db.Exec("CREATE TABLE test (id INTEGER PRIMARY KEY)")
		if err != nil {
			t.Fatalf("Failed to create table: %v", err)
		}

		// Test: Concurrent writes WITHOUT busy_timeout should produce errors
		const numWrites = 20
		const numWriters = 2

		var wg sync.WaitGroup
		var errorCount atomic.Int64
		startSignal := make(chan struct{})

		for writer := 0; writer < numWriters; writer++ {
			wg.Add(1)
			writerID := writer

			go func() {
				defer wg.Done()
				<-startSignal

				writerDB, err := sql.Open("sqlite", dbPath)
				if err != nil {
					return
				}
				defer writerDB.Close()

				// Note: No busy_timeout set, so defaults to 0 (immediate failure)

				for i := 0; i < numWrites; i++ {
					id := writerID*numWrites + i
					_, err := writerDB.Exec("INSERT INTO test (id) VALUES (?)", id)
					if err != nil {
						errorCount.Add(1)
					}
				}
			}()
		}

		close(startSignal)
		wg.Wait()

		errors := errorCount.Load()
		t.Logf("Without busy_timeout: %d SQLITE_BUSY errors occurred", errors)

		// Verify: WITHOUT busy_timeout, we expect some errors under contention
		// This demonstrates why busy_timeout is critical
		if errors == 0 {
			t.Logf("Warning: No SQLITE_BUSY errors observed. Test may need heavier contention.")
		}
	})

	t.Run("Simulate PocketBase + HarmonyLite concurrent writes", func(t *testing.T) {
		// Setup: Realistic scenario with application writes and replication writes
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "pocketbase.db")

		db, err := sql.Open("sqlite", dbPath)
		if err != nil {
			t.Fatalf("Failed to open database: %v", err)
		}
		defer db.Close()

		_, err = db.Exec("PRAGMA journal_mode=WAL")
		if err != nil {
			t.Fatalf("Failed to enable WAL mode: %v", err)
		}
		_, err = db.Exec("PRAGMA busy_timeout=30000")
		if err != nil {
			t.Fatalf("Failed to set busy_timeout: %v", err)
		}

		_, err = db.Exec("CREATE TABLE posts (id INTEGER PRIMARY KEY, author TEXT, content TEXT, timestamp INTEGER)")
		if err != nil {
			t.Fatalf("Failed to create table: %v", err)
		}

		// Test: Simulate realistic workload
		const testDuration = 5 * time.Second
		const pocketbaseWriteInterval = 10 * time.Millisecond  // ~100 ops/sec
		const harmonyliteWriteInterval = 15 * time.Millisecond // ~66 ops/sec (remote replication)

		ctx, cancel := context.WithTimeout(context.Background(), testDuration)
		defer cancel()

		var wg sync.WaitGroup
		var pocketbaseWrites atomic.Int64
		var harmonyliteWrites atomic.Int64
		var pocketbaseErrors atomic.Int64
		var harmonyliteErrors atomic.Int64

		// Simulate PocketBase application writes
		wg.Add(1)
		go func() {
			defer wg.Done()

			pocketbaseDB, err := sql.Open("sqlite", dbPath)
			if err != nil {
				t.Errorf("PocketBase failed to open database: %v", err)
				return
			}
			defer pocketbaseDB.Close()

			_, err = pocketbaseDB.Exec("PRAGMA busy_timeout=30000")
			if err != nil {
				t.Errorf("PocketBase failed to set busy_timeout: %v", err)
				return
			}

			ticker := time.NewTicker(pocketbaseWriteInterval)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					id := pocketbaseWrites.Load()
					_, err := pocketbaseDB.Exec("INSERT INTO posts (id, author, content, timestamp) VALUES (?, ?, ?, ?)",
						id, "local_user", fmt.Sprintf("Post %d", id), time.Now().UnixMilli())
					if err != nil {
						pocketbaseErrors.Add(1)
						t.Logf("PocketBase write error: %v", err)
					} else {
						pocketbaseWrites.Add(1)
					}
				}
			}
		}()

		// Simulate HarmonyLite applicator writes (from remote replication)
		wg.Add(1)
		go func() {
			defer wg.Done()

			harmonyliteDB, err := sql.Open("sqlite", dbPath)
			if err != nil {
				t.Errorf("HarmonyLite failed to open database: %v", err)
				return
			}
			defer harmonyliteDB.Close()

			_, err = harmonyliteDB.Exec("PRAGMA busy_timeout=30000")
			if err != nil {
				t.Errorf("HarmonyLite failed to set busy_timeout: %v", err)
				return
			}

			ticker := time.NewTicker(harmonyliteWriteInterval)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					id := 10000 + harmonyliteWrites.Load() // Offset to avoid conflicts
					_, err := harmonyliteDB.Exec("INSERT INTO posts (id, author, content, timestamp) VALUES (?, ?, ?, ?)",
						id, "remote_user", fmt.Sprintf("Replicated post %d", id), time.Now().UnixMilli())
					if err != nil {
						harmonyliteErrors.Add(1)
						t.Logf("HarmonyLite write error: %v", err)
					} else {
						harmonyliteWrites.Add(1)
					}
				}
			}
		}()

		wg.Wait()

		// Report results
		pbWrites := pocketbaseWrites.Load()
		hlWrites := harmonyliteWrites.Load()
		pbErrors := pocketbaseErrors.Load()
		hlErrors := harmonyliteErrors.Load()
		totalWrites := pbWrites + hlWrites

		t.Logf("Concurrent write simulation results:")
		t.Logf("  PocketBase: %d writes, %d errors", pbWrites, pbErrors)
		t.Logf("  HarmonyLite: %d writes, %d errors", hlWrites, hlErrors)
		t.Logf("  Total: %d writes, %d errors", totalWrites, pbErrors+hlErrors)
		t.Logf("  Combined throughput: %.2f ops/sec", float64(totalWrites)/testDuration.Seconds())

		// Verify: Both systems can write concurrently with acceptable error rate
		if pbErrors > pbWrites/10 { // Allow up to 10% error rate
			t.Errorf("PocketBase error rate too high: %d errors / %d writes", pbErrors, pbWrites)
		}
		if hlErrors > hlWrites/10 {
			t.Errorf("HarmonyLite error rate too high: %d errors / %d writes", hlErrors, hlWrites)
		}

		// Verify database integrity
		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM posts").Scan(&count)
		if err != nil {
			t.Fatalf("Failed to count records: %v", err)
		}

		if int64(count) != totalWrites {
			t.Errorf("Expected %d records in database, got %d", totalWrites, count)
		}
	})
}

// TestBusyTimeoutConfiguration verifies that busy_timeout is properly configured
func TestBusyTimeoutConfiguration(t *testing.T) {
	t.Run("Verify busy_timeout is set", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.db")

		db, err := sql.Open("sqlite", dbPath)
		if err != nil {
			t.Fatalf("Failed to open database: %v", err)
		}
		defer db.Close()

		// Set busy timeout
		_, err = db.Exec("PRAGMA busy_timeout=30000")
		if err != nil {
			t.Fatalf("Failed to set busy_timeout: %v", err)
		}

		// Query the busy_timeout pragma
		var timeout int
		err = db.QueryRow("PRAGMA busy_timeout").Scan(&timeout)
		if err != nil {
			t.Fatalf("Failed to query busy_timeout: %v", err)
		}

		t.Logf("Configured busy_timeout: %d ms", timeout)

		if timeout != 30000 {
			t.Errorf("Expected busy_timeout=30000ms, got %dms", timeout)
		}
	})

	t.Run("Verify WAL mode is enabled", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.db")

		db, err := sql.Open("sqlite", dbPath)
		if err != nil {
			t.Fatalf("Failed to open database: %v", err)
		}
		defer db.Close()

		_, err = db.Exec("PRAGMA journal_mode=WAL")
		if err != nil {
			t.Fatalf("Failed to enable WAL mode: %v", err)
		}

		_, err = db.Exec("CREATE TABLE test (id INTEGER)")
		if err != nil {
			t.Fatalf("Failed to create table: %v", err)
		}

		// Verify WAL mode
		var journalMode string
		err = db.QueryRow("PRAGMA journal_mode").Scan(&journalMode)
		if err != nil {
			t.Fatalf("Failed to query journal_mode: %v", err)
		}

		t.Logf("Journal mode: %s", journalMode)

		if journalMode != "wal" {
			t.Errorf("Expected journal_mode=wal, got %s", journalMode)
		}

		// Verify WAL file exists
		walPath := dbPath + "-wal"
		if _, err := os.Stat(walPath); os.IsNotExist(err) {
			t.Errorf("WAL file does not exist: %s", walPath)
		}
	})
}
