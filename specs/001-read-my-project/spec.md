# Feature Specification: WAL-Based SQLite Replication System

**Feature Branch**: `001-read-my-project`
**Created**: 2025-10-06
**Status**: Draft
**Input**: User description: "read my Project-PRD.md file and build the spec from that"

## Execution Flow (main)
```
1. Parse user description from Input
   → Extracted: Build WAL-based SQLite replication system
2. Extract key concepts from description
   → Identified: WAL monitoring, change data capture, multi-primary replication, NATS JetStream, conflict resolution
3. For each unclear aspect:
   → No major ambiguities - PRD is comprehensive
4. Fill User Scenarios & Testing section
   → User flows: PocketBase HA deployment, distributed SQLite apps
5. Generate Functional Requirements
   → All requirements derived from PRD sections
6. Identify Key Entities (if data involved)
   → Entities: Change events, transactions, OpIDs, LSN positions
7. Run Review Checklist
   → PRD provides complete specification
8. Return: SUCCESS (spec ready for planning)
```

---

## ⚡ Quick Guidelines
- ✅ Focus on WHAT users need and WHY
- ❌ Avoid HOW to implement (no tech stack, APIs, code structure)
- 👥 Written for business stakeholders, not developers

---

## User Scenarios & Testing

### Primary User Story

**PocketBase Application Developer**: "I need to deploy PocketBase with high availability across multiple regions. When one server fails, my application should continue serving requests without downtime or data loss. I want this to happen automatically without complex database modifications that could corrupt my data."

**SQLite Application Developer**: "I'm building a distributed edge application where each node needs to replicate data to other nodes. I want multi-primary replication where any node can accept writes, with automatic conflict resolution when writes happen simultaneously on different nodes."

### Acceptance Scenarios

1. **Given** a 3-node cluster with replication enabled, **When** a user writes data to node 1, **Then** the data appears on nodes 2 and 3 within 100ms without any database trigger setup
2. **Given** a multi-statement transaction on node 1, **When** the transaction commits, **Then** all operations replicate atomically to other nodes (all or nothing)
3. **Given** node 2 crashes mid-replication, **When** node 2 restarts, **Then** it automatically catches up and applies all missed changes without data loss
4. **Given** NATS becomes unavailable, **When** nodes continue accepting writes, **Then** changes are buffered locally and replicate when NATS reconnects
5. **Given** two nodes write to the same row simultaneously, **When** both changes replicate, **Then** conflict resolution (LWW, counters, or append-only) produces consistent state across all nodes
6. **Given** a database in replication, **When** examining the database schema, **Then** no triggers, views, or other replication artifacts exist in the database
7. **Given** a replicated database, **When** replication is disabled, **Then** the database file can be used independently without cleanup

### Edge Cases

- What happens when WAL file grows beyond 100MB? → System forces checkpoint and triggers resync if needed
- How does system handle BLOBs larger than 10MB? → System fails replication and alerts operator (configurable strategy)
- What happens during network partition? → Nodes continue independently; Lamport clocks + conflict resolution reconcile on heal
- How does system handle clock skew across nodes? → Lamport clocks provide primary ordering; wall time is tiebreaker only
- What happens if external process checkpoints WAL? → System detects via change counter and triggers resync if frames were missed
- How does system handle transaction with 10,000+ operations? → Uses disk-based buffering (no memory limit)
- What happens when nodes have different conflict resolution strategies configured? → System defaults to LWW strategy and logs warning
- What happens when NATS JetStream storage quota is exhausted? → System halts replication and emits critical alert until storage freed

## Requirements

### Functional Requirements

**Zero Database Modification**
- **FR-001**: System MUST replicate changes without creating triggers in the SQLite database
- **FR-002**: System MUST replicate changes without modifying database schema or tables
- **FR-003**: System MUST allow database to be copied and used independently without replication artifacts
- **FR-004**: System MUST operate by reading WAL files externally without writing to database files

**WAL Parsing and Change Detection**
- **FR-005**: System MUST automatically enable WAL mode (PRAGMA journal_mode=WAL) on startup if database is not in WAL mode
- **FR-006**: System MUST monitor SQLite WAL files for new changes using filesystem notifications or efficient polling
- **FR-007**: System MUST decode WAL frames according to SQLite WAL format specification
- **FR-008**: System MUST detect transaction boundaries via commit frames
- **FR-009**: System MUST extract INSERT, UPDATE, and DELETE operations with table names, primary keys, and column values
- **FR-010**: System MUST buffer operations within a transaction until commit frame is detected
- **FR-011**: System MUST support transactions up to 1000 operations in memory (configurable)
- **FR-012**: System MUST support unlimited transaction size using disk-based buffering
- **FR-013**: System MUST validate WAL frame checksums and detect corruption

**Transaction Atomicity**
- **FR-014**: System MUST replicate multi-statement transactions atomically (all operations or none)
- **FR-015**: System MUST NOT replicate partial transactions (commit frame required)
- **FR-016**: System MUST discard buffered operations if transaction rolls back

**Checkpoint Coordination**
- **FR-017**: System MUST disable SQLite autocheckpoint on startup
- **FR-018**: System MUST track last replicated LSN (Log Sequence Number) position
- **FR-019**: System MUST persist LSN to both NATS JetStream metadata and local state file
- **FR-020**: System MUST execute checkpoints only after confirming replication to NATS
- **FR-021**: System MUST detect external checkpoints and verify no frames were missed
- **FR-022**: System MUST trigger forced checkpoint when WAL exceeds configurable size limit (default: 100MB)
- **FR-023**: System MUST trigger automatic resync if checkpoint occurs before replication completes

**Replication and Distribution**
- **FR-024**: System MUST publish change events to NATS JetStream with unique OpID, NodeID, LSN, and timestamps
- **FR-025**: System MUST subscribe to change events from other nodes via NATS JetStream
- **FR-026**: System MUST apply remote changes to local SQLite database
- **FR-027**: System MUST preserve transaction atomicity when applying remote changes
- **FR-028**: System MUST use Lamport clocks for event ordering across nodes
- **FR-029**: System MUST support configurable NATS connection URLs and credentials

**Idempotency and Deduplication**
- **FR-030**: System MUST assign unique OpID to every change (format: node_id:lsn:timestamp)
- **FR-031**: System MUST maintain persistent store of processed OpIDs
- **FR-032**: System MUST skip duplicate operations gracefully when OpID already exists
- **FR-033**: System MUST clean up OpIDs older than retention window (default: 1 hour, configurable)
- **FR-034**: Deduplication store MUST survive process restarts

**Conflict Resolution**
- **FR-035**: System MUST support Last-Write-Wins (LWW) conflict resolution strategy
- **FR-036**: System MUST support counter-based conflict resolution (sum increments)
- **FR-037**: System MUST support append-only conflict resolution strategy
- **FR-038**: System MUST allow per-table conflict resolution strategy configuration
- **FR-039**: System MUST use Lamport clock as primary ordering mechanism for LWW
- **FR-040**: System MUST use wall time as tiebreaker when Lamport clocks are equal
- **FR-041**: System MUST default to LWW strategy when nodes have conflicting conflict resolution configurations for the same table

**Network Resilience**
- **FR-042**: System MUST buffer changes locally when NATS is unavailable (up to configurable limit)
- **FR-043**: System MUST retry NATS connection with exponential backoff (initial: 1s, max: 30s)
- **FR-044**: System MUST resume publishing buffered changes when NATS reconnects
- **FR-045**: System MUST continue operating during network partitions
- **FR-046**: System MUST reconcile divergent changes when partition heals
- **FR-046a**: System MUST halt replication and emit critical alert when NATS JetStream storage quota is exhausted

**Crash Recovery**
- **FR-047**: System MUST resume parsing from last replicated LSN after parser crash
- **FR-048**: System MUST load LSN from NATS JetStream metadata on startup
- **FR-049**: System MUST fall back to local state file if NATS metadata unavailable
- **FR-050**: System MUST apply buffered changes from other nodes after node crash
- **FR-051**: System MUST trigger full snapshot resync if LSN gap exceeds 10,000 operations or metadata is stale

**Performance**
- **FR-052**: System MUST achieve sub-100ms p95 replication latency for typical workloads
- **FR-053**: System MUST support 1000+ operations per second sustained write load per node
- **FR-054**: System MUST operate with ≤10% CPU overhead compared to non-replicated SQLite
- **FR-055**: System MUST operate with ≤100MB base memory usage plus transaction buffers

**Observability**
- **FR-056**: System MUST expose Prometheus metrics on /metrics endpoint including LSN positions, replication lag, WAL size, operation counts, and error counts
- **FR-057**: System MUST provide /health/live endpoint returning 200 if process is running
- **FR-058**: System MUST provide /health/ready endpoint returning 200 if parser and NATS are connected
- **FR-059**: System MUST provide /health/status endpoint returning detailed JSON status
- **FR-060**: System MUST emit structured JSON logs with appropriate levels (DEBUG, INFO, WARN, ERROR)
- **FR-061**: System MUST log all checkpoint operations with LSN positions and durations
- **FR-062**: System MUST log all conflict resolutions with strategy used
- **FR-063**: System MUST emit metrics for NATS connectivity status and reconnection events

**Configuration**
- **FR-064**: System MUST support TOML or YAML configuration for all parameters
- **FR-065**: System MUST provide sensible defaults for all configuration parameters
- **FR-066**: System MUST validate configuration on startup and fail fast with clear errors
- **FR-067**: System MUST support configuration of node ID, database ID, NATS URLs, WAL parameters, checkpoint settings, deduplication settings, and conflict resolution strategies
- **FR-067a**: System MUST require process restart to enable or disable replication (no runtime control)

**Error Handling**
- **FR-068**: System MUST detect and alert on WAL corruption via checksum validation
- **FR-069**: System MUST stop parsing on corruption to prevent incorrect replication
- **FR-070**: System MUST handle disk full errors gracefully and emit critical alerts
- **FR-071**: System MUST retry filesystem I/O errors with exponential backoff
- **FR-072**: System MUST detect clock skew and emit warnings when exceeds threshold (5 seconds)
- **FR-073**: System MUST fail replication for changes exceeding max size limit (configurable, default: 10MB)

**Security**
- **FR-074**: Parser MUST run with read-only access to WAL files
- **FR-075**: Parser MUST NEVER write to .db, .db-wal, or .db-shm files
- **FR-076**: System MUST support NATS authentication via credentials file
- **FR-077**: System MUST support TLS for NATS connections
- **FR-078**: System MUST NEVER log sensitive data (credentials, tokens)

### Key Entities

- **Change**: Represents a logical database operation extracted from WAL, containing OpID, NodeID, LSN, table name, primary key, operation type (INSERT/UPDATE/DELETE), column values, before/after states, transaction ID, and commit LSN

- **Transaction**: Group of Change events that must be applied atomically, identified by transaction ID, bounded by commit frames in WAL

- **LSN (Log Sequence Number)**: Position in WAL file, used to track parsing progress, replication progress, and checkpoint positions

- **OpID (Operation ID)**: Unique identifier for each change event (format: node_id:lsn:timestamp), used for deduplication across nodes

- **ReplicationMessage**: NATS envelope containing OpID, NodeID, DBID, schema version, wall time, Lamport clock, and transaction batch

- **DeduplicationStore**: Persistent key-value store mapping OpID to timestamp, used to prevent duplicate application of operations

- **CheckpointState**: Tracks last_replicated_lsn, last_checkpointed_lsn, persisted to both NATS JetStream metadata and local state file

- **ConflictResolutionStrategy**: Per-table configuration specifying how to resolve concurrent writes (LWW, counter, append-only)

---

## Clarifications

### Session 2025-10-06

- Q: What threshold should trigger automatic resync when LSN gap is too large? → A: 10,000 operations behind
- Q: What should happen when nodes have different conflict resolution strategies configured? → A: Default to LWW when mismatch detected
- Q: What should happen when replication starts on a non-WAL database? → A: Automatically enable WAL mode
- Q: What should happen when NATS JetStream storage quota is exhausted? → A: Nodes fail fast and halt replication until storage freed
- Q: How should users enable/disable replication at runtime? → A: Configuration file only - requires process restart
- Q: How is replication controlled via configuration? → A: Config file 'enabled' flag controls whether replication starts; process ignores databases when disabled

---

## Review & Acceptance Checklist

### Content Quality
- [x] No implementation details (languages, frameworks, APIs)
- [x] Focused on user value and business needs
- [x] Written for non-technical stakeholders
- [x] All mandatory sections completed

### Requirement Completeness
- [x] No [NEEDS CLARIFICATION] markers remain
- [x] Requirements are testable and unambiguous
- [x] Success criteria are measurable
- [x] Scope is clearly bounded
- [x] Dependencies and assumptions identified

---

## Execution Status

- [x] User description parsed
- [x] Key concepts extracted
- [x] Ambiguities marked
- [x] User scenarios defined
- [x] Requirements generated
- [x] Entities identified
- [x] Review checklist passed

---

## Success Criteria

**Data Correctness**
- Zero data loss across 3-node cluster under normal operations
- Zero duplicate operations (deduplication working)
- 100% transaction atomicity (all operations or none)
- Consistent state across all nodes after partition heal

**Performance**
- p95 replication latency < 100ms for transactions with < 100 operations
- Sustained throughput of 1000+ ops/sec per node
- CPU overhead ≤ 10% vs non-replicated SQLite
- Memory usage ≤ 100MB base + transaction buffers

**Reliability**
- Crash recovery time < 5 seconds
- Zero data loss after any single node crash
- Graceful degradation during NATS unavailability
- Automatic recovery without manual intervention

**Operational**
- No triggers created in database (verified via schema inspection)
- Database files usable independently after disabling replication
- Health check endpoints functional
- All metrics exposed and accurate
- Structured logs with appropriate levels

**PocketBase Integration**
- 3-node PocketBase cluster deploys successfully
- Collections CRUD operations replicate correctly
- Admin UI accessible via load balancer with automatic failover
- Realtime subscriptions work across nodes

---

## Dependencies and Assumptions

**Dependencies**
- SQLite 3.37+ with WAL mode enabled
- NATS 2.9+ with JetStream enabled
- Stable network connectivity between nodes and NATS
- NTP/chrony for time synchronization (clock skew < 5s recommended)
- Local filesystem (SSD/NVMe recommended; NFS not supported for database files)

**Assumptions**
- Single application writer per node (SQLite's WAL mode constraint)
- Schema changes happen during maintenance windows with cluster shutdown
- PocketBase stores schema changes as data changes (collections in tables)
- Nodes have sufficient disk space for WAL files and transaction buffers
- NATS JetStream provides durable message storage and delivery guarantees
- Lamport clocks provide sufficient ordering for conflict resolution

**Out of Scope**
- DDL/Schema change detection and replication
- Migration from trigger-based CDC systems
- Support for SQLite versions prior to 3.37
- Replication over unreliable networks (high packet loss, frequent partitions)
- Multi-region deployments with high inter-region latency (>500ms)
- Custom conflict resolution strategies beyond LWW, counters, append-only
- NATS message schema changes or protocol modifications
