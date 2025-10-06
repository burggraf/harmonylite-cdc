package checkpoint

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// Manager coordinates checkpoint operations
type Manager struct {
	db                *sql.DB
	tracker           *LSNTracker
	persistence       *StatePersistence
	forceThresholdMB  int
	disableAuto       bool
	mu                sync.Mutex
}

// Config holds checkpoint manager configuration
type Config struct {
	DBPath               string
	StateFile            string
	NodeID               string
	ForceCheckpointWALMB int
	DisableAutoCheckpoint bool
}

// NewManager creates a new checkpoint manager
func NewManager(db *sql.DB, config Config) (*Manager, error) {
	// Create state persistence
	persistence := NewStatePersistence(config.StateFile, config.NodeID)

	// Load initial state
	state, err := persistence.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load checkpoint state: %w", err)
	}

	// Create LSN tracker with loaded state
	tracker := NewLSNTracker(config.NodeID, state.CheckpointedLSN)
	tracker.SetParsedLSN(state.ParsedLSN)
	tracker.SetReplicatedLSN(state.ReplicatedLSN)
	tracker.SetCheckpointedLSN(state.CheckpointedLSN)

	manager := &Manager{
		db:               db,
		tracker:          tracker,
		persistence:      persistence,
		forceThresholdMB: config.ForceCheckpointWALMB,
		disableAuto:      config.DisableAutoCheckpoint,
	}

	// Disable SQLite auto-checkpoint if configured (FR-017)
	if config.DisableAutoCheckpoint {
		if err := manager.disableSQLiteAutoCheckpoint(); err != nil {
			return nil, fmt.Errorf("failed to disable auto-checkpoint: %w", err)
		}
	}

	return manager, nil
}

// ExecuteCheckpoint performs a checkpoint operation
func (m *Manager) ExecuteCheckpoint(ctx context.Context, mode int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Verify replication is caught up (FR-020)
	parsed, replicated, _ := m.tracker.GetAll()
	if parsed > replicated {
		return fmt.Errorf("checkpoint unsafe: parsed=%d > replicated=%d", parsed, replicated)
	}

	startTime := time.Now()

	// Execute SQLite checkpoint
	// PRAGMA wal_checkpoint(mode)
	// Modes: PASSIVE=0, FULL=1, RESTART=2, TRUNCATE=3
	var modeStr string
	switch mode {
	case 0:
		modeStr = "PASSIVE"
	case 1:
		modeStr = "FULL"
	case 2:
		modeStr = "RESTART"
	case 3:
		modeStr = "TRUNCATE"
	default:
		modeStr = "PASSIVE"
	}

	query := fmt.Sprintf("PRAGMA wal_checkpoint(%s)", modeStr)
	rows, err := m.db.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("checkpoint failed: %w", err)
	}
	defer rows.Close()

	// Parse checkpoint result
	// Returns: busy, log frames, checkpointed frames
	var busy, logFrames, checkpointedFrames int
	if rows.Next() {
		if err := rows.Scan(&busy, &logFrames, &checkpointedFrames); err != nil {
			return fmt.Errorf("failed to scan checkpoint result: %w", err)
		}
	}

	if busy != 0 {
		return fmt.Errorf("checkpoint busy: database locked")
	}

	// Update checkpointed LSN
	if err := m.tracker.SetCheckpointedLSN(replicated); err != nil {
		return fmt.Errorf("failed to update checkpointed LSN: %w", err)
	}

	// Persist state
	if err := m.saveState(); err != nil {
		return fmt.Errorf("failed to persist checkpoint state: %w", err)
	}

	duration := time.Since(startTime)

	// Log checkpoint operation (FR-061)
	log.Info().
		Int64("lsn", replicated).
		Dur("duration", duration).
		Str("mode", modeStr).
		Int("checkpointed_frames", checkpointedFrames).
		Msg("Checkpoint completed")

	return nil
}

// ShouldCheckpoint determines if checkpoint should be triggered
func (m *Manager) ShouldCheckpoint() (bool, string) {
	// Check WAL size
	walSize, err := m.getWALSize()
	if err != nil {
		return false, ""
	}

	// Convert threshold to bytes
	thresholdBytes := int64(m.forceThresholdMB) * 1024 * 1024

	if walSize > thresholdBytes {
		return true, fmt.Sprintf("WAL size (%d MB) exceeds threshold (%d MB)",
			walSize/(1024*1024), m.forceThresholdMB)
	}

	return false, ""
}

// GetTracker returns the LSN tracker
func (m *Manager) GetTracker() *LSNTracker {
	return m.tracker
}

// saveState persists current checkpoint state
func (m *Manager) saveState() error {
	parsed, replicated, checkpointed := m.tracker.GetAll()

	state := &CheckpointState{
		ParsedLSN:       parsed,
		ReplicatedLSN:   replicated,
		CheckpointedLSN: checkpointed,
	}

	return m.persistence.Save(state)
}

// disableSQLiteAutoCheckpoint disables SQLite's automatic checkpoint
func (m *Manager) disableSQLiteAutoCheckpoint() error {
	// PRAGMA wal_autocheckpoint = 0 disables auto-checkpoint
	_, err := m.db.Exec("PRAGMA wal_autocheckpoint = 0")
	if err != nil {
		return fmt.Errorf("failed to disable auto-checkpoint: %w", err)
	}

	log.Info().Msg("SQLite auto-checkpoint disabled")
	return nil
}

// getWALSize returns the current WAL file size in bytes
func (m *Manager) getWALSize() (int64, error) {
	// Query SQLite for WAL size
	var pageCount, pageSize int64
	err := m.db.QueryRow("PRAGMA page_count").Scan(&pageCount)
	if err != nil {
		return 0, err
	}

	err = m.db.QueryRow("PRAGMA page_size").Scan(&pageSize)
	if err != nil {
		return 0, err
	}

	// Approximate WAL size (actual implementation would check file size)
	return pageCount * pageSize, nil
}

// DetectExternalCheckpoint checks if an external checkpoint occurred
func (m *Manager) DetectExternalCheckpoint(ctx context.Context) (bool, error) {
	// TODO: Monitor nBackfill from WAL-index (SHM file)
	// If nBackfill increases without manager triggering it, external checkpoint occurred
	// See FR-021
	return false, fmt.Errorf("not implemented")
}
