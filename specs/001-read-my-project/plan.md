# Implementation Plan: WAL-Based SQLite Replication System

**Branch**: `001-read-my-project` | **Date**: 2025-10-06 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/Users/markb/dev/harmonylite-cdc/specs/001-read-my-project/spec.md`

## Execution Flow (/plan command scope)
```
1. Load feature spec from Input path
   → ✅ LOADED: WAL-Based SQLite Replication System
2. Fill Technical Context (scan for NEEDS CLARIFICATION)
   → ✅ FILLED: No NEEDS CLARIFICATION markers in spec
3. Fill the Constitution Check section
   → ⏳ IN PROGRESS
4. Evaluate Constitution Check section
   → ⏳ PENDING
5. Execute Phase 0 → research.md
   → ⏳ PENDING
6. Execute Phase 1 → contracts, data-model.md, quickstart.md, CLAUDE.md
   → ⏳ PENDING
7. Re-evaluate Constitution Check section
   → ⏳ PENDING
8. Plan Phase 2 → Describe task generation approach
   → ⏳ PENDING
9. STOP - Ready for /tasks command
   → ⏳ PENDING
```

**IMPORTANT**: The /plan command STOPS at step 9. Phases 2-4 are executed by other commands:
- Phase 2: /tasks command creates tasks.md
- Phase 3-4: Implementation execution (manual or via tools)

## Summary

Build a high-performance, trigger-free SQLite replication system using Write-Ahead Log (WAL) monitoring to deliver multi-primary distributed consistency through NATS JetStream. The system provides automatic failover, conflict resolution, and zero-modification replication for SQLite databases, with primary focus on PocketBase high-availability deployments.

**Key Innovation**: External WAL parsing eliminates database modification overhead while maintaining full transaction atomicity and sub-100ms replication latency.

## Technical Context

**Language/Version**: Go 1.21+ (use latest stable, per constitution)
**Primary Dependencies**:
- NATS 2.9+ with JetStream (message transport & coordination)
- SQLite 3.37+ (WAL format parsing)
- Ginkgo v2 (E2E testing framework, per HarmonyLite practices)
- Prometheus client (metrics)
- zerolog (structured logging)

**Storage**:
- SQLite databases in WAL mode (read-only access to .db-wal files)
- NATS JetStream (durable message storage)
- Local state files (LSN persistence, deduplication store)

**Testing**:
- Ginkgo for E2E tests (per constitution)
- Go testing for unit tests
- Chaos engineering tools (Toxiproxy, iptables for network partitions)
- Performance benchmarks (latency, throughput measurement)

**Target Platform**:
- Linux (primary) - SSD/NVMe local filesystem required
- macOS (development)
- Windows (community support)
- Container deployment (Docker, Kubernetes)

**Project Type**: single - standalone replication system (sidecar deployment model)

**Performance Goals**:
- p95 replication latency < 100ms for transactions < 100 operations
- Sustained throughput ≥ 1000 ops/sec per node
- CPU overhead ≤ 10% vs non-replicated SQLite
- Memory usage ≤ 100MB base + transaction buffers

**Constraints**:
- Read-only access to WAL files (security principle)
- No database modification (constitutional principle I)
- SQLite single writer per node (WAL mode constraint)
- LSN gap > 10,000 ops triggers resync (clarification)
- Max change size 10MB (default, configurable)
- NATS JetStream storage quota must not be exhausted

**Scale/Scope**:
- 3-7 node clusters (recommended)
- Database size tested to 100GB
- WAL size tested to 1GB
- Unlimited transaction size (disk-based buffering)
- Support for PocketBase production deployments

## Constitution Check
*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

### Principle I: Zero Database Modification (NON-NEGOTIABLE)
- ✅ **COMPLIANT**: System design based on external WAL parsing
- ✅ **COMPLIANT**: Read-only access to WAL files (FR-072, FR-075)
- ✅ **COMPLIANT**: No triggers, schema changes, or modifications (FR-001, FR-002)
- ✅ **COMPLIANT**: Database files usable independently after disabling replication (FR-003)

### Principle II: WAL-Based Change Detection
- ✅ **COMPLIANT**: Core architecture uses WAL file parsing (FR-006, FR-007)
- ✅ **COMPLIANT**: Read-only parser with no writes to .db* files (FR-004, FR-073)
- ✅ **COMPLIANT**: External change detection via WAL frames (FR-008, FR-009)

### Principle III: Test-First Development (NON-NEGOTIABLE)
- ✅ **COMPLIANT**: TDD required for all code (per constitution)
- ✅ **COMPLIANT**: Unit tests planned for WAL parsing, frame decoding, transaction reconstruction
- ✅ **COMPLIANT**: Integration tests planned for multi-node scenarios (7 acceptance scenarios)
- ✅ **COMPLIANT**: E2E tests using Ginkgo framework (per HarmonyLite practices)
- ✅ **COMPLIANT**: Chaos engineering tests planned for network partitions, crashes
- ✅ **COMPLIANT**: Performance benchmarks required (FR-052, FR-053)

### Principle IV: Transaction Atomicity
- ✅ **COMPLIANT**: Atomic replication enforced (FR-014)
- ✅ **COMPLIANT**: Transaction boundary detection via commit frames (FR-008, FR-015)
- ✅ **COMPLIANT**: Buffering until commit (FR-010, FR-011, FR-012)
- ✅ **COMPLIANT**: Rollback handling (FR-016)
- ✅ **COMPLIANT**: Disk-based buffering for large transactions (FR-012)

### Principle V: Observability and Operations
- ✅ **COMPLIANT**: Prometheus metrics on /metrics endpoint (FR-056)
- ✅ **COMPLIANT**: Structured JSON logging with levels (FR-060)
- ✅ **COMPLIANT**: Health check endpoints (FR-057, FR-058, FR-059)
- ✅ **COMPLIANT**: Checkpoint operations logged (FR-061)
- ✅ **COMPLIANT**: Conflict resolution logged (FR-062)

### Principle VI: Performance and Scalability
- ✅ **COMPLIANT**: p95 latency < 100ms target (FR-052)
- ✅ **COMPLIANT**: Throughput ≥ 1000 ops/sec (FR-053)
- ✅ **COMPLIANT**: CPU overhead ≤ 10% (FR-054)
- ✅ **COMPLIANT**: Memory ≤ 100MB base (FR-055)

### Principle VII: Simplicity and Minimal Configuration
- ✅ **COMPLIANT**: TOML/YAML configuration with sensible defaults (FR-064, FR-065)
- ✅ **COMPLIANT**: Clear error messages (FR-066)
- ✅ **COMPLIANT**: Self-service deployment documentation planned
- ✅ **COMPLIANT**: Minimal abstraction (direct WAL parsing, no unnecessary layers)

### Quality Standards: Error Handling Priorities
- ✅ **COMPLIANT**: Priority 1 (Network issues) - automated recovery (FR-042, FR-043, FR-044)
- ✅ **COMPLIANT**: Priority 2 (Node crashes) - automated recovery (FR-047, FR-048, FR-051)
- ✅ **COMPLIANT**: Priority 3-4 (Data/Storage issues) - alerts + recovery procedures (FR-068, FR-070, FR-071)

### Quality Standards: Security Requirements
- ✅ **COMPLIANT**: Read-only access to WAL files (FR-074)
- ✅ **COMPLIANT**: No writes to database files (FR-075)
- ✅ **COMPLIANT**: NATS authentication support (FR-076)
- ✅ **COMPLIANT**: TLS support (FR-077)
- ✅ **COMPLIANT**: No sensitive data in logs (FR-078)

### Quality Standards: Compatibility and Versioning
- ✅ **COMPLIANT**: SQLite 3.37+ support documented
- ✅ **COMPLIANT**: Go 1.21+ required
- ✅ **COMPLIANT**: NATS 2.9+ with JetStream
- ✅ **COMPLIANT**: Semantic versioning planned
- ✅ **COMPLIANT**: Backward compatibility for NATS messages

### Development Workflow: Build and Version Management
- ✅ **COMPLIANT**: Makefile planned for build automation
- ✅ **COMPLIANT**: Version injection (semantic ver, commit hash, timestamp, Go version)

### Development Workflow: Code Organization
- ✅ **COMPLIANT**: Modular structure planned:
  - walparser/ (WAL parsing library)
  - checkpoint/ (checkpoint coordination)
  - replication/ (NATS integration)
  - dedup/ (OpID deduplication)
  - config/ (configuration)
  - health/ (health checks)
  - telemetry/ (metrics and logging)
  - tests/e2e/ (Ginkgo E2E tests)

### Development Workflow: Documentation Standards
- ✅ **COMPLIANT**: Deployment guide planned (Docker Compose, K8s, systemd)
- ✅ **COMPLIANT**: PocketBase integration guide planned
- ✅ **COMPLIANT**: Configuration reference planned
- ✅ **COMPLIANT**: Troubleshooting guide and runbooks planned
- ✅ **COMPLIANT**: GoDoc for all public interfaces

**GATE STATUS**: ✅ **PASS** - No constitutional violations detected

## Project Structure

### Documentation (this feature)
```
specs/001-read-my-project/
├── plan.md              # This file (/plan command output)
├── research.md          # Phase 0 output (/plan command)
├── data-model.md        # Phase 1 output (/plan command)
├── quickstart.md        # Phase 1 output (/plan command)
├── contracts/           # Phase 1 output (/plan command)
└── tasks.md             # Phase 2 output (/tasks command - NOT created by /plan)
```

### Source Code (repository root)
```
# Single project structure (sidecar deployment model)
walparser/
├── parser.go           # WAL file reading and frame decoding
├── frame.go            # Frame structure and decoding
├── transaction.go      # Transaction reconstruction
├── buffer.go           # Hybrid memory + disk buffering
└── checksum.go         # Frame checksum validation

checkpoint/
├── manager.go          # Checkpoint coordination logic
├── lsn.go              # LSN tracking and persistence
└── state.go            # State file management

replication/
├── publisher.go        # NATS change event publishing
├── subscriber.go       # NATS change event subscription
├── applicator.go       # Apply remote changes to local DB
├── message.go          # ReplicationMessage envelope
└── conflict.go         # Conflict resolution (LWW, counter, append-only)

dedup/
├── store.go            # OpID deduplication store interface
├── sqlite.go           # SQLite-based implementation
└── cleanup.go          # OpID retention and cleanup

config/
├── config.go           # Configuration loading and validation
├── yaml.go             # YAML parsing
└── defaults.go         # Sensible defaults

health/
├── handlers.go         # HTTP health check endpoints
├── live.go             # Liveness probe
├── ready.go            # Readiness probe
└── status.go           # Detailed status endpoint

telemetry/
├── metrics.go          # Prometheus metrics
├── logging.go          # Structured JSON logging
└── alerts.go           # Alert emission

cmd/
└── harmonylite-cdc/
    └── main.go         # Entry point, CLI

tests/
├── e2e/                # Ginkgo E2E tests
│   ├── replication_test.go
│   ├── crash_recovery_test.go
│   ├── network_partition_test.go
│   └── performance_test.go
├── integration/        # Integration tests
│   ├── wal_parser_test.go
│   ├── checkpoint_test.go
│   └── nats_test.go
└── unit/               # Unit tests
    ├── frame_decode_test.go
    ├── transaction_test.go
    └── dedup_test.go

docs/
├── deployment.md       # Deployment guide
├── pocketbase.md       # PocketBase integration
├── configuration.md    # Config reference
├── troubleshooting.md  # Troubleshooting guide
└── runbooks/           # Operational runbooks
```

**Structure Decision**: Single project structure selected. This is a standalone replication system deployed as a sidecar alongside SQLite applications (primarily PocketBase). The modular organization follows constitutional requirements with standalone, independently testable components. The walparser/ package is the core library, with other packages providing coordination, messaging, observability, and operational features.

## Phase 0: Outline & Research

**Status**: ⏳ Starting research phase

### Research Areas

All technical context is specified from the PRD and constitution. No NEEDS CLARIFICATION markers remain. Research will focus on:

1. **SQLite WAL Format Specification**
   - Decision: Study official SQLite WAL file format documentation
   - Rationale: Need precise understanding of frame structure, checksums, commit markers
   - Alternatives: Reverse engineering (rejected - official spec is authoritative)

2. **Go SQLite Libraries**
   - Decision: Evaluate go-sqlite3, modernc.org/sqlite for PRAGMA execution (WAL mode, autocheckpoint disable)
   - Rationale: Need to interact with SQLite for configuration, not data access
   - Alternatives: Direct SQL via database/sql (preferred for minimal dependencies)

3. **NATS JetStream Best Practices**
   - Decision: Study JetStream durable consumers, ack policies, retention
   - Rationale: Need reliable message delivery and LSN persistence
   - Alternatives: Redis Streams (rejected per PRD - NATS selected)

4. **Lamport Clock Implementation**
   - Decision: Research standard Lamport clock patterns in Go
   - Rationale: Required for conflict resolution ordering (FR-039)
   - Alternatives: Vector clocks (rejected - Lamport sufficient for LWW)

5. **Filesystem Notification Libraries**
   - Decision: Evaluate fsnotify for WAL file monitoring
   - Rationale: Efficient change detection vs polling (FR-006)
   - Alternatives: Polling with configurable interval (fallback option)

6. **Deduplication Store Options**
   - Decision: Compare SQLite, BoltDB, BadgerDB for OpID storage
   - Rationale: Need persistent, fast key-value store (FR-030, FR-033)
   - Alternatives: In-memory with periodic snapshots (rejected - must survive restart)

7. **Chaos Engineering Tools**
   - Decision: Evaluate Toxiproxy, iptables, Chaos Mesh for testing
   - Rationale: Required for network partition and failure testing
   - Alternatives: Manual testing (rejected - too error-prone)

8. **Prometheus Go Client**
   - Decision: Use official prometheus/client_golang
   - Rationale: Standard library for Prometheus metrics (FR-056)
   - Alternatives: Custom metrics (rejected - unnecessary complexity)

9. **Structured Logging Libraries**
   - Decision: Evaluate zerolog vs zap for performance
   - Rationale: High-throughput logging required (FR-060)
   - Alternatives: Standard log package (rejected - no structured logging)

10. **Ginkgo Testing Framework**
    - Decision: Use Ginkgo v2 for E2E tests (per HarmonyLite practices)
    - Rationale: Constitutional requirement, behavior-driven testing style
    - Alternatives: Standard Go testing (rejected - constitution mandates Ginkgo)

**Output**: ✅ research.md generated in /Users/markb/dev/harmonylite-cdc/specs/001-read-my-project/research.md

## Phase 1: Design & Contracts

**Status**: ✅ Complete

### Deliverables Generated

1. **data-model.md** - Core entities with fields, validation rules, relationships
   - 8 entities: Change, Transaction, LSN, OpID, ReplicationMessage, DeduplicationStore, CheckpointState, ConflictResolutionStrategy
   - 3 enumerations: ChangeType, LSNType, StrategyType
   - Relationship diagram and data volume estimates

2. **contracts/** - Interface and message specifications
   - `parser-contract.md` - WAL Parser interface with contract guarantees
   - `replication-message-schema.json` - JSON schema for NATS messages
   - `health-api-spec.yaml` - OpenAPI spec for health/metrics endpoints

3. **quickstart.md** - Manual testing scenarios
   - 7 acceptance test scenarios
   - Performance validation procedures
   - Troubleshooting guide

4. **CLAUDE.md** - Agent context file updated with:
   - Language: Go 1.21+
   - Project type: single (sidecar deployment)
   - Dependencies: NATS, SQLite, Ginkgo

### Design Validation

All designs comply with constitutional principles:
- ✅ No database modification (read-only WAL access)
- ✅ TDD approach (contracts define tests first)
- ✅ Transaction atomicity preserved
- ✅ Comprehensive observability (health checks, metrics, logging)
- ✅ Performance targets specified (< 100ms p95, ≥ 1000 ops/sec)
- ✅ Modular structure (standalone components)

## Phase 2: Task Planning Approach

**Status**: Ready for /tasks command

**IMPORTANT**: This section describes the strategy for /tasks command - DO NOT execute task generation during /plan

### Task Generation Strategy

The /tasks command will:

1. **Load templates and artifacts**:
   - Base: `.specify/templates/tasks-template.md`
   - Inputs: plan.md, research.md, data-model.md, contracts/, quickstart.md

2. **Generate setup tasks**:
   - T001: Initialize Go module with dependencies (NATS, SQLite, Ginkgo, etc.)
   - T002: Create directory structure per plan.md (walparser/, checkpoint/, etc.)
   - T003: Configure Makefile with version injection (per constitution)
   - T004: Set up CI/CD pipeline (GitHub Actions for tests, linting)

3. **Generate test tasks (TDD - BEFORE implementation)**:
   - From parser-contract.md:
     - T005: [P] WAL frame decoding unit tests (tests/unit/frame_decode_test.go)
     - T006: [P] Transaction reconstruction unit tests (tests/unit/transaction_test.go)
     - T007: [P] Checksum validation unit tests (tests/unit/checksum_test.go)
   - From data-model.md:
     - T008: [P] Change entity validation tests
     - T009: [P] OpID parsing tests
     - T010: [P] Deduplication store tests
   - From health-api-spec.yaml:
     - T011: [P] Health endpoint contract tests
   - From quickstart.md acceptance scenarios:
     - T012: [P] E2E basic replication test (Ginkgo)
     - T013: [P] E2E transaction atomicity test
     - T014: [P] E2E crash recovery test
     - T015: [P] E2E NATS unavailability test
     - T016: [P] E2E conflict resolution test

4. **Generate implementation tasks (AFTER tests fail)**:
   - Core parsing:
     - T017: Implement WAL frame decoder (walparser/frame.go)
     - T018: Implement transaction buffer (walparser/buffer.go)
     - T019: Implement checksum validator (walparser/checksum.go)
     - T020: Implement Parser main logic (walparser/parser.go)
   - Checkpoint coordination:
     - T021: Implement LSN tracker (checkpoint/lsn.go)
     - T022: Implement checkpoint manager (checkpoint/manager.go)
   - Replication:
     - T023: Implement NATS publisher (replication/publisher.go)
     - T024: Implement NATS subscriber (replication/subscriber.go)
     - T025: Implement change applicator (replication/applicator.go)
     - T026: Implement conflict resolution (replication/conflict.go)
   - Supporting:
     - T027: Implement deduplication store (dedup/sqlite.go)
     - T028: Implement configuration (config/config.go)
     - T029: Implement health checks (health/handlers.go)
     - T030: Implement telemetry (telemetry/metrics.go, telemetry/logging.go)
     - T031: Implement main entry point (cmd/harmonylite-cdc/main.go)

5. **Generate integration tasks**:
   - T032: Integration test: WAL parser + checkpoint (tests/integration/wal_parser_test.go)
   - T033: Integration test: Replication + NATS (tests/integration/nats_test.go)

6. **Generate polish tasks**:
   - T034: Run full E2E test suite (Ginkgo)
   - T035: Execute quickstart.md scenarios manually
   - T036: Performance benchmarks (latency, throughput)
   - T037: Documentation: deployment.md, pocketbase.md, configuration.md, troubleshooting.md
   - T038: Code review and refactoring

### Task Ordering Strategy

**TDD Order** (constitutional requirement):
1. Setup (T001-T004)
2. Tests FIRST (T005-T016) - all tests must FAIL before proceeding
3. Implementation (T017-T031) - make tests pass
4. Integration (T032-T033)
5. Polish (T034-T038)

**Dependency Order**:
- Setup tasks sequential (T001 → T002 → T003 → T004)
- Test tasks parallel [P] (different files, no dependencies)
- Implementation tasks respect dependencies:
  - Frame decoder (T017) before Parser (T020)
  - LSN tracker (T021) before Checkpoint manager (T022)
  - Publisher (T023) and Subscriber (T024) before Applicator (T025)
  - Config (T028) before Main (T031)

**Parallelization** (marked with [P]):
- Tests: T005-T016 (all parallel, different files)
- Dedup store (T027) parallel with Config (T028)
- Health checks (T029) parallel with Telemetry (T030)

### Estimated Output

- **Total tasks**: ~38 numbered tasks
- **Parallel opportunities**: ~14 tasks marked [P]
- **Critical path**: Setup → Tests → Core parsing → Replication → Integration → E2E
- **Estimated duration**: 4-6 weeks (assuming TDD discipline, thorough testing)

### Validation Checklist (for /tasks command)

The /tasks command must verify:
- ✅ All contracts have corresponding tests
- ✅ All entities from data-model.md have validation tests
- ✅ All quickstart scenarios have E2E tests
- ✅ Tests come before implementation (TDD order)
- ✅ Parallel tasks truly independent (different files)
- ✅ Each task specifies exact file path
- ✅ Constitutional principles reflected in task descriptions

## Phase 3+: Future Implementation

**Phase 3**: Task execution (/tasks command will create tasks.md)
**Phase 4**: Implementation (execute tasks.md following constitutional principles)
**Phase 5**: Validation (run tests, execute quickstart.md, performance validation)

## Complexity Tracking

*No constitutional violations detected - this section remains empty.*

## Progress Tracking

**Phase Status**:
- [x] Phase 0: Research complete (/plan command)
- [x] Phase 1: Design complete (/plan command)
- [x] Phase 2: Task planning approach complete (/plan command - described above)
- [x] Phase 3: Tasks generated (/tasks command) - 57 tasks created
- [ ] Phase 4: Implementation complete
- [ ] Phase 5: Validation passed

**Gate Status**:
- [x] Initial Constitution Check: PASS
- [x] Post-Design Constitution Check: PASS
- [x] All NEEDS CLARIFICATION resolved (5 clarifications recorded in spec.md)
- [x] Complexity deviations documented (none - no violations)

**Execution Flow Status**:
1. ✅ Feature spec loaded: WAL-Based SQLite Replication System
2. ✅ Technical Context filled: No NEEDS CLARIFICATION markers
3. ✅ Constitution Check section filled: All principles reviewed
4. ✅ Constitution Check evaluated: PASS (no violations)
5. ✅ Phase 0 executed: research.md generated
6. ✅ Phase 1 executed: data-model.md, contracts/, quickstart.md, CLAUDE.md created
7. ✅ Constitution Check re-evaluated: PASS (design compliant)
8. ✅ Phase 2 planned: Task generation approach documented above
9. ✅ STOP - Ready for /tasks command

---
*Plan based on Constitution v1.0.0 - See `.specify/memory/constitution.md`*
*Feature specification with 5 clarifications - See `spec.md`*

