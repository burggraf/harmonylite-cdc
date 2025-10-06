# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

HarmonyLite CDC is a distributed SQLite replication system being migrated from trigger-based CDC to **WAL-based (Write-Ahead Log) parsing**. The system enables leaderless, eventually consistent replication between SQLite nodes using NATS JetStream.

**Current State**: Fork of [HarmonyLite](https://github.com/wongfei2009/harmonylite) transitioning to WAL-based approach
**Key Innovation**: External WAL parsing eliminates database modification overhead while maintaining transaction atomicity

## Build & Development Commands

### Building
```bash
# Standard build with version injection
make build

# Static binary (for containers)
make build-static

# Linux AMD64 (for cross-compilation)
make build-linux-amd64

# CGO is REQUIRED for SQLite - builds will fail without it
```

### Testing
```bash
# Run all tests
go test -v ./...

# Run E2E tests using Ginkgo v2 (constitutional requirement)
go run github.com/onsi/ginkgo/v2/ginkgo tests/e2e

# Run specific test package
go test -v ./walparser
go test -v ./checkpoint
```

### Running
```bash
# Display version information
./harmonylite -version

# Run with config file
./harmonylite -config config.toml

# Clean up CDC triggers (removes trigger-based implementation)
./harmonylite -config config.toml -cleanup

# Save snapshot
./harmonylite -config config.toml -save-snapshot
```

## Architecture Overview

### Current (Trigger-Based) Architecture
```
Application → SQLite DB → Triggers → __harmonylite__* tables → fsnotify → NATS JetStream
                                                                              ↓
Application ← SQLite DB ← Replicator ← NATS Consumer ←──────────────────────┘
```

### Target (WAL-Based) Architecture
```
Application → SQLite DB → WAL file (read-only) → WAL Parser → NATS JetStream
                                                                      ↓
Application ← SQLite DB ← Applicator ← NATS Consumer ←──────────────┘
```

### Key Architectural Components

**Existing (to be preserved/extended)**:
- `stream/`: NATS connection and message routing → **Adapt for JetStream**
- `logstream/`: Replication event handling → **Adapt for WAL-based messages**
- `snapshot/`: Database snapshot for resync → **Reuse**
- `health/`: Health check endpoints → **Extend with parser status**
- `telemetry/`: Prometheus metrics and zerolog logging → **Extend**
- `cfg/`: TOML configuration → **Extend**

**New (to be implemented)**:
- `walparser/`: WAL file parsing, frame decoding, transaction reconstruction
- `checkpoint/`: LSN tracking, checkpoint coordination
- `dedup/`: OpID deduplication store (SQLite-based)
- `replication/`: Conflict resolution (LWW, counter, append-only), Lamport clocks

**To be removed**:
- `db/change_log.go`, `db/change_log_event.go`: Trigger-based CDC (replaced by WAL parsing)
- `db/*.tmpl`: Trigger templates (no longer needed)

## Code Organization Principles

### Constitutional Requirements (`.specify/memory/constitution.md`)

**NON-NEGOTIABLE**:
1. **Zero Database Modification**: No triggers, no schema changes, read-only WAL access
2. **Test-First Development (TDD)**: Write tests → verify they FAIL → implement → tests PASS
3. **Transaction Atomicity**: Multi-statement transactions replicate atomically

**Performance Targets**:
- p95 replication latency < 100ms
- Sustained throughput ≥ 1000 ops/sec per node
- CPU overhead ≤ 10% vs non-replicated SQLite
- Memory ≤ 100MB base + transaction buffers

### Module Structure

All new modules follow this pattern:
```go
// walparser/parser.go - Core interface
type Parser interface {
    NextTransaction(ctx context.Context) ([]Change, error)
    GetLSN() int64
    Checkpoint(mode CheckpointMode) error
    Close() error
}

// Tests MUST exist before implementation
// walparser/parser_test.go or tests/unit/parser_test.go
```

### Testing Strategy

**TDD Order** (strictly enforced):
1. Write contract tests (from `specs/001-read-my-project/contracts/`)
2. Write entity validation tests (from `specs/001-read-my-project/data-model.md`)
3. Write E2E tests using **Ginkgo v2** (from `specs/001-read-my-project/quickstart.md`)
4. Verify ALL tests FAIL
5. Implement to make tests pass

**Test Organization**:
- `tests/unit/`: Unit tests (frame decoding, checksum, OpID parsing)
- `tests/integration/`: Multi-component tests (parser + checkpoint, NATS + replication)
- `tests/e2e/`: End-to-end tests using Ginkgo v2 (7 acceptance scenarios)

## Configuration

Configuration uses TOML format (`config.toml`). Key sections:

```toml
db_path = "/path/to/database.db"  # Target SQLite database
node_id = 1                        # Unique node identifier

[snapshot]
enabled = true
store = "nats"  # Options: nats, s3, webdav, sftp

[replication_log]
nats_url = "nats://localhost:4222"
shards = 8
```

**New WAL-specific config** (to be added):
```toml
[wal_parser]
buffer_size = 1000              # In-memory transaction buffer (ops)
disk_buffer_threshold = 1000    # When to spill to disk
checkpoint_threshold = 104857600 # 100MB WAL size triggers checkpoint

[deduplication]
retention_window = 3600000      # 1 hour in milliseconds

[conflict_resolution]
default_strategy = "lww"        # lww, counter, append-only
```

## Data Flow

### Publishing Flow (Local Changes → Remote Nodes)
1. Application writes to SQLite → WAL file updated
2. **WAL Parser** (NEW): `walparser.NextTransaction()` reads WAL, decodes frames, buffers until commit
3. **Checkpoint Manager** (NEW): Tracks LSN, prevents checkpoint until replication confirmed
4. **Publisher** (MODIFIED): Publishes `ReplicationMessage` to NATS JetStream with OpID, Lamport clock
5. NATS JetStream: Durable storage, delivers to subscribers

### Subscribing Flow (Remote Changes → Local DB)
1. **Subscriber** (MODIFIED): Pulls messages from NATS JetStream (durable consumer)
2. **Deduplicator** (NEW): Checks OpID, skips if already processed
3. **Conflict Resolver** (NEW): Applies LWW/counter/append-only strategy if conflicts
4. **Applicator** (NEW): Applies changes to local SQLite, preserves transaction atomicity

## Key Design Decisions (from `specs/001-read-my-project/research.md`)

- **SQLite Library**: `modernc.org/sqlite` (CGo-free) preferred over `mattn/go-sqlite3`
- **Logging**: `zerolog` (already in use) - zero allocations, 40 B/op
- **Testing**: Ginkgo v2 for E2E (constitutional requirement)
- **NATS JetStream**: Durable pull consumers with `AckExplicit` policy
- **Lamport Clocks**: HashiCorp Serf pattern (atomic-based)
- **Deduplication**: SQLite-based (consistent with existing stack)
- **Filesystem Watching**: `fsnotify` for WAL file monitoring

## Implementation Status

**Phase 0**: ✅ Research complete (`specs/001-read-my-project/research.md`)
**Phase 1**: ✅ Design complete (data model, contracts, quickstart scenarios)
**Phase 2**: ✅ Task planning complete (`specs/001-read-my-project/tasks.md` - 57 tasks)
**Phase 3**: ⏳ IN PROGRESS - Implementation

**Current Branch**: `001-read-my-project`

## Common Pitfalls

1. **Do NOT modify database schema**: Parser operates read-only on WAL files
2. **Do NOT skip TDD**: Tests must be written and fail before implementation
3. **Do NOT use `mattn/go-sqlite3`**: Prefer `modernc.org/sqlite` for CGo-free deployment
4. **Do NOT checkpoint before replication**: Violates safety guarantees (LSN tracking)
5. **Do NOT ignore existing modules**: Extend `health/`, `telemetry/`, `stream/` rather than rewrite

## Versioning

Version information is automatically injected during build via Makefile:
- Semantic version (from Git tags)
- Git commit hash
- Build date/time
- Go version
- Platform

Access via:
```go
import "github.com/wongfei2009/harmonylite/version"
version.Get().String()
```

## Related Documentation

- **Feature Spec**: `specs/001-read-my-project/spec.md` (78 functional requirements)
- **Implementation Plan**: `specs/001-read-my-project/plan.md` (architecture, constitution check)
- **Data Model**: `specs/001-read-my-project/data-model.md` (8 entities, relationships)
- **Contracts**: `specs/001-read-my-project/contracts/` (parser, message schema, health API)
- **Quickstart Tests**: `specs/001-read-my-project/quickstart.md` (7 manual acceptance scenarios)
- **Tasks**: `specs/001-read-my-project/tasks.md` (57 implementation tasks, TDD order)
- **Constitution**: `.specify/memory/constitution.md` (development principles, quality standards)

## Dependencies

**Required**:
- Go 1.21+ (use latest stable)
- SQLite 3.37+ (for WAL format compatibility)
- NATS 2.9+ with JetStream enabled
- CGO_ENABLED=1 (for SQLite C bindings)

**Key Go Packages**:
- `modernc.org/sqlite` (CGo-free SQLite)
- `github.com/nats-io/nats.go` (NATS client with JetStream)
- `github.com/onsi/ginkgo/v2` (E2E testing)
- `github.com/onsi/gomega` (Ginkgo assertions)
- `github.com/rs/zerolog` (structured logging)
- `github.com/prometheus/client_golang` (metrics)
- `github.com/fsnotify/fsnotify` (filesystem watching)

## Performance Considerations

- **WAL Parsing**: Target 10,000 ops/sec raw parsing throughput
- **Memory**: 100MB base + 1KB per buffered operation
- **Disk Buffer**: Activates for transactions > 1000 operations
- **Checkpoint**: Triggered when WAL > 100MB (configurable)
- **Deduplication**: OpID cleanup after 1 hour retention (configurable)
