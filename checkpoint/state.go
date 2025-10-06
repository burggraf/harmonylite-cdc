package checkpoint

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// CheckpointState tracks LSN positions for checkpoint coordination
type CheckpointState struct {
	ParsedLSN       int64     `json:"parsed_lsn"`
	ReplicatedLSN   int64     `json:"replicated_lsn"`
	CheckpointedLSN int64     `json:"checkpointed_lsn"`
	UpdatedAt       time.Time `json:"updated_at"`
	NodeID          string    `json:"node_id"`
}

// StatePersistence handles dual-write persistence to NATS and local file
type StatePersistence struct {
	localFilePath string
	nodeID        string
	mu            sync.Mutex
}

// NewStatePersistence creates a new state persistence manager
func NewStatePersistence(localFilePath, nodeID string) *StatePersistence {
	return &StatePersistence{
		localFilePath: localFilePath,
		nodeID:        nodeID,
	}
}

// SaveLocal persists checkpoint state to local JSON file
func (sp *StatePersistence) SaveLocal(state *CheckpointState) error {
	sp.mu.Lock()
	defer sp.mu.Unlock()

	// Update timestamp
	state.UpdatedAt = time.Now()
	state.NodeID = sp.nodeID

	// Marshal to JSON
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal checkpoint state: %w", err)
	}

	// Write to temp file first (atomic write)
	tempFile := sp.localFilePath + ".tmp"
	if err := os.WriteFile(tempFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	// Rename to final location (atomic on POSIX)
	if err := os.Rename(tempFile, sp.localFilePath); err != nil {
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

// LoadLocal loads checkpoint state from local JSON file
func (sp *StatePersistence) LoadLocal() (*CheckpointState, error) {
	sp.mu.Lock()
	defer sp.mu.Unlock()

	// Read file
	data, err := os.ReadFile(sp.localFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist, return initial state
			return &CheckpointState{
				ParsedLSN:       0,
				ReplicatedLSN:   0,
				CheckpointedLSN: 0,
				UpdatedAt:       time.Now(),
				NodeID:          sp.nodeID,
			}, nil
		}
		return nil, fmt.Errorf("failed to read state file: %w", err)
	}

	// Unmarshal JSON
	var state CheckpointState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to unmarshal state: %w", err)
	}

	return &state, nil
}

// SaveToNATS persists checkpoint state to NATS JetStream metadata
// TODO: Implement NATS persistence (FR-019)
func (sp *StatePersistence) SaveToNATS(state *CheckpointState) error {
	// TODO: Store in NATS JetStream consumer metadata or stream metadata
	// This will be implemented when integrating with NATS module
	return fmt.Errorf("NATS persistence not yet implemented")
}

// LoadFromNATS loads checkpoint state from NATS JetStream metadata
// TODO: Implement NATS loading (FR-048)
func (sp *StatePersistence) LoadFromNATS() (*CheckpointState, error) {
	// TODO: Read from NATS JetStream consumer metadata
	// This will be implemented when integrating with NATS module
	return nil, fmt.Errorf("NATS loading not yet implemented")
}

// Save performs dual-write to both NATS and local file
func (sp *StatePersistence) Save(state *CheckpointState) error {
	// Always save to local file
	if err := sp.SaveLocal(state); err != nil {
		return fmt.Errorf("failed to save to local file: %w", err)
	}

	// Attempt to save to NATS (best effort for now)
	if err := sp.SaveToNATS(state); err != nil {
		// Log warning but don't fail (NATS might not be available)
		// TODO: Add proper logging
		_ = err
	}

	return nil
}

// Load attempts to load from NATS first, falls back to local file
func (sp *StatePersistence) Load() (*CheckpointState, error) {
	// Try NATS first (FR-048)
	state, err := sp.LoadFromNATS()
	if err == nil {
		return state, nil
	}

	// Fallback to local file (FR-049)
	return sp.LoadLocal()
}
