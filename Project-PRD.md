# PRD: WAL-Based SQLite Replication System

**Product Vision:** A high-performance, trigger-free SQLite replication system built on Write-Ahead Log (WAL) monitoring, delivering multi-primary distributed consistency through NATS JetStream without modifying the database.

**Base Architecture:** Forked from [HarmonyLite](https://github.com/wongfei2009/harmonylite)

**License:** Open Source with permissive license (MIT or Apache 2.0)

**Target Users:** Developers building distributed systems with SQLite, with primary focus on PocketBase applications requiring high availability.

---

## Table of Contents
1. [Background & Problem Statement](#1-background--problem-statement)
2. [Product Positioning](#2-product-positioning)
3. [Objectives and Scope](#3-objectives-and-scope)
4. [Success Metrics](#4-success-metrics)
5. [Personas and Use Cases](#5-personas-and-use-cases)
6. [Architecture Overview](#6-architecture-overview)
7. [Technical Requirements](#7-technical-requirements)
8. [Error Handling & Resilience](#8-error-handling--resilience)
9. [Non-Functional Requirements](#9-non-functional-requirements)
10. [Testing and Validation](#10-testing-and-validation)
11. [Milestones](#11-milestones)
12. [Dependencies and Risks](#12-dependencies-and-risks)
13. [Acceptance Criteria](#13-acceptance-criteria)

---

## 1. Background & Problem Statement

### 1.1 The Problem with Trigger-Based CDC

Traditional change-data-capture (CDC) approaches using SQLite triggers suffer from several critical limitations:

**Database Modification Overhead**
- Triggers add per-table write overhead to every DML operation
- Complexity in trigger setup and maintenance
- Database schema becomes polluted with CDC infrastructure

**Operational Complexity**
- Setting up triggers is a complex initialization step
- Database copies/backups contain trigger infrastructure
- Migration away from the system requires trigger cleanup
- Potential for trigger conflicts with application logic

**User Experience Issues**
- Users must trust the CDC system to modify their database
- No guarantee of non-invasive operation
- Debugging trigger-related issues is difficult

### 1.2 The WAL Monitoring Solution

SQLite's Write-Ahead Log (WAL) mode provides a non-invasive alternative:

- **Zero Database Modification**: Parse WAL files externally without touching the database
- **Simplicity**: No trigger setup, no schema pollution
- **Clean Migration**: Database remains pristine; removing replication is trivial
- **Transparency**: Users can verify the system never modifies their data
- **Performance**: Asynchronous parsing eliminates per-write overhead

---

## 2. Product Positioning

### 2.1 What This Product Is

A **generic SQLite replication system** that:
- Provides multi-primary distributed replication via WAL monitoring
- Works with any SQLite database in WAL mode
- Uses NATS JetStream for coordination and message transport
- Supports conflict resolution (LWW, counters, append-only)
- Requires zero modifications to the target database

### 2.2 Primary Use Case: PocketBase

While generic, this system is optimized for **PocketBase** deployments:
- PocketBase uses SQLite as its primary database
- Schema changes happen as data changes (collections stored in tables)
- High-availability PocketBase clusters are a major user request
- Clean sidecar deployment model

### 2.3 Differentiation

- **Only WAL-based**: No trigger mode, simplified architecture
- **Non-invasive**: Guaranteed zero database modification
- **PocketBase-optimized**: Tested and documented for PocketBase workflows
- **Open source**: Permissive licensing for maximum adoption

---

## 3. Objectives and Scope

### 3.1 Objectives

1. **Eliminate database modification** by parsing WAL files externally
2. **Implement robust WAL parser** to extract INSERT/UPDATE/DELETE operations
3. **Maintain NATS JetStream replication** protocol and message formats from HarmonyLite
4. **Ensure transaction atomicity** across distributed nodes
5. **Provide coordinated checkpoint management** for reliability without data loss
6. **Preserve conflict resolution** behaviors (LWW, counters, append-only)
7. **Deliver production-ready system** with comprehensive error handling

### 3.2 In Scope

- Standalone WAL parser library (Go)
- Integration with NATS JetStream replication engine
- Transaction boundary detection and atomic replication
- Coordinated WAL checkpoint management
- Hybrid transaction buffering (memory + disk)
- OpID deduplication with persistent store
- Network resilience and automatic recovery
- Comprehensive test suite and benchmarks
- Documentation for deployment and operation
- PocketBase-specific guides and examples

### 3.3 Out of Scope

- **DDL/Schema change detection**: Users handle schema migrations via cluster shutdown/restart
- **Trigger mode**: No trigger-based CDC (WAL only)
- **Migration from trigger mode**: This is a new product, not an upgrade
- **NATS message schema changes**: Preserve HarmonyLite's existing format
- **New conflict-resolution strategies**: Use existing LWW, counters, append-only

### 3.4 Operational Constraints

**Schema Migrations**: DDL changes requiring ALTER TABLE, CREATE TABLE, etc. must be performed during maintenance windows with full cluster shutdown. PocketBase handles most schema changes as data changes (collections), so this is rarely needed.

---

## 4. Success Metrics

### 4.1 Functional Correctness

- **Data Fidelity**: Zero data loss or duplication in replication across 3+ node clusters
- **Transaction Atomicity**: 100% of multi-statement transactions replicate atomically
- **Idempotency**: Duplicate OpID detection prevents double-application in all scenarios
- **Ordering Guarantees**: Operations maintain causal ordering across nodes

### 4.2 Reliability

- **Crash Recovery**: 100% successful recovery from parser/node crashes without data loss
- **Network Resilience**: Graceful degradation and recovery from NATS unavailability or partitions
- **Checkpoint Safety**: Zero frames lost during WAL checkpoints in all scenarios

### 4.3 Performance

- **Replication Latency**: Sub-100ms p95 latency for typical PocketBase workloads (< 100 ops/transaction)
- **Overhead**: ≤ 10% CPU and memory overhead compared to non-replicated SQLite
- **Throughput**: Support 1K+ ops/sec sustained write load per node

### 4.4 Community Adoption

- **PocketBase Community**: Successful adoption by PocketBase users requiring HA
- **Documentation Quality**: Users can deploy without support (self-service)
- **Open Source Engagement**: Active issues, PRs, and community contributions

---

## 5. Personas and Use Cases

### 5.1 Persona: PocketBase Application Developer

**Background**: Building a SaaS application with PocketBase as the backend. Needs high availability and zero downtime for production deployments.

**Goals**:
- Deploy PocketBase with automatic failover across multiple regions
- Ensure no data loss during node failures
- Avoid database modifications or complex setup
- Simple sidecar deployment model

**Pain Points**:
- PocketBase doesn't support native clustering
- Existing replication solutions require trigger setup
- Concerns about database integrity with external tools
- Need transparent, trustworthy replication

**User Stories**:
- *As a PocketBase developer, I want to deploy a 3-node HA cluster so that my application survives node failures without downtime*
- *As a PocketBase developer, I want replication that doesn't touch my database so that I can trust data integrity*
- *As a PocketBase developer, I want to migrate away from replication easily so that I'm not locked in*

**Workflows**:
1. Install replication sidecar alongside PocketBase instances
2. Configure NATS connection and replication peers
3. Start cluster and verify replication health
4. Deploy PocketBase application with load balancer
5. Monitor replication metrics via exposed endpoints

### 5.2 Persona: DevOps Engineer

**Background**: Manages infrastructure for applications using SQLite databases. Responsible for availability, backups, and disaster recovery.

**Goals**:
- Deploy highly available SQLite-based services
- Automate failover and recovery
- Monitor replication health and troubleshoot issues
- Minimize operational complexity

**Pain Points**:
- SQLite typically single-node, creating SPOF
- Manual failover processes are error-prone
- Existing solutions require invasive database changes
- Need observability into replication state

**User Stories**:
- *As a DevOps engineer, I want automated failover so that node failures don't require manual intervention*
- *As a DevOps engineer, I want replication metrics so that I can detect issues before data loss occurs*
- *As a DevOps engineer, I want simple configuration so that deployment is automated via IaC*

**Workflows**:
1. Define infrastructure as code (Terraform, K8s manifests)
2. Deploy NATS JetStream cluster for coordination
3. Deploy replicated SQLite services with sidecars
4. Configure monitoring and alerting
5. Test failover scenarios and validate recovery
6. Create runbooks for common operational tasks

### 5.3 Persona: SQLite Application Developer

**Background**: Building distributed systems or edge applications with SQLite. Needs occasional replication or sync without heavyweight databases.

**Goals**:
- Replicate SQLite databases across edge nodes
- Maintain conflict resolution for multi-primary writes
- Keep SQLite's simplicity and embeddability
- Avoid database modification

**Pain Points**:
- Most replication systems are overkill for SQLite
- Trigger-based solutions feel invasive
- Need flexibility to enable/disable replication
- Want clean separation between app and replication logic

**User Stories**:
- *As a SQLite developer, I want multi-primary replication so that edge nodes can operate independently*
- *As a SQLite developer, I want WAL-based CDC so that my database remains pristine*
- *As a SQLite developer, I want conflict resolution strategies so that concurrent writes don't corrupt data*

**Workflows**:
1. Integrate replication library or run as sidecar
2. Configure replication for specific tables/databases
3. Choose conflict resolution strategy per table
4. Enable replication in production environments
5. Disable replication for development/testing

---

## 6. Architecture Overview

### 6.1 High-Level Architecture

```
┌─────────────────────────────────────────────┐
│              Node A                         │
│  ┌──────────────┐      ┌────────────────┐  │
│  │ SQLite DB    │      │ Replication    │  │
│  │ (WAL mode)   │      │ Sidecar        │  │
│  │              │      │                │  │
│  │ *.db         │──────│ WAL Parser     │  │
│  │ *.db-wal     │ read │                │  │
│  └──────────────┘      │ Checkpoint Mgr │  │
│         │              │                │  │
│         │              │ NATS Publisher │  │
│         │              └────────┬───────┘  │
│         │                       │          │
└─────────┼───────────────────────┼──────────┘
          │                       │
          │                       ▼
          │              ┌────────────────┐
          │              │ NATS JetStream │
          │              │   (Cluster)    │
          │              └────────┬───────┘
          │                       │
          │                       ▼
┌─────────┼───────────────────────┼──────────┐
│         │                       │          │
│         │              ┌────────┴───────┐  │
│         │              │ NATS Subscriber│  │
│         │              │                │  │
│         │          ┌───┤ Op Applicator  │  │
│         │          │   │                │  │
│  ┌──────▼──────┐   │   │ Conflict Res.  │  │
│  │ SQLite DB   │◄──┘   │                │  │
│  │ (WAL mode)  │       └────────────────┘  │
│  │             │              Node B        │
│  └─────────────┘                            │
└─────────────────────────────────────────────┘
```

### 6.2 Core Components

**WAL Parser**
- Reads and tails `.db-wal` file via filesystem notifications or efficient polling
- Decodes WAL frames per SQLite WAL format specification
- Detects transaction boundaries (commit frames)
- Buffers operations until commit (hybrid memory + disk)
- Exposes Change events for replication

**Checkpoint Manager**
- Disables SQLite autocheckpoint (`PRAGMA wal_autocheckpoint=0`)
- Tracks parser LSN (Log Sequence Number) position
- Executes coordinated checkpoints after replication confirmed
- Enforces WAL size safety limits with automatic resync fallback
- Persists LSN state for crash recovery

**Replication Engine**
- Receives Change events from parser
- Publishes to NATS JetStream with metadata (OpID, NodeID, LSN, timestamps)
- Subscribes to remote changes from other nodes
- Applies changes to local SQLite database
- Handles conflict resolution (LWW, counters, append-only)

**Deduplication Store**
- Persistent OpID tracking (survives restarts)
- Configurable retention window (minutes to hour)
- Prevents duplicate application of operations
- Graceful handling of replayed events

---

## 7. Technical Requirements

### 7.1 WAL Parser and Change Extractor

**Module**: `walparser` library (Go)

**Features**:

1. **WAL File Monitoring**
   - Read and tail `.db-wal` file using filesystem notifications (fsnotify) or efficient polling
   - Handle WAL file rotation and resets
   - Detect external checkpoints and recover gracefully

2. **Frame Decoding**
   - Decode WAL frames per [SQLite WAL format specification](https://www.sqlite.org/fileformat2.html#walformat)
   - Parse frame headers (page number, db size, salt, checksum)
   - Extract page data and reconstruct logical operations
   - Support SQLite versions 3.x (specify minimum version, e.g., 3.37+)

3. **Transaction Reconstruction**
   - Detect transaction boundaries via commit frames
   - Buffer operations within a transaction
   - Reconstruct: table name, primary key(s), column names, old/new values
   - Maintain transaction atomicity

4. **Transaction Buffering Strategy**
   - **Fast path**: In-memory buffer for transactions ≤ 1000 operations (configurable via `wal.transaction_buffer_memory_limit`)
   - **Slow path**: Disk-based buffer for transactions > limit (unlimited size, writes to temp file)
   - Both paths maintain atomicity and transaction boundaries
   - Auto-cleanup of temp files on commit/rollback

**API Specification**:

```go
// Change represents a logical database operation extracted from WAL
type Change struct {
    OpID        string                 // Unique operation identifier
    NodeID      string                 // Originating node identifier
    LSN         int64                  // Log Sequence Number in WAL
    Table       string                 // Table name
    PrimaryKey  []interface{}          // Primary key value(s)
    Type        ChangeType             // INSERT, UPDATE, DELETE
    Columns     map[string]interface{} // Column values
    Before      map[string]interface{} // Old values (UPDATE/DELETE)
    After       map[string]interface{} // New values (INSERT/UPDATE)
    TransactionID string               // Groups operations in same transaction
    CommitLSN   int64                  // LSN of transaction commit frame
}

type ChangeType int
const (
    ChangeInsert ChangeType = iota
    ChangeUpdate
    ChangeDelete
)

// Parser reads and parses SQLite WAL files
type Parser struct {
    // ... internal state
}

// NewParser creates a WAL parser for the given database
// dbPath: path to SQLite .db file (parser will open .db-wal)
// startLSN: position to start parsing from (0 = beginning, or last_replicated_lsn)
// config: parser configuration options
func NewParser(dbPath string, startLSN int64, config *ParserConfig) (*Parser, error)

// NextTransaction blocks until a complete transaction is available
// Returns a slice of Change events representing the atomic transaction
func (p *Parser) NextTransaction() ([]Change, error)

// Checkpoint triggers a coordinated checkpoint
// Should only be called after confirming replication of all frames up to current LSN
func (p *Parser) Checkpoint(mode CheckpointMode) error

type CheckpointMode int
const (
    CheckpointPassive CheckpointMode = iota  // Non-blocking checkpoint
    CheckpointTruncate                       // Truncate WAL after checkpoint
)

// GetLSN returns the current parser position
func (p *Parser) GetLSN() int64

// Close releases resources
func (p *Parser) Close() error

// ParserConfig provides parser configuration options
type ParserConfig struct {
    TransactionBufferMemoryLimit int    // Max ops in memory before disk spill (default: 1000)
    PollInterval                 time.Duration // WAL polling interval if fsnotify unavailable
    MaxWALSize                   int64  // Max WAL size before forced checkpoint (default: 100MB)
    TempDir                      string // Directory for disk-spilled transactions
}
```

**Edge Cases**:

- **WAL resets**: Detect via salt changes in WAL header; trigger full resync if frames missed
- **Corruption**: Checksum validation; alert and stop parsing on corruption
- **Incompatible WAL versions**: Version check on startup; fail fast with clear error
- **External checkpoints**: Detect via WAL file size shrink; compare db change counter to verify no missed frames
- **Partial transactions**: Only emit complete transactions (buffered until commit frame)
- **Rollback frames**: Discard buffered operations if rollback detected

### 7.2 Checkpoint Management

**Objective**: Ensure WAL frames are never checkpointed before they're successfully replicated.

**Strategy**:

1. **Disable Autocheckpoint**: On startup, execute `PRAGMA wal_autocheckpoint=0` on the SQLite database

2. **Parser-Controlled Checkpointing**:
   - Parser tracks `last_replicated_lsn` (persisted to JetStream metadata + local file)
   - After confirming replication (JetStream acks received), trigger checkpoint
   - Use `PRAGMA wal_checkpoint(PASSIVE)` for non-blocking checkpoint
   - Update `last_checkpointed_lsn` after successful checkpoint

3. **Safety Limits**:
   - Monitor WAL file size continuously
   - If WAL exceeds `max_wal_size` (default: 100MB, configurable):
     - Force `PRAGMA wal_checkpoint(TRUNCATE)` even if parser is behind
     - Mark node as "replication degraded"
     - Trigger automatic snapshot-based resync from healthy peer node
     - Emit alert for monitoring systems

4. **External Checkpoint Detection**:
   - On parser startup or periodically, check WAL file size vs. expected position
   - If WAL is smaller than expected (external checkpoint occurred):
     - Read database change counter from SQLite header
     - Compare with last known change counter
     - If counters match: resume parsing from WAL start
     - If counters differ: frames were missed, trigger full resync

5. **LSN Persistence**:
   - Persist `last_replicated_lsn` to two locations for redundancy:
     - **JetStream consumer metadata**: Primary durable store
     - **Local state file**: Fallback if JetStream temporarily unavailable
   - Persist frequency: After every 100 successfully replicated operations (configurable)
   - On restart: Load LSN from JetStream metadata, fallback to local file

**Configuration**:

```yaml
replication:
  wal:
    autocheckpoint: false  # Always disable
    max_wal_size_mb: 100
    checkpoint_mode: passive  # or truncate
    lsn_persist_interval: 100  # operations
```

### 7.3 Integration with Replication Engine

**Components**:

1. **Change Publisher**:
   - Receives `[]Change` (transaction batch) from `Parser.NextTransaction()`
   - Packages into NATS envelope preserving metadata:
     ```go
     type ReplicationMessage struct {
         OpID          string      `json:"op_id"`
         NodeID        string      `json:"node_id"`
         DBID          string      `json:"db_id"`
         SchemaVersion int         `json:"schema_version"`
         WallTime      time.Time   `json:"wall_time"`
         LamportClock  int64       `json:"lamport"`
         Transaction   []Change    `json:"transaction"`  // Atomic batch
     }
     ```
   - Publishes to NATS JetStream subject (e.g., `replication.{db_id}.changes`)
   - Waits for JetStream ack before updating `last_replicated_lsn`

2. **Change Subscriber**:
   - Subscribes to NATS JetStream for remote changes
   - Receives `ReplicationMessage` from other nodes
   - Checks deduplication store for OpID
   - Applies transaction atomically to local SQLite:
     - Begin SQLite transaction
     - Apply all changes in order
     - Commit transaction
     - If any operation fails: rollback and retry with conflict resolution

3. **Backfill on Startup**:
   - On service startup, read `last_replicated_lsn` from JetStream consumer metadata
   - Initialize parser with `NewParser(dbPath, last_replicated_lsn, config)`
   - Parser resumes from last known position
   - If `last_replicated_lsn` is stale or unavailable: start from WAL beginning or trigger full resync

4. **Idempotency**:
   - Every `Change` has unique `OpID` (format: `{node_id}:{lsn}:{timestamp}`)
   - Before applying, check deduplication store
   - If OpID exists: skip gracefully (log at DEBUG level)
   - If OpID new: apply operation and store OpID with timestamp
   - Periodically clean up OpIDs older than retention window (default: 1 hour)

### 7.4 Deduplication Store

**Implementation**: Persistent key-value store (options: embedded SQLite table, BoltDB, BadgerDB)

**Schema**:
```
OpID (string) -> Timestamp (int64)
```

**Operations**:
- `Exists(opID string) bool`: Check if OpID has been seen
- `Add(opID string, timestamp int64)`: Record new OpID
- `Cleanup(olderThan time.Time)`: Remove OpIDs older than threshold
- `Count() int`: Return number of tracked OpIDs (for monitoring)

**Configuration**:
```yaml
replication:
  deduplication:
    store_type: sqlite  # or boltdb, badgerdb
    retention_duration: 1h
    cleanup_interval: 5m
```

**Design Considerations**:
- Store should survive process restarts (persistent)
- Configurable retention window (default: 1 hour, longer for slower networks)
- Automatic cleanup to prevent unbounded growth
- Fast lookups (hash-based index)

### 7.5 Configuration Schema

**Complete YAML Configuration**:

```yaml
replication:
  # Node identification
  node_id: "node-1"
  db_id: "pocketbase-main"

  # NATS connection
  nats:
    urls:
      - "nats://nats-1:4222"
      - "nats://nats-2:4222"
    credentials_file: "/etc/replication/nats.creds"

  # WAL parser configuration
  wal:
    db_path: "/data/pocketbase.db"
    start_lsn: 0  # or "auto" to resume from last position
    transaction_buffer_memory_limit: 1000
    poll_interval: 100ms
    max_wal_size_mb: 100
    temp_dir: "/tmp/replication"

  # Checkpoint management
  checkpoint:
    mode: passive  # or truncate
    lsn_persist_interval: 100
    state_file: "/var/lib/replication/lsn_state.json"

  # Deduplication
  deduplication:
    store_type: sqlite
    store_path: "/var/lib/replication/dedup.db"
    retention_duration: 1h
    cleanup_interval: 5m

  # Conflict resolution
  conflict_resolution:
    default_strategy: lww  # last-write-wins
    strategies:
      table_name:
        strategy: counter  # or append_only

  # Observability
  metrics:
    enabled: true
    port: 9090
  logging:
    level: info  # debug, info, warn, error
    format: json
```

---

## 8. Error Handling & Resilience

Error handling is prioritized based on likelihood and impact:

### Priority 1: Network Issues (HIGHEST)

**8.1.1 NATS/JetStream Unavailable**

**Scenario**: NATS server is down, unreachable, or JetStream is overloaded.

**Impact**: Cannot publish changes or receive remote changes; replication stalls.

**Handling**:
- **Detection**: Connection loss events, publish timeout, subscribe errors
- **Response**:
  - Continue parsing WAL and buffer changes locally (up to configurable limit, e.g., 10K operations)
  - Use disk-based buffer if memory limit exceeded (WAL acts as natural buffer)
  - Retry NATS connection with exponential backoff (initial: 1s, max: 30s)
  - Emit `harmonylite_nats_unavailable` metric and log WARNING
- **Recovery**:
  - On reconnection, resume publishing from buffered changes
  - JetStream's durable consumers ensure no message loss
  - Monitor `harmonylite_nats_reconnections` counter for alerting

**Configuration**:
```yaml
nats:
  retry_attempts: -1  # infinite
  retry_initial_backoff: 1s
  retry_max_backoff: 30s
  local_buffer_size: 10000
```

**8.1.2 Network Partitions**

**Scenario**: Network split isolates nodes; some can reach NATS, others cannot.

**Impact**: Split-brain risk; nodes continue accepting writes independently.

**Handling**:
- **Detection**: NATS connection status, peer heartbeat loss
- **Response**:
  - Each partition continues operating independently
  - Nodes track their own Lamport clocks
  - On partition heal: NATS delivers all buffered messages
  - Conflict resolution (LWW, counters) reconciles divergent writes
- **Reconciliation**:
  - Lamport clocks + wall time determine event ordering
  - Apply conflict resolution strategy per table
  - Log conflicts at INFO level for audit
  - Expose `harmonylite_conflicts_resolved` metric

**Testing**: Simulate partitions using `iptables` or chaos engineering tools (e.g., Toxiproxy).

**8.1.3 Slow/Degraded Network**

**Scenario**: High latency or packet loss slows replication.

**Impact**: Increased replication lag, potential WAL growth.

**Handling**:
- **Detection**: Monitor `harmonylite_replication_lag_seconds` metric
- **Response**:
  - Continue operating; NATS handles buffering
  - If lag exceeds threshold (e.g., 60s): emit WARN alert
  - If WAL exceeds `max_wal_size`: trigger checkpoint + resync (see section 7.2)
- **Alerting**:
  - `harmonylite_replication_lag_seconds > 60` for 5 minutes
  - `harmonylite_wal_size_bytes > 100MB`

### Priority 2: Node Crashes (HIGH)

**8.2.1 WAL Parser Crash**

**Scenario**: Parser process crashes or panics mid-parsing.

**Impact**: Replication stops; WAL continues growing.

**Handling**:
- **Detection**: Process monitor (systemd, Docker health check, K8s liveness probe)
- **Response**:
  - Process manager restarts parser automatically
  - On restart:
    - Load `last_replicated_lsn` from JetStream metadata or local state file
    - Resume parsing from last known position
    - Replication continues without data loss
- **Recovery Time**: < 5 seconds (restart + resume)
- **Testing**: Use `SIGKILL` to simulate unexpected crash

**8.2.2 SQLite/Application Crash**

**Scenario**: SQLite process or application crashes during write operation.

**Impact**: WAL may contain partial transaction; parser state may be inconsistent.

**Handling**:
- **Detection**: Process monitor detects crash
- **Response**:
  - On restart:
    - SQLite's WAL recovery runs automatically (restores consistent state)
    - Parser resumes from `last_replicated_lsn`
    - Any partial transaction in WAL is either complete (commit frame present) or absent (rollback)
    - Parser only emits complete transactions (commit frame required)
- **Guarantee**: No partial transactions replicated due to commit frame requirement
- **Testing**: Use `SIGKILL` during active write; verify recovery

**8.2.3 Full Node Crash**

**Scenario**: Entire node (hardware, VM, container) fails.

**Impact**: Node offline; cluster continues with remaining nodes.

**Handling**:
- **Detection**: Orchestrator (K8s, systemd) detects node down
- **Response**:
  - Remaining nodes continue replicating among themselves
  - NATS JetStream buffers messages for crashed node's consumer
  - On node recovery:
    - Parser resumes from `last_replicated_lsn` (from JetStream metadata)
    - Applies all buffered changes from other nodes
    - Catches up to cluster state
- **Recovery**:
  - If LSN is too far behind or JetStream retention exceeded: trigger full snapshot resync
  - Monitor `harmonylite_sync_full_count` metric
- **Testing**: Simulate node failure; verify automatic recovery

### Priority 3: Data Issues (MEDIUM)

**8.3.1 Large BLOBs**

**Scenario**: WAL contains very large BLOB data (e.g., multi-MB images).

**Impact**: Memory pressure, slow parsing, large NATS messages.

**Handling**:
- **Detection**: Monitor `Change` size before publishing
- **Response**:
  - If `Change` exceeds threshold (e.g., 10MB, configurable):
    - Log WARNING with table/column info
    - Option 1: Chunk BLOB across multiple NATS messages (implement BLOB streaming protocol)
    - Option 2: Exclude BLOB from replication; trigger full snapshot for that row
    - Option 3: Fail replication and alert operator (safest default)
  - Configuration:
    ```yaml
    wal:
      max_change_size_mb: 10
      large_blob_strategy: fail  # fail, chunk, snapshot
    ```
- **Mitigation**: Document recommended BLOB handling strategies (e.g., store BLOBs in object storage, replicate references)

**8.3.2 Rapid Write Bursts**

**Scenario**: Application performs bulk insert (e.g., 100K rows in seconds).

**Impact**: Parser falls behind; WAL grows rapidly; memory pressure.

**Handling**:
- **Detection**: Monitor `harmonylite_parser_lag_lsn` (distance between WAL end and parser position)
- **Response**:
  - Parser continues processing as fast as possible
  - Disk-based transaction buffering handles large transactions (no memory limit)
  - If WAL exceeds `max_wal_size`: trigger checkpoint + resync (see section 7.2)
  - Rate limit NATS publishing if JetStream is overwhelmed (backpressure)
- **Configuration**:
  ```yaml
  wal:
    max_concurrent_transactions: 10  # Limit concurrent transaction processing
  nats:
    publish_rate_limit: 1000  # messages per second
  ```
- **Testing**: Simulate bulk inserts; measure parser throughput and latency

**8.3.3 Clock Skew**

**Scenario**: System clocks differ significantly across nodes.

**Impact**: Timestamp-based ordering (wall_time) may be incorrect; LWW conflict resolution affected.

**Handling**:
- **Detection**: Compare wall_time in `ReplicationMessage` with local time; alert if skew > threshold (e.g., 5s)
- **Response**:
  - Primary ordering: Use Lamport clocks (logical time, clock-skew immune)
  - Secondary ordering: Use wall_time only as tiebreaker
  - Document requirement for NTP/chrony on all nodes
  - Emit `harmonylite_clock_skew_seconds` metric
- **Alerting**: `harmonylite_clock_skew_seconds > 5` for 1 minute
- **Mitigation**: Configure NTP in deployment documentation

### Priority 4: Storage Issues (LOW)

**8.4.1 Disk Full During Parsing**

**Scenario**: Disk space exhausted while writing disk-based transaction buffer.

**Impact**: Parser cannot buffer large transaction; replication stalls.

**Handling**:
- **Detection**: `write()` syscall returns `ENOSPC`; monitor disk usage
- **Response**:
  - Fail current transaction parsing with error
  - Mark node as "replication failed"
  - Emit CRITICAL alert: `harmonylite_disk_full`
  - Operator intervention required: free disk space or expand volume
  - On disk space available: resume parsing from last successful transaction
- **Prevention**: Monitor disk usage proactively; alert at 80% full

**8.4.2 Corrupted WAL File**

**Scenario**: WAL file contains invalid data (checksum mismatch, invalid frame format).

**Impact**: Parser cannot decode frames; replication stalls.

**Handling**:
- **Detection**: Checksum validation fails; frame decode error
- **Response**:
  - Log ERROR with LSN position and error details
  - Stop parsing to prevent incorrect data replication
  - Emit `harmonylite_wal_corruption_detected` metric
  - Operator intervention required:
    - Option 1: Force SQLite checkpoint to flush WAL; restart parser
    - Option 2: Trigger full snapshot resync from healthy peer
  - Do NOT continue parsing corrupted data
- **Testing**: Manually corrupt WAL file bytes; verify parser halts safely

**8.4.3 Filesystem Errors**

**Scenario**: I/O errors reading WAL file (hardware failure, NFS issues, etc.).

**Impact**: Parser cannot read WAL; replication stalls.

**Handling**:
- **Detection**: `read()` syscall returns `EIO` or timeout (NFS)
- **Response**:
  - Retry read with exponential backoff (transient errors)
  - After N retries (e.g., 5): fail and alert
  - Emit `harmonylite_io_errors` metric
  - Operator intervention: check filesystem health, repair if needed
- **Prevention**: Use local SSD for WAL files; avoid NFS for database files

---

## 9. Non-Functional Requirements

### 9.1 Compatibility

- **Operating Systems**: Linux (primary), macOS (development), Windows (community support)
- **Go Version**: 1.21+ (use latest stable)
- **SQLite Version**: 3.37+ (for WAL2 support if needed; document minimum version)
- **NATS Version**: 2.9+ with JetStream enabled

### 9.2 Performance

- **Replication Latency**:
  - Target: p50 < 50ms, p95 < 100ms, p99 < 200ms (for typical PocketBase workloads)
  - Measured: Time from WAL write to change applied on remote node
  - Monitor: `harmonylite_replication_latency_seconds` histogram

- **Throughput**:
  - Target: 1K ops/sec sustained per node (single writer)
  - Target: 10K ops/sec burst for short duration (< 1 minute)
  - Scale: Linear with number of nodes (each node parses own WAL independently)

- **Resource Overhead**:
  - CPU: ≤ 10% overhead compared to non-replicated SQLite (measured under 1K ops/sec load)
  - Memory: ≤ 100MB base + transaction buffer (configurable)
  - Disk: Temporary transaction buffer (only for large transactions)
  - Network: ~2x write bandwidth (publish to NATS, receive from peers)

### 9.3 Scalability

- **Cluster Size**: 3-7 nodes recommended (higher node counts increase NATS message fan-out)
- **Database Size**: Tested up to 100GB database (parser performance independent of DB size)
- **WAL Size**: Tested up to 1GB WAL (with checkpoint coordination)
- **Transaction Size**: Unlimited (disk-based buffering)
- **Table Count**: Unlimited (parser doesn't load schema into memory)

### 9.4 Reliability

- **Uptime**: 99.9% availability per node (3-node cluster: 99.99% cluster availability)
- **Data Loss**: Zero data loss guarantee with ≥ 2 healthy nodes
- **Crash Recovery**: < 5 seconds to resume replication after parser crash
- **Failover**: Automatic; no manual intervention required

### 9.5 Security

- **Principle of Least Privilege**: Parser runs with read-only access to WAL file
- **No Database Writes**: Parser never writes to `.db`, `.db-wal`, or `.db-shm` files
- **NATS Authentication**: Support NATS credentials file (`.creds`) and TLS
- **TLS**: Support TLS for NATS connections (configurable, not required)
- **Secrets**: Never log sensitive data (credentials, NATS tokens)

### 9.6 Observability

**9.6.1 Metrics** (Prometheus format, exposed on `/metrics` endpoint)

Core Metrics:
- `harmonylite_cdc_last_lsn`: Last LSN parsed from WAL (gauge)
- `harmonylite_cdc_last_replicated_lsn`: Last LSN successfully replicated to NATS (gauge)
- `harmonylite_cdc_last_checkpointed_lsn`: Last LSN checkpointed to database (gauge)
- `harmonylite_replication_lag_seconds`: Time since last parsed change (gauge)
- `harmonylite_parser_lag_lsn`: Distance between WAL end and parser position (gauge)
- `harmonylite_wal_size_bytes`: Current WAL file size (gauge)
- `harmonylite_cdc_parse_errors_total`: Count of parse errors (counter)
- `harmonylite_replication_latency_seconds`: Replication latency histogram
- `harmonylite_operations_total`: Operations processed, by type (counter, labels: insert/update/delete)
- `harmonylite_transactions_total`: Transactions processed (counter)
- `harmonylite_conflicts_resolved_total`: Conflicts resolved, by strategy (counter, labels: lww/counter/append)
- `harmonylite_nats_unavailable`: NATS connection status (gauge, 0=connected, 1=unavailable)
- `harmonylite_nats_reconnections_total`: NATS reconnection count (counter)
- `harmonylite_dedup_store_size`: Number of OpIDs in deduplication store (gauge)
- `harmonylite_sync_full_count`: Full resync count (counter)

Error Metrics:
- `harmonylite_disk_full`: Disk full condition (gauge, 0/1)
- `harmonylite_wal_corruption_detected`: WAL corruption detected (gauge, 0/1)
- `harmonylite_io_errors_total`: Filesystem I/O errors (counter)
- `harmonylite_clock_skew_seconds`: Clock skew vs. peers (gauge)

**9.6.2 Logging**

Levels:
- **DEBUG**: WAL frame details, OpID tracking, checkpoint decisions
- **INFO**: Startup, replication events, checkpoints, recoveries, conflict resolutions
- **WARN**: Replication lag, clock skew, retry attempts, degraded state
- **ERROR**: Parse errors, replication failures, NATS unavailability, disk full

Format: Structured JSON logs (e.g., using `zerolog` or `zap`)

Example:
```json
{
  "level": "info",
  "ts": "2024-01-15T10:30:45.123Z",
  "msg": "checkpoint completed",
  "node_id": "node-1",
  "lsn_before": 1500,
  "lsn_after": 2000,
  "wal_size_before_mb": 45,
  "wal_size_after_mb": 5,
  "duration_ms": 234
}
```

**9.6.3 Alerting Thresholds** (recommended)

Critical:
- `harmonylite_disk_full == 1` for > 1 minute
- `harmonylite_wal_corruption_detected == 1`
- `harmonylite_cdc_parse_errors_total` rate > 1/sec for > 5 minutes
- `harmonylite_nats_unavailable == 1` for > 5 minutes

Warning:
- `harmonylite_replication_lag_seconds > 60` for > 5 minutes
- `harmonylite_parser_lag_lsn > 10000` for > 10 minutes
- `harmonylite_wal_size_bytes > 100MB` for > 10 minutes
- `harmonylite_clock_skew_seconds > 5` for > 1 minute

**9.6.4 Health Checks**

HTTP endpoints:
- `GET /health/live`: Liveness probe (returns 200 if process running)
- `GET /health/ready`: Readiness probe (returns 200 if parser running and NATS connected)
- `GET /health/status`: Detailed status JSON (LSN positions, lag, NATS status)

Example `/health/status` response:
```json
{
  "status": "healthy",
  "node_id": "node-1",
  "parser": {
    "running": true,
    "last_lsn": 2500,
    "last_replicated_lsn": 2450,
    "lag_lsn": 50,
    "lag_seconds": 0.5
  },
  "nats": {
    "connected": true,
    "url": "nats://nats-1:4222"
  },
  "wal": {
    "size_bytes": 15728640,
    "last_checkpoint_lsn": 2000
  }
}
```

### 9.7 Disaster Recovery

**9.7.1 Backup Strategy**

- **WAL Files**: Not needed for backups (ephemeral, checkpointed to DB)
- **Database Files**: Standard SQLite backup methods:
  - Online backup: Use SQLite backup API (`.backup` command or `sqlite3_backup_*`)
  - Offline backup: Copy `.db` file when application stopped
  - Replication-aware: Any node's `.db` file is a valid backup (eventually consistent)

**9.7.2 Restore Procedure**

1. Stop replication on all nodes
2. Restore `.db` file on primary node from backup
3. Delete `.db-wal` and `.db-shm` files (start fresh WAL)
4. Start replication on primary node
5. Trigger full resync on secondary nodes (copy DB from primary)
6. Resume normal operation

**9.7.3 Full Resync Procedure**

Triggered when:
- Node falls too far behind (LSN gap too large)
- WAL corruption detected
- Operator manually requests resync

Steps:
1. Stop local parser and replication
2. Request snapshot from healthy peer node
3. Perform SQLite backup from peer to local node (over NATS or HTTP)
4. Reset `last_replicated_lsn` to peer's current LSN
5. Restart parser from new LSN
6. Resume replication

Configuration:
```yaml
replication:
  resync:
    auto_trigger_threshold: 100000  # LSN gap triggering automatic resync
    snapshot_method: nats  # or http
    snapshot_compress: true
```

---

## 10. Testing and Validation

### 10.1 Unit Tests

**Scope**: Individual components in isolation.

**Test Cases**:

1. **WAL Frame Decoding**:
   - Valid frame decoding (header + payload)
   - Checksum validation (valid/invalid)
   - Frame type detection (commit, abort, data)
   - Boundary conditions (empty frame, maximum size)

2. **Transaction Reconstruction**:
   - Single-operation transaction
   - Multi-operation transaction (10, 100, 1000 ops)
   - Rollback handling (buffered ops discarded)
   - Partial transaction (no commit frame yet)

3. **Checkpoint Logic**:
   - LSN persistence (save/load)
   - Checkpoint decision logic (safe to checkpoint vs. not)
   - External checkpoint detection

4. **Deduplication**:
   - OpID add/exists operations
   - Cleanup of old OpIDs
   - Persistence across restart

**Coverage Target**: 80%+ line coverage for core parser and replication logic.

### 10.2 Integration Tests

**Scope**: Multi-node clusters with NATS and SQLite.

**Test Cases**:

1. **Basic Replication**:
   - 3-node cluster, write on node1, verify replicated to node2 and node3
   - Measure replication latency

2. **Transaction Atomicity**:
   - Multi-operation transaction on node1
   - Verify all operations applied atomically on node2 (all or nothing)

3. **Conflict Resolution**:
   - Concurrent writes to same row on node1 and node2
   - Verify LWW conflict resolution (last write wins based on Lamport clock)
   - Verify counter conflict resolution (sum of counter increments)

4. **Node Crash Recovery**:
   - Write 1000 ops on node1
   - Kill node2 parser mid-replication
   - Restart node2 parser
   - Verify node2 catches up and applies all 1000 ops

5. **NATS Unavailability**:
   - Stop NATS server mid-replication
   - Continue writing on node1
   - Restart NATS
   - Verify node1 resumes publishing and node2 receives all changes

6. **Large Transactions**:
   - Transaction with 10,000 operations
   - Verify disk-based buffering activated
   - Verify transaction applied atomically on peers

7. **Checkpoint Coordination**:
   - Disable autocheckpoint
   - Write 100MB of data (exceeding `max_wal_size`)
   - Verify forced checkpoint triggered
   - Verify no frames lost (resync if needed)

**Environment**: Docker Compose with 3 app nodes, 3 NATS nodes, monitoring stack.

**Duration**: ~30 minutes for full integration test suite.

### 10.3 Chaos Engineering Tests

**Scope**: Network partitions, random failures, Byzantine faults.

**Test Cases**:

1. **Network Partitions**:
   - Use `iptables` or Toxiproxy to isolate node1 from NATS
   - Continue writes on node1 (local buffering)
   - Heal partition after 60 seconds
   - Verify node1 republishes buffered changes

2. **Random Node Crashes**:
   - Randomly kill parser or SQLite process every 10-30 seconds
   - Continue write workload across all nodes
   - After 10 minutes, verify data consistency across all nodes

3. **Clock Skew**:
   - Set node1 clock 10 seconds ahead
   - Perform concurrent writes on node1 and node2
   - Verify Lamport clocks resolve ordering correctly

4. **Slow Network**:
   - Add 500ms latency and 5% packet loss between nodes
   - Measure replication lag
   - Verify eventual consistency

**Tools**: Chaos Mesh, Pumba, Toxiproxy, Jepsen (if resources available).

### 10.4 Performance Benchmarks

**Scope**: Measure throughput, latency, resource usage under various loads.

**Benchmarks**:

1. **Write Throughput**:
   - Single-node: Measure ops/sec with replication enabled vs. disabled
   - Multi-node: Measure cluster-wide ops/sec with 3 nodes

2. **Replication Latency**:
   - Write on node1, measure time until applied on node2
   - Plot p50, p95, p99 latency under 100, 500, 1000 ops/sec load

3. **Large Transaction Performance**:
   - Transaction sizes: 10, 100, 1K, 10K operations
   - Measure parsing time, buffering time, replication time

4. **Checkpoint Performance**:
   - WAL sizes: 10MB, 50MB, 100MB, 500MB
   - Measure checkpoint duration (PASSIVE vs. TRUNCATE mode)

5. **Resource Usage**:
   - Measure CPU, memory, disk I/O under 1K ops/sec sustained load
   - Compare with non-replicated SQLite baseline

**Reporting**: Publish benchmark results as markdown table + graphs in GitHub repo.

### 10.5 Fuzz Testing

**Scope**: Feed parser with malformed/random WAL data to uncover crashes or panics.

**Tool**: Go's built-in `go test -fuzz` (Go 1.18+)

**Targets**:
- WAL frame decoding
- Transaction reconstruction
- OpID parsing

**Duration**: Run for 1 hour+ as part of CI/CD (extended fuzzing on weekends).

### 10.6 PocketBase-Specific Tests

**Scope**: Validate integration with real PocketBase deployments.

**Test Cases**:

1. **PocketBase Startup**:
   - Deploy PocketBase with replication sidecar
   - Verify replication activates automatically
   - Check no triggers created in PocketBase database

2. **Collections CRUD**:
   - Create collection via PocketBase Admin UI
   - Add/edit/delete records
   - Verify changes replicated across nodes
   - Access any node via load balancer; verify consistent reads

3. **File Uploads**:
   - Upload file to record on node1
   - Verify file metadata replicated (file itself may be in object storage)

4. **Realtime Subscriptions**:
   - Subscribe to PocketBase realtime API on node2
   - Create record on node1
   - Verify realtime event received on node2

5. **Admin UI Failover**:
   - Access PocketBase Admin UI via load balancer
   - Kill node1 mid-session
   - Verify session continues on node2 (eventual consistency)

**Environment**: Real PocketBase instances with replication sidecars in Docker Compose or K8s.

---

## 11. Milestones

No fixed timeline; tasks are completed incrementally with AI assistance. Progress tracked via GitHub Issues/Project Board.

### Phase 1: Core WAL Parser

**Tasks**:
- [ ] Design `walparser` package structure
- [ ] Implement WAL file reading and frame decoding
- [ ] Implement transaction boundary detection (commit frames)
- [ ] Implement hybrid transaction buffering (memory + disk)
- [ ] Unit tests for frame decoding and transaction reconstruction
- [ ] Fuzz tests for frame decoding

**Deliverables**:
- `walparser` package with `Parser` API
- Unit tests with 80%+ coverage
- Benchmarks for parsing throughput

### Phase 2: Checkpoint Management

**Tasks**:
- [ ] Implement LSN persistence (JetStream metadata + local file)
- [ ] Implement checkpoint coordination (disable autocheckpoint)
- [ ] Implement coordinated checkpoint execution (PASSIVE mode)
- [ ] Implement external checkpoint detection
- [ ] Implement safety limits (max WAL size, forced checkpoint)
- [ ] Unit tests for checkpoint logic

**Deliverables**:
- Checkpoint manager module
- LSN persistence working end-to-end
- Tests for checkpoint coordination

### Phase 3: Replication Engine Integration

**Tasks**:
- [ ] Integrate `walparser` with NATS publisher
- [ ] Implement `ReplicationMessage` envelope format
- [ ] Implement change subscriber and applicator
- [ ] Implement deduplication store (persistent OpID tracking)
- [ ] Implement backfill on startup (resume from LSN)
- [ ] Integration tests for basic replication

**Deliverables**:
- End-to-end replication working (write on node1, read on node2)
- Deduplication preventing duplicate application
- Integration tests passing

### Phase 4: Error Handling & Resilience

**Tasks**:
- [ ] Implement NATS unavailability handling (retry, backoff, local buffering)
- [ ] Implement parser crash recovery (LSN resume)
- [ ] Implement full resync procedure (snapshot-based)
- [ ] Implement network partition handling (Lamport clocks, conflict resolution)
- [ ] Implement large transaction handling (disk spill)
- [ ] Chaos engineering tests

**Deliverables**:
- Robust error handling for all Priority 1-3 scenarios
- Chaos tests validating resilience
- Runbooks for operational failures

### Phase 5: Observability & Monitoring

**Tasks**:
- [ ] Implement Prometheus metrics (all metrics from section 9.6.1)
- [ ] Implement structured logging (JSON, zerolog)
- [ ] Implement health check endpoints (`/health/*`)
- [ ] Create Grafana dashboard for monitoring
- [ ] Document alerting thresholds and runbooks

**Deliverables**:
- Metrics exposed on `/metrics` endpoint
- Health checks working in K8s/Docker
- Grafana dashboard JSON
- Alerting documentation

### Phase 6: Testing & Validation

**Tasks**:
- [ ] Expand integration test suite (all scenarios from section 10.2)
- [ ] Implement performance benchmarks (section 10.4)
- [ ] Run extended fuzz testing (1+ hour per target)
- [ ] PocketBase-specific tests (section 10.6)
- [ ] Load testing with realistic PocketBase workloads

**Deliverables**:
- Comprehensive test suite (unit + integration + chaos)
- Benchmark results published
- PocketBase integration validated

### Phase 7: Documentation & Release

**Tasks**:
- [ ] Write deployment guide (Docker Compose, Kubernetes, systemd)
- [ ] Write PocketBase integration guide
- [ ] Write configuration reference
- [ ] Write troubleshooting guide and runbooks
- [ ] Write API documentation (GoDoc)
- [ ] Create example configurations
- [ ] Prepare v1.0.0 release notes
- [ ] Open source repository (GitHub)

**Deliverables**:
- Complete documentation site (GitHub Pages or similar)
- Example deployments (Docker Compose, K8s manifests)
- v1.0.0 release on GitHub
- Announcement to PocketBase community

---

## 12. Dependencies and Risks

### 12.1 Dependencies

**External Dependencies**:
- **SQLite**: WAL format stability (risk: low; SQLite highly stable)
  - Mitigation: Target specific SQLite version (e.g., 3.37+); test against version matrix (3.37, 3.40, 3.43)
- **NATS JetStream**: Availability and performance
  - Mitigation: NATS is mature; extensive testing in production; community support
- **Go standard library**: Filesystem APIs (`os`, `io`, `syscall`)
  - Mitigation: Go's filesystem APIs are stable and well-tested
- **Third-party libraries**: Consider using existing SQLite parsing libraries (e.g., `go-sqlite3`, WAL format parsers)
  - Mitigation: Evaluate license compatibility; prefer writing custom parser for control

**Internal Dependencies**:
- HarmonyLite's replication engine and message formats
  - Mitigation: Fork HarmonyLite codebase; maintain compatibility in message envelope

### 12.2 Risks

**12.2.1 SQLite WAL Format Changes**

**Risk**: Future SQLite versions change WAL format, breaking parser.

**Likelihood**: Low (SQLite maintains backward compatibility rigorously).

**Impact**: High (parser stops working).

**Mitigation**:
- Document supported SQLite versions explicitly (e.g., 3.37-3.45)
- Version-gate parser: check SQLite version on startup; fail if unsupported
- Maintain test matrix with multiple SQLite versions in CI
- Monitor SQLite release notes for WAL format changes
- Community engagement: Watch SQLite mailing list for advance notice

**12.2.2 Parser Performance Under Heavy Load**

**Risk**: WAL parser cannot keep up with write-heavy workloads; replication lag grows unbounded.

**Likelihood**: Medium (depends on workload characteristics).

**Impact**: High (replication lag, WAL growth, potential data loss if checkpoint forced).

**Mitigation**:
- Optimize parser for performance:
  - Minimize memory allocations
  - Batch operations
  - Use efficient data structures (zero-copy frame parsing)
- Benchmark early and often
- Implement backpressure: slow down writes if parser falls too far behind (configurable)
- Document performance limits and sizing guidelines
- Provide tuning guide for high-throughput scenarios

**12.2.3 Network Partition Edge Cases**

**Risk**: Complex network partition scenarios (split-brain, partial partition) lead to data inconsistency.

**Likelihood**: Medium (network partitions are common in distributed systems).

**Impact**: High (data divergence, conflict resolution failures).

**Mitigation**:
- Use Lamport clocks as primary ordering mechanism (clock-skew immune)
- Implement robust conflict resolution (LWW with Lamport + wall_time tiebreaker)
- Extensive chaos engineering tests (partition scenarios)
- Document network requirements (stable connectivity, NTP)
- Provide guidance on partition handling in operations runbook

**12.2.4 Concurrent Writers to SQLite**

**Risk**: Multiple processes writing to same SQLite database bypassing replication (not supported by design).

**Likelihood**: Low (SQLite's WAL mode supports multiple readers, single writer; enforce single writer).

**Impact**: High (changes missed by parser).

**Mitigation**:
- Document clearly: only one application instance per node should write to replicated database
- Detect concurrent writers: monitor database lock contention
- SQLite's BUSY errors will naturally block concurrent writers
- Recommend using application-level locks (e.g., leader election via NATS)

**12.2.5 Community Adoption**

**Risk**: PocketBase community doesn't adopt the solution; low usage.

**Likelihood**: Medium (depends on need and quality of solution).

**Impact**: Medium (lower return on investment, less feedback for improvement).

**Mitigation**:
- Engage PocketBase community early: share roadmap, gather feedback
- Provide excellent documentation and examples
- Offer support via GitHub Discussions or Discord
- Publish benchmarks and success stories
- Contribute to PocketBase discussions around HA/clustering
- Consider contributing replication support directly to PocketBase upstream

**12.2.6 Filesystem Edge Cases**

**Risk**: Exotic filesystems (NFS, network-attached storage) have different WAL behavior; parser fails.

**Likelihood**: Medium (users may deploy on NFS for shared storage).

**Impact**: Medium (parser errors, replication stalls).

**Mitigation**:
- Document supported filesystems: recommend local SSD/NVMe
- Warn against NFS or network storage for SQLite databases
- Test on common filesystems: ext4, XFS, APFS, NTFS
- Implement robust error handling for I/O errors (retries with backoff)

---

## 13. Acceptance Criteria

### 13.1 Functional Requirements

✅ **Zero Database Modification**:
- Running replication does not create triggers, views, or modify database schema
- `SELECT name FROM sqlite_master WHERE type='trigger'` returns no replication-related triggers
- Database files can be copied/used independently without replication artifacts

✅ **Transaction Atomicity**:
- Multi-operation transactions replicate atomically (all ops applied together on peers)
- No partial transaction application (verified via integration tests)

✅ **Data Fidelity**:
- Zero data loss across 3-node cluster under standard workload (verified via integration tests)
- Zero data duplication (deduplication store prevents double-application)
- 100% of operations replicated correctly (insert/update/delete)

✅ **Checkpoint Coordination**:
- WAL autocheckpoint disabled (`PRAGMA wal_autocheckpoint=0`)
- Parser-controlled checkpoints occur only after replication confirmed
- LSN persisted to JetStream metadata and local file
- Recovery after checkpoint successful (parser resumes correctly)

✅ **Network Resilience**:
- NATS unavailability handled gracefully (local buffering, automatic reconnection)
- Cluster continues operating during network partitions; reconverges on heal
- No data loss or duplication during partition/heal cycles

✅ **Crash Recovery**:
- Parser crash recovery within 5 seconds (process restart + resume from LSN)
- Node crash recovery successful (applies buffered changes on restart)
- Zero data loss after any crash scenario

### 13.2 Performance Requirements

✅ **Replication Latency**:
- p95 latency < 100ms for typical PocketBase workloads (< 100 ops/txn)
- Measured and documented in benchmark report

✅ **Throughput**:
- Sustains 1K ops/sec per node under standard load
- Tested and validated via performance benchmarks

✅ **Resource Overhead**:
- CPU overhead ≤ 10% compared to non-replicated SQLite
- Memory usage ≤ 100MB base + transaction buffers
- Measured and documented in benchmark report

### 13.3 Operational Requirements

✅ **Configuration**:
- Complete YAML configuration schema documented
- Sensible defaults for all parameters
- Example configurations provided (Docker Compose, K8s, systemd)

✅ **Observability**:
- All metrics from section 9.6.1 exposed on `/metrics` endpoint
- Health check endpoints functional (`/health/live`, `/health/ready`, `/health/status`)
- Structured JSON logging working with appropriate levels

✅ **Documentation**:
- Deployment guide complete (Docker Compose, Kubernetes, systemd)
- PocketBase integration guide with step-by-step instructions
- Configuration reference with all parameters documented
- Troubleshooting guide and runbooks for common failures
- API documentation (GoDoc) published

### 13.4 Testing Requirements

✅ **Test Coverage**:
- Unit tests: 80%+ line coverage for core parser and replication logic
- Integration tests: All scenarios from section 10.2 passing
- Chaos tests: Random failures and partition scenarios validated
- Performance benchmarks: Results published and meeting targets

✅ **PocketBase Integration**:
- Real PocketBase deployment tested (3-node cluster)
- Collections CRUD operations replicated successfully
- Admin UI accessible via load balancer with failover

### 13.5 Release Readiness

✅ **Open Source**:
- Repository public on GitHub
- License file present (MIT or Apache 2.0)
- Contribution guidelines (CONTRIBUTING.md)
- Code of conduct (CODE_OF_CONDUCT.md)

✅ **Release**:
- v1.0.0 release tagged on GitHub
- Release notes published
- Binary releases for Linux, macOS, Windows (or Docker image)
- Announcement posted to PocketBase community

---

## Next Steps

1. **Set up development environment**:
   - Fork HarmonyLite repository (or start from scratch with reference to HarmonyLite)
   - Set up Go project structure (`walparser`, `replication`, `checkpoint` packages)
   - Configure CI/CD pipeline (GitHub Actions for tests, linting)

2. **Begin Phase 1 (Core WAL Parser)**:
   - Research SQLite WAL format specification
   - Design `walparser.Parser` API
   - Implement basic WAL file reading and frame decoding
   - Write unit tests for frame decoding

3. **Create project board**:
   - GitHub Issues for all tasks from section 11
   - GitHub Project board for tracking progress

4. **Community engagement**:
   - Create discussion thread in PocketBase community (GitHub Discussions, Discord)
   - Share PRD and gather feedback
   - Identify early adopters for beta testing

**AI-Assisted Development Note**: This PRD is structured as a detailed specification to enable AI-assisted development via Claude Code and SDD tools. Each section provides sufficient context and detail for an AI to generate implementation code, tests, and documentation incrementally.
