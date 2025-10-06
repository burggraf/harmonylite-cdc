# Contract: WAL Parser Interface

## Overview
Defines the contract for the WAL parser component that reads SQLite WAL files and extracts change events.

## Interface

```go
type Parser interface {
    // NextTransaction blocks until a complete transaction is available
    // Returns atomic batch of changes or error
    NextTransaction(ctx context.Context) ([]Change, error)

    // GetLSN returns current parser position in WAL
    GetLSN() int64

    // Checkpoint triggers coordinated checkpoint after replication
    Checkpoint(mode CheckpointMode) error

    // Close releases resources
    Close() error
}
```

## Contract Guarantees

### NextTransaction()
**Pre-conditions**:
- Parser initialized with valid database path
- Database in WAL mode (or automatically enabled per FR-005)
- Context not cancelled

**Post-conditions**:
- Returns complete transaction (commit frame detected) OR
- Returns error if parsing failed OR
- Blocks if no complete transaction available

**Invariants**:
- Changes within transaction have same TransactionID
- Changes ordered by LSN
- Transaction atomicity preserved (all or nothing)
- Partial transactions never returned (FR-015)
- Rollback frames cause buffered operations to be discarded (FR-016)

**Error Conditions**:
- WAL file not found or unreadable → ErrWALNotFound
- WAL checksum validation failed (FR-013) → ErrWALCorrupted
- Context cancelled → context.Canceled
- Parser closed → ErrParserClosed

### GetLSN()
**Pre-conditions**: Parser initialized

**Post-conditions**: Returns current LSN position (monotonically increasing)

**Invariants**: LSN never decreases

### Checkpoint()
**Pre-conditions**:
- All changes up to current LSN replicated to NATS (confirmed)
- Database connection available

**Post-conditions**:
- SQLite checkpoint executed OR
- Error returned if checkpoint failed

**Invariants**:
- Checkpoints only occur after replication confirmed (FR-020)
- LSN tracking updated after successful checkpoint

**Error Conditions**:
- Checkpoint before replication → ErrCheckpointUnsafe
- Database locked → ErrDatabaseBusy
- Filesystem error → ErrCheckpointFailed

## Test Contract

Tests MUST verify:
1. ✅ Valid WAL frame decoding (unit test)
2. ✅ Transaction boundary detection via commit frames (unit test)
3. ✅ Rollback handling discards buffered operations (unit test)
4. ✅ Large transaction (>1000 ops) triggers disk buffering (integration test)
5. ✅ Checksum validation detects corruption (unit test)
6. ✅ Parser resumes from last LSN after restart (integration test)
7. ✅ External checkpoint detected and handled (integration test)
8. ✅ WAL size > 100MB triggers forced checkpoint (integration test)

## Performance Contract

- Parsing throughput ≥ 10,000 ops/sec (raw parsing, no replication)
- Memory usage ≤ 100MB for transactions up to 1000 operations
- Disk-based buffering for transactions > 1000 operations (configurable, FR-011)
- Checkpoint execution < 1 second for WAL up to 100MB

## Example Usage

```go
// Initialize parser
parser, err := walparser.NewParser(dbPath, lastReplicatedLSN, config)
if err != nil {
    return err
}
defer parser.Close()

// Main loop
for {
    // Block until transaction available
    changes, err := parser.NextTransaction(ctx)
    if err != nil {
        if errors.Is(err, walparser.ErrWALCorrupted) {
            // Stop parsing, emit alert, trigger resync
            return err
        }
        // Handle other errors
    }

    // Replicate transaction
    if err := replicateTransaction(changes); err != nil {
        // Retry logic
    }

    // Checkpoint after replication confirmed
    if parser.GetLSN() - lastCheckpointLSN > checkpointInterval {
        if err := parser.Checkpoint(walparser.CheckpointPassive); err != nil {
            // Log warning, continue
        }
        lastCheckpointLSN = parser.GetLSN()
    }
}
```
