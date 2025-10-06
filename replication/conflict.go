package replication

import (
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
)

// ConflictStrategy defines how to resolve concurrent writes
type ConflictStrategy string

const (
	StrategyLWW        ConflictStrategy = "lww"         // Last-Write-Wins
	StrategyCounter    ConflictStrategy = "counter"     // Sum increments
	StrategyAppendOnly ConflictStrategy = "append-only" // Append all writes
)

// ConflictResolver resolves conflicts between concurrent writes
type ConflictResolver struct {
	defaultStrategy ConflictStrategy
	tableStrategies map[string]ConflictStrategy
}

// NewConflictResolver creates a new conflict resolver
func NewConflictResolver(defaultStrategy ConflictStrategy, tableStrategies map[string]ConflictStrategy) *ConflictResolver {
	if tableStrategies == nil {
		tableStrategies = make(map[string]ConflictStrategy)
	}

	return &ConflictResolver{
		defaultStrategy: defaultStrategy,
		tableStrategies: tableStrategies,
	}
}

// ResolveConflict determines which change wins in a conflict
// Returns: winner change, loser change, strategy used
func (r *ConflictResolver) ResolveConflict(local, remote Change) (Change, Change, ConflictStrategy, error) {
	// Get strategy for this table
	strategy := r.getStrategy(local.Table)

	// Validate both changes are for same table/row
	if local.Table != remote.Table {
		return Change{}, Change{}, "", fmt.Errorf("table mismatch: %s vs %s", local.Table, remote.Table)
	}

	switch strategy {
	case StrategyLWW:
		return r.resolveLWW(local, remote)

	case StrategyCounter:
		return r.resolveCounter(local, remote)

	case StrategyAppendOnly:
		return r.resolveAppendOnly(local, remote)

	default:
		// Unknown strategy, default to LWW
		log.Warn().
			Str("table", local.Table).
			Str("strategy", string(strategy)).
			Msg("Unknown conflict strategy, defaulting to LWW")
		return r.resolveLWW(local, remote)
	}
}

// resolveLWW implements Last-Write-Wins using Lamport clock
func (r *ConflictResolver) resolveLWW(local, remote Change) (Change, Change, ConflictStrategy, error) {
	// Compare Lamport clocks (FR-039)
	if remote.LamportClock > local.LamportClock {
		// Remote wins
		r.logResolution(remote.Table, StrategyLWW, "remote", remote.LamportClock, local.LamportClock)
		return remote, local, StrategyLWW, nil
	} else if local.LamportClock > remote.LamportClock {
		// Local wins
		r.logResolution(local.Table, StrategyLWW, "local", local.LamportClock, remote.LamportClock)
		return local, remote, StrategyLWW, nil
	}

	// Lamport clocks are equal, use wall time as tiebreaker (FR-040)
	if remote.WallTime.After(local.WallTime) {
		r.logResolution(remote.Table, StrategyLWW, "remote (wall time tiebreaker)", remote.LamportClock, local.LamportClock)
		return remote, local, StrategyLWW, nil
	}

	// Local wins (wall time equal or local later)
	r.logResolution(local.Table, StrategyLWW, "local (wall time tiebreaker)", local.LamportClock, remote.LamportClock)
	return local, remote, StrategyLWW, nil
}

// resolveCounter implements counter-based conflict resolution (sum increments)
func (r *ConflictResolver) resolveCounter(local, remote Change) (Change, Change, ConflictStrategy, error) {
	// For counter strategy, both changes are applied (summed)
	// This requires special handling in the applicator
	// For now, we mark both as winners
	r.logResolution(local.Table, StrategyCounter, "both (sum)", 0, 0)
	return local, remote, StrategyCounter, nil
}

// resolveAppendOnly implements append-only conflict resolution
func (r *ConflictResolver) resolveAppendOnly(local, remote Change) (Change, Change, ConflictStrategy, error) {
	// For append-only tables, all writes are preserved
	// Both changes are winners
	r.logResolution(local.Table, StrategyAppendOnly, "both (append)", 0, 0)
	return local, remote, StrategyAppendOnly, nil
}

// getStrategy returns the conflict resolution strategy for a table
func (r *ConflictResolver) getStrategy(table string) ConflictStrategy {
	if strategy, ok := r.tableStrategies[table]; ok {
		return strategy
	}
	return r.defaultStrategy
}

// logResolution logs conflict resolution (FR-062)
func (r *ConflictResolver) logResolution(table string, strategy ConflictStrategy, winner string, winnerClock, loserClock uint64) {
	log.Info().
		Str("table", table).
		Str("strategy", string(strategy)).
		Str("winner", winner).
		Uint64("winner_clock", winnerClock).
		Uint64("loser_clock", loserClock).
		Msg("Conflict resolved")
}

// Change represents a database change with conflict resolution metadata
type Change struct {
	OpID         string
	Table        string
	LamportClock uint64
	WallTime     time.Time
	// ... other fields
}
