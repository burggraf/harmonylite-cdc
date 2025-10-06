package walparser

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// TransactionBuffer manages buffering of operations until commit
type TransactionBuffer struct {
	memoryBuffer     []Change
	diskBufferPath   string
	memoryThreshold  int
	usingDisk        bool
	currentTxID      string
	mu               sync.Mutex
}

// NewTransactionBuffer creates a new transaction buffer
func NewTransactionBuffer(memoryThreshold int, diskBufferDir string) (*TransactionBuffer, error) {
	// Create disk buffer directory if needed
	if err := os.MkdirAll(diskBufferDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create disk buffer directory: %w", err)
	}

	return &TransactionBuffer{
		memoryBuffer:    make([]Change, 0, memoryThreshold),
		memoryThreshold: memoryThreshold,
	}, nil
}

// Add adds a change to the buffer
func (tb *TransactionBuffer) Add(change Change) error {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	// Set transaction ID if this is the first operation
	if tb.currentTxID == "" {
		tb.currentTxID = change.TransactionID
	}

	// Verify all changes belong to same transaction
	if change.TransactionID != tb.currentTxID {
		return fmt.Errorf("change transaction ID mismatch: expected %s, got %s",
			tb.currentTxID, change.TransactionID)
	}

	// Check if we need to spill to disk
	if !tb.usingDisk && len(tb.memoryBuffer) >= tb.memoryThreshold {
		if err := tb.spillToDisk(); err != nil {
			return fmt.Errorf("failed to spill to disk: %w", err)
		}
	}

	if tb.usingDisk {
		// Append to disk file
		return tb.appendToDisk(change)
	}

	// Add to memory buffer
	tb.memoryBuffer = append(tb.memoryBuffer, change)
	return nil
}

// Commit returns all buffered changes and clears the buffer
func (tb *TransactionBuffer) Commit() ([]Change, error) {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	var changes []Change

	if tb.usingDisk {
		// Read from disk
		var err error
		changes, err = tb.readFromDisk()
		if err != nil {
			return nil, fmt.Errorf("failed to read from disk: %w", err)
		}

		// Clean up disk file
		if err := os.Remove(tb.diskBufferPath); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to remove disk buffer: %w", err)
		}
	} else {
		// Return memory buffer
		changes = make([]Change, len(tb.memoryBuffer))
		copy(changes, tb.memoryBuffer)
	}

	// Reset buffer
	tb.reset()

	return changes, nil
}

// Rollback discards all buffered changes
func (tb *TransactionBuffer) Rollback() error {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	if tb.usingDisk {
		// Remove disk file
		if err := os.Remove(tb.diskBufferPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove disk buffer: %w", err)
		}
	}

	tb.reset()
	return nil
}

// Size returns the number of buffered changes
func (tb *TransactionBuffer) Size() int {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	if tb.usingDisk {
		// Count from disk file
		changes, err := tb.readFromDisk()
		if err != nil {
			return 0
		}
		return len(changes)
	}

	return len(tb.memoryBuffer)
}

// IsEmpty returns true if buffer is empty
func (tb *TransactionBuffer) IsEmpty() bool {
	return tb.Size() == 0
}

// spillToDisk moves memory buffer to disk
func (tb *TransactionBuffer) spillToDisk() error {
	// Create temp file for disk buffer
	tempFile, err := os.CreateTemp(filepath.Dir(tb.diskBufferPath), "txbuffer-*.json")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tb.diskBufferPath = tempFile.Name()

	// Write memory buffer to disk
	encoder := json.NewEncoder(tempFile)
	for _, change := range tb.memoryBuffer {
		if err := encoder.Encode(change); err != nil {
			tempFile.Close()
			os.Remove(tb.diskBufferPath)
			return fmt.Errorf("failed to encode change: %w", err)
		}
	}

	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	// Clear memory buffer and mark using disk
	tb.memoryBuffer = nil
	tb.usingDisk = true

	return nil
}

// appendToDisk appends a change to the disk buffer
func (tb *TransactionBuffer) appendToDisk(change Change) error {
	file, err := os.OpenFile(tb.diskBufferPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open disk buffer: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	if err := encoder.Encode(change); err != nil {
		return fmt.Errorf("failed to encode change: %w", err)
	}

	return nil
}

// readFromDisk reads all changes from disk buffer
func (tb *TransactionBuffer) readFromDisk() ([]Change, error) {
	file, err := os.Open(tb.diskBufferPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open disk buffer: %w", err)
	}
	defer file.Close()

	var changes []Change
	decoder := json.NewDecoder(file)

	for {
		var change Change
		if err := decoder.Decode(&change); err != nil {
			if err.Error() == "EOF" {
				break
			}
			return nil, fmt.Errorf("failed to decode change: %w", err)
		}
		changes = append(changes, change)
	}

	return changes, nil
}

// reset clears the buffer state
func (tb *TransactionBuffer) reset() {
	tb.memoryBuffer = make([]Change, 0, tb.memoryThreshold)
	tb.diskBufferPath = ""
	tb.usingDisk = false
	tb.currentTxID = ""
}

// Close cleans up any resources
func (tb *TransactionBuffer) Close() error {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	if tb.usingDisk && tb.diskBufferPath != "" {
		if err := os.Remove(tb.diskBufferPath); err != nil && !os.IsNotExist(err) {
			return err
		}
	}

	return nil
}
