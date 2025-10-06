package db

import (
	"database/sql"
	"fmt"

	"github.com/rs/zerolog/log"
)

// EnableWALMode enables WAL mode on the database
// This is required for WAL-based CDC
func EnableWALMode(db *sql.DB) error {
	// Check current journal mode
	var journalMode string
	err := db.QueryRow("PRAGMA journal_mode").Scan(&journalMode)
	if err != nil {
		return fmt.Errorf("failed to query journal mode: %w", err)
	}

	// If already in WAL mode, nothing to do
	if journalMode == "wal" {
		log.Info().Msg("Database already in WAL mode")
		return nil
	}

	// Enable WAL mode
	var newMode string
	err = db.QueryRow("PRAGMA journal_mode = WAL").Scan(&newMode)
	if err != nil {
		return fmt.Errorf("failed to enable WAL mode: %w", err)
	}

	if newMode != "wal" {
		return fmt.Errorf("failed to enable WAL mode: got %s", newMode)
	}

	log.Info().
		Str("previous_mode", journalMode).
		Str("new_mode", newMode).
		Msg("WAL mode enabled")

	return nil
}

// GetJournalMode returns the current journal mode
func GetJournalMode(db *sql.DB) (string, error) {
	var journalMode string
	err := db.QueryRow("PRAGMA journal_mode").Scan(&journalMode)
	if err != nil {
		return "", fmt.Errorf("failed to query journal mode: %w", err)
	}
	return journalMode, nil
}

// DisableAutoCheckpoint disables SQLite's automatic checkpoint
// This is handled by checkpoint.Manager instead
func DisableAutoCheckpoint(db *sql.DB) error {
	_, err := db.Exec("PRAGMA wal_autocheckpoint = 0")
	if err != nil {
		return fmt.Errorf("failed to disable auto-checkpoint: %w", err)
	}

	log.Info().Msg("SQLite auto-checkpoint disabled")
	return nil
}
