package replication

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/rs/zerolog/log"
)

// Applicator applies replicated changes to the local database
type Applicator struct {
	db               *sql.DB
	conflictResolver *ConflictResolver
}

// ApplicatorConfig holds applicator configuration
type ApplicatorConfig struct {
	DB               *sql.DB
	ConflictResolver *ConflictResolver
}

// NewApplicator creates a new change applicator
func NewApplicator(config ApplicatorConfig) (*Applicator, error) {
	if config.DB == nil {
		return nil, fmt.Errorf("database is required")
	}
	if config.ConflictResolver == nil {
		return nil, fmt.Errorf("conflict resolver is required")
	}

	return &Applicator{
		db:               config.DB,
		conflictResolver: config.ConflictResolver,
	}, nil
}

// Apply applies a replication message atomically
func (a *Applicator) Apply(msg ReplicationMessage) error {
	// Start transaction
	tx, err := a.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Apply all changes in transaction
	for _, change := range msg.Transaction {
		if err := a.applyChange(tx, change, msg); err != nil {
			return fmt.Errorf("failed to apply change: %w", err)
		}
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// applyChange applies a single change
func (a *Applicator) applyChange(tx *sql.Tx, change ChangeMessage, msg ReplicationMessage) error {
	switch change.Type {
	case "INSERT":
		return a.applyInsert(tx, change)
	case "UPDATE":
		return a.applyUpdate(tx, change, msg)
	case "DELETE":
		return a.applyDelete(tx, change)
	default:
		return fmt.Errorf("unknown change type: %s", change.Type)
	}
}

// applyInsert applies an INSERT operation
func (a *Applicator) applyInsert(tx *sql.Tx, change ChangeMessage) error {
	// Build INSERT statement
	columns := make([]string, 0, len(change.After))
	placeholders := make([]string, 0, len(change.After))
	values := make([]interface{}, 0, len(change.After))

	for col, val := range change.After {
		columns = append(columns, col)
		placeholders = append(placeholders, "?")
		values = append(values, val)
	}

	query := fmt.Sprintf(
		"INSERT OR REPLACE INTO %s (%s) VALUES (%s)",
		change.Table,
		strings.Join(columns, ", "),
		strings.Join(placeholders, ", "),
	)

	// Execute INSERT
	result, err := tx.Exec(query, values...)
	if err != nil {
		return fmt.Errorf("failed to execute INSERT: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	log.Debug().
		Str("table", change.Table).
		Int64("rows_affected", rowsAffected).
		Msg("INSERT applied")

	return nil
}

// applyUpdate applies an UPDATE operation with conflict resolution
func (a *Applicator) applyUpdate(tx *sql.Tx, change ChangeMessage, msg ReplicationMessage) error {
	// For LWW strategy, check if local row exists and resolve conflict
	// This is a simplified implementation - full version would read local state
	// and use ConflictResolver to determine winner

	// Build UPDATE statement
	setClauses := make([]string, 0, len(change.After))
	values := make([]interface{}, 0, len(change.After)+len(change.PrimaryKey))

	for col, val := range change.After {
		setClauses = append(setClauses, fmt.Sprintf("%s = ?", col))
		values = append(values, val)
	}

	// Build WHERE clause from primary key
	whereClauses := make([]string, 0, len(change.PrimaryKey))
	for i, pkVal := range change.PrimaryKey {
		whereClauses = append(whereClauses, fmt.Sprintf("col%d = ?", i+1)) // Simplified
		values = append(values, pkVal)
	}

	query := fmt.Sprintf(
		"UPDATE %s SET %s WHERE %s",
		change.Table,
		strings.Join(setClauses, ", "),
		strings.Join(whereClauses, " AND "),
	)

	// Execute UPDATE
	result, err := tx.Exec(query, values...)
	if err != nil {
		return fmt.Errorf("failed to execute UPDATE: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	log.Debug().
		Str("table", change.Table).
		Int64("rows_affected", rowsAffected).
		Msg("UPDATE applied")

	return nil
}

// applyDelete applies a DELETE operation
func (a *Applicator) applyDelete(tx *sql.Tx, change ChangeMessage) error {
	// Build DELETE statement
	whereClauses := make([]string, 0, len(change.PrimaryKey))
	values := make([]interface{}, 0, len(change.PrimaryKey))

	for i, pkVal := range change.PrimaryKey {
		whereClauses = append(whereClauses, fmt.Sprintf("col%d = ?", i+1)) // Simplified
		values = append(values, pkVal)
	}

	query := fmt.Sprintf(
		"DELETE FROM %s WHERE %s",
		change.Table,
		strings.Join(whereClauses, " AND "),
	)

	// Execute DELETE
	result, err := tx.Exec(query, values...)
	if err != nil {
		return fmt.Errorf("failed to execute DELETE: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	log.Debug().
		Str("table", change.Table).
		Int64("rows_affected", rowsAffected).
		Msg("DELETE applied")

	return nil
}
