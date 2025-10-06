# Data Model: WAL-Based SQLite Replication System

## Overview

This document defines the core data entities for the WAL-based SQLite replication system. These entities represent the domain model derived from functional requirements in the feature specification.

## Core Entities

### 1. Change

Represents a logical database operation extracted from the SQLite WAL file.

**Fields**:
- `OpID` (string, required): Unique operation identifier (format: `{node_id}:{lsn}:{timestamp}`)
- `NodeID` (string, required): Identifier of the originating node
- `LSN` (int64, required): Log Sequence Number in WAL (position of operation)
- `Table` (string, required): Name of the table being modified
- `PrimaryKey` ([]interface{}, required): Primary key value(s) identifying the row
- `Type` (ChangeType enum, required): Operation type (INSERT, UPDATE, DELETE)
- `Columns` (map[string]interface{}, optional): All column values for the row
- `Before` (map[string]interface{}, optional): Old values (for UPDATE/DELETE)
- `After` (map[string]interface{}, optional): New values (for INSERT/UPDATE)
- `TransactionID` (string, required): Groups operations in same transaction
- `CommitLSN` (int64, required): LSN of transaction commit frame

**Validation Rules**:
- OpID must be unique across all nodes (enforced by deduplication store)
- LSN must be monotonically increasing within a node
- Type must be one of: INSERT, UPDATE, DELETE
- Before must be nil for INSERT operations
- After must be nil for DELETE operations
- Both Before and After must be present for UPDATE operations
- TransactionID groups all operations from same SQLite transaction

**State Transitions**: None (immutable once created)

**Relationships**:
- Belongs to one Transaction (via TransactionID)
- Referenced by DeduplicationStore (via OpID)
- Published in one ReplicationMessage

### 2. Transaction

Group of Change events that must be applied atomically.

**Fields**:
- `ID` (string, required): Unique transaction identifier
- `Changes` ([]Change, required): Ordered list of changes in transaction
- `CommitLSN` (int64, required): LSN where transaction committed
- `NodeID` (string, required): Originating node
- `Timestamp` (time.Time, required): Wall clock time of commit

**Validation Rules**:
- Must contain at least one Change
- All Changes must have same TransactionID
- All Changes must have same CommitLSN
- Changes must be ordered by LSN within transaction
- Transaction size unlimited (disk-based buffering)

**State Transitions**:
- Pending → Committed (when commit frame detected in WAL)
- Pending → Rolled Back (when rollback detected, buffer discarded)

**Relationships**:
- Contains multiple Changes (1:N)
- Included in one ReplicationMessage for transmission

### 3. LSN (Log Sequence Number)

Position tracking in WAL file for replication progress.

**Fields**:
- `Value` (int64, required): Byte offset or frame number in WAL
- `NodeID` (string, required): Which node's WAL this LSN refers to
- `Type` (LSNType enum, required): Purpose of this LSN (parsed, replicated, checkpointed)
- `Timestamp` (time.Time, required): When this LSN value was recorded

**Validation Rules**:
- Value must be non-negative
- Value must be monotonically increasing for a given NodeID and Type
- Type must be one of: LastParsed, LastReplicated, LastCheckpointed

**State Transitions**: Value increases monotonically as WAL is processed

**Relationships**:
- Tracked by CheckpointState
- Used by Parser to resume after crash

### 4. OpID (Operation Identifier)

Unique identifier for deduplication of change events.

**Fields**:
- `Full` (string, required): Complete OpID string (format: `{node_id}:{lsn}:{timestamp}`)
- `NodeID` (string, derived): Extracted from Full
- `LSN` (int64, derived): Extracted from Full
- `Timestamp` (int64, derived): Unix timestamp extracted from Full

**Validation Rules**:
- Format must match: `{node_id}:{lsn}:{timestamp}`
- NodeID must be valid node identifier
- LSN must be positive integer
- Timestamp must be valid Unix timestamp (seconds)
- Must be globally unique across all nodes

**State Transitions**: None (immutable)

**Relationships**:
- One OpID per Change (1:1)
- Stored in DeduplicationStore with retention window

### 5. ReplicationMessage

NATS JetStream envelope for transmitting changes between nodes.

**Fields**:
- `OpID` (string, required): Unique operation identifier
- `NodeID` (string, required): Originating node
- `DBID` (string, required): Database identifier
- `SchemaVersion` (int, required): Message schema version for compatibility
- `WallTime` (time.Time, required): Wall clock timestamp
- `LamportClock` (int64, required): Lamport logical clock for ordering
- `Transaction` ([]Change, required): Atomic batch of changes

**Validation Rules**:
- OpID must be unique
- SchemaVersion must match expected version (reject mismatches)
- Transaction must contain at least one Change
- All Changes in Transaction must have same TransactionID
- LamportClock must increase monotonically

**State Transitions**: Published → Acknowledged (by NATS JetStream)

**Relationships**:
- Contains one Transaction (1:1)
- Published to NATS JetStream subject
- Consumed by remote node Subscribers

### 6. DeduplicationStore

Persistent key-value store for tracking processed OpIDs.

**Fields**:
- `OpID` (string, key): Operation identifier
- `ProcessedAt` (int64, value): Unix timestamp when operation was processed

**Validation Rules**:
- OpID must be unique key
- ProcessedAt must be valid Unix timestamp
- Store must persist across process restarts
- Cleanup must remove entries older than retention window (default: 1 hour)

**State Transitions**:
- Not Present → Present (when operation first processed)
- Present → Deleted (after retention window expires)

**Relationships**:
- Stores OpIDs from processed Changes
- Queried before applying remote changes

### 7. CheckpointState

Tracks LSN positions for safe checkpoint coordination.

**Fields**:
- `LastParsedLSN` (int64, required): Latest LSN read from WAL
- `LastReplicatedLSN` (int64, required): Latest LSN confirmed replicated to NATS
- `LastCheckpointedLSN` (int64, required): Latest LSN checkpointed to database
- `NodeID` (string, required): This node's identifier
- `UpdatedAt` (time.Time, required): Last update timestamp

**Validation Rules**:
- LastCheckpointedLSN ≤ LastReplicatedLSN ≤ LastParsedLSN (checkpoint safety)
- Gap between LastReplicatedLSN and LastParsedLSN triggers resync if > 10,000 ops
- Must persist to both NATS JetStream metadata and local state file

**State Transitions**: LSN values increase monotonically as replication progresses

**Relationships**:
- Managed by CheckpointManager
- Persisted to local state file and NATS metadata

### 8. ConflictResolutionStrategy

Configuration for how to resolve concurrent writes to same row.

**Fields**:
- `TableName` (string, required): Which table this strategy applies to
- `Strategy` (StrategyType enum, required): Resolution algorithm
- `Config` (map[string]interface{}, optional): Strategy-specific parameters

**Validation Rules**:
- Strategy must be one of: LastWriteWins, Counter, AppendOnly
- TableName must match existing database table
- If nodes have different strategies for same table, default to LastWriteWins (per clarification)

**State Transitions**: Loaded from configuration, static at runtime

**Relationships**:
- Applied by Applicator when conflict detected
- One strategy per table (default: LastWriteWins)

## Enumerations

### ChangeType
- `INSERT`: New row created
- `UPDATE`: Existing row modified
- `DELETE`: Row removed

### LSNType
- `LastParsed`: Latest position read from WAL
- `LastReplicated`: Latest position confirmed replicated to NATS
- `LastCheckpointed`: Latest position checkpointed to database

### StrategyType
- `LastWriteWins`: Use Lamport clock + wall time to pick winner
- `Counter`: Sum all counter increments
- `AppendOnly`: Merge all values (for append-only columns)

## Relationships Diagram

```
Change
  ├─ belongs to Transaction (via TransactionID)
  ├─ has OpID (unique identifier)
  └─ included in ReplicationMessage

Transaction
  ├─ contains Changes (1:N)
  └─ packaged in ReplicationMessage

ReplicationMessage
  ├─ contains Transaction
  ├─ published to NATS JetStream
  └─ consumed by remote Subscribers

LSN
  └─ tracked by CheckpointState

CheckpointState
  └─ persisted to NATS metadata + local file

OpID
  └─ stored in DeduplicationStore

DeduplicationStore
  └─ queries for duplicate detection

ConflictResolutionStrategy
  └─ applied by Applicator
```

## Data Volume Estimates

Based on specification requirements:

- **Changes per second**: 1,000 ops/sec sustained (per node)
- **Change size**: Average 1KB, max 10MB (configurable)
- **Transaction size**: Typically 1-100 operations, max unlimited (disk buffer)
- **Deduplication store**: ~3.6M OpIDs per hour (1K ops/sec × 3600s), cleanup after 1 hour
- **LSN growth**: Proportional to WAL size (tested to 1GB WAL, triggers checkpoint at 100MB)
- **ReplicationMessage**: Matches transaction size, transmitted via NATS JetStream

## Persistence Strategy

- **WAL files**: Read-only, managed by SQLite
- **LSN state**: Dual persistence (NATS JetStream metadata + local JSON file)
- **Deduplication store**: SQLite database (per research.md recommendation)
- **Configuration**: YAML file, loaded at startup
- **Metrics**: In-memory, exposed via Prometheus endpoint
- **Logs**: Streaming to stdout (structured JSON)
