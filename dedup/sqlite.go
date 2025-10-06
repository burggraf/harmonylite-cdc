package dedup

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite" // SQLite driver
)

// SQLiteStore implements Store using SQLite
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore creates a new SQLite-based deduplication store
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	// Open database
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open dedup database: %w", err)
	}

	// Enable WAL mode for consistency
	if _, err := db.Exec("PRAGMA journal_mode = WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable WAL mode: %w", err)
	}

	// Create dedup table if not exists
	createTableSQL := `
		CREATE TABLE IF NOT EXISTS dedup (
			msg_id TEXT PRIMARY KEY,
			seen_at INTEGER NOT NULL
		)
	`
	if _, err := db.Exec(createTableSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create dedup table: %w", err)
	}

	// Create index on seen_at for efficient cleanup
	createIndexSQL := `
		CREATE INDEX IF NOT EXISTS idx_dedup_seen_at ON dedup(seen_at)
	`
	if _, err := db.Exec(createIndexSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create index: %w", err)
	}

	return &SQLiteStore{db: db}, nil
}

// Has checks if OpID exists in store
func (s *SQLiteStore) Has(opID string) (bool, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM dedup WHERE msg_id = ?", opID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check OpID: %w", err)
	}
	return count > 0, nil
}

// Add stores OpID with timestamp (idempotent with INSERT OR IGNORE)
func (s *SQLiteStore) Add(opID string, timestamp time.Time) error {
	unixTime := timestamp.Unix()
	_, err := s.db.Exec("INSERT OR IGNORE INTO dedup (msg_id, seen_at) VALUES (?, ?)", opID, unixTime)
	if err != nil {
		return fmt.Errorf("failed to add OpID: %w", err)
	}
	return nil
}

// Cleanup removes OpIDs older than retention window
func (s *SQLiteStore) Cleanup(retentionWindow time.Duration) (int, error) {
	// Calculate cutoff time
	cutoffTime := time.Now().Add(-retentionWindow).Unix()

	// Delete old entries
	result, err := s.db.Exec("DELETE FROM dedup WHERE seen_at < ?", cutoffTime)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup old OpIDs: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}

	// Optionally run VACUUM to reclaim space (can be expensive)
	// Only vacuum if we deleted a significant number of rows
	if rowsAffected > 10000 {
		if _, err := s.db.Exec("VACUUM"); err != nil {
			// Log warning but don't fail
			// VACUUM failure is not critical
			_ = err
		}
	}

	return int(rowsAffected), nil
}

// Close releases database resources
func (s *SQLiteStore) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// Count returns the total number of OpIDs in the store
func (s *SQLiteStore) Count() (int, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM dedup").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count OpIDs: %w", err)
	}
	return count, nil
}
