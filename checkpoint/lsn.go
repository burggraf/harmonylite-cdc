package checkpoint

import (
	"fmt"
	"sync"
	"time"
)

// LSNType represents the purpose of an LSN value
type LSNType string

const (
	LSNTypeParsed       LSNType = "LastParsed"
	LSNTypeReplicated   LSNType = "LastReplicated"
	LSNTypeCheckpointed LSNType = "LastCheckpointed"
)

// LSN represents a Log Sequence Number tracking position in WAL
type LSN struct {
	Value     int64     // LSN value (byte offset or frame number)
	NodeID    string    // Which node's WAL this refers to
	Type      LSNType   // Purpose of this LSN
	Timestamp time.Time // When this LSN was recorded
}

// LSNTracker tracks LSN values for parsing, replication, and checkpointing
type LSNTracker struct {
	nodeID            string
	parsedLSN         int64
	replicatedLSN     int64
	checkpointedLSN   int64
	mu                sync.RWMutex
}

// NewLSNTracker creates a new LSN tracker
func NewLSNTracker(nodeID string, initialLSN int64) *LSNTracker {
	return &LSNTracker{
		nodeID:          nodeID,
		parsedLSN:       initialLSN,
		replicatedLSN:   initialLSN,
		checkpointedLSN: initialLSN,
	}
}

// GetParsedLSN returns the last parsed LSN
func (t *LSNTracker) GetParsedLSN() int64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.parsedLSN
}

// GetReplicatedLSN returns the last replicated LSN
func (t *LSNTracker) GetReplicatedLSN() int64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.replicatedLSN
}

// GetCheckpointedLSN returns the last checkpointed LSN
func (t *LSNTracker) GetCheckpointedLSN() int64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.checkpointedLSN
}

// SetParsedLSN updates the parsed LSN
func (t *LSNTracker) SetParsedLSN(lsn int64) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Validate monotonic increase
	if lsn < t.parsedLSN {
		return fmt.Errorf("parsed LSN cannot decrease: current=%d, new=%d", t.parsedLSN, lsn)
	}

	t.parsedLSN = lsn
	return nil
}

// SetReplicatedLSN updates the replicated LSN
func (t *LSNTracker) SetReplicatedLSN(lsn int64) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Validate monotonic increase
	if lsn < t.replicatedLSN {
		return fmt.Errorf("replicated LSN cannot decrease: current=%d, new=%d", t.replicatedLSN, lsn)
	}

	// Validate invariant: replicated <= parsed
	if lsn > t.parsedLSN {
		return fmt.Errorf("replicated LSN cannot exceed parsed LSN: replicated=%d, parsed=%d", lsn, t.parsedLSN)
	}

	t.replicatedLSN = lsn
	return nil
}

// SetCheckpointedLSN updates the checkpointed LSN
func (t *LSNTracker) SetCheckpointedLSN(lsn int64) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Validate monotonic increase
	if lsn < t.checkpointedLSN {
		return fmt.Errorf("checkpointed LSN cannot decrease: current=%d, new=%d", t.checkpointedLSN, lsn)
	}

	// Validate invariant: checkpointed <= replicated
	if lsn > t.replicatedLSN {
		return fmt.Errorf("checkpointed LSN cannot exceed replicated LSN: checkpointed=%d, replicated=%d", lsn, t.replicatedLSN)
	}

	t.checkpointedLSN = lsn
	return nil
}

// GetLag returns the replication lag (parsed - replicated)
func (t *LSNTracker) GetLag() int64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.parsedLSN - t.replicatedLSN
}

// ValidateInvariant checks that parsed >= replicated >= checkpointed
func (t *LSNTracker) ValidateInvariant() error {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.replicatedLSN > t.parsedLSN {
		return fmt.Errorf("invariant violation: replicated (%d) > parsed (%d)", t.replicatedLSN, t.parsedLSN)
	}

	if t.checkpointedLSN > t.replicatedLSN {
		return fmt.Errorf("invariant violation: checkpointed (%d) > replicated (%d)", t.checkpointedLSN, t.replicatedLSN)
	}

	return nil
}

// GetAll returns all LSN values
func (t *LSNTracker) GetAll() (parsed, replicated, checkpointed int64) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.parsedLSN, t.replicatedLSN, t.checkpointedLSN
}

// ToLSNs converts tracker state to LSN objects
func (t *LSNTracker) ToLSNs() []LSN {
	t.mu.RLock()
	defer t.mu.RUnlock()

	now := time.Now()
	return []LSN{
		{
			Value:     t.parsedLSN,
			NodeID:    t.nodeID,
			Type:      LSNTypeParsed,
			Timestamp: now,
		},
		{
			Value:     t.replicatedLSN,
			NodeID:    t.nodeID,
			Type:      LSNTypeReplicated,
			Timestamp: now,
		},
		{
			Value:     t.checkpointedLSN,
			NodeID:    t.nodeID,
			Type:      LSNTypeCheckpointed,
			Timestamp: now,
		},
	}
}
