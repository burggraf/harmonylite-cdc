package dedup

import (
	"time"
)

// Store interface for OpID deduplication
type Store interface {
	// Has checks if OpID exists in store
	Has(opID string) (bool, error)

	// Add stores OpID with timestamp
	Add(opID string, timestamp time.Time) error

	// Cleanup removes OpIDs older than retention window
	Cleanup(retentionWindow time.Duration) (int, error)

	// Close releases resources
	Close() error
}
