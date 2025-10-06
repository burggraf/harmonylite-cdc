# Tasks: WAL-Based SQLite Replication System

**Input**: Design documents from `/Users/markb/dev/harmonylite-cdc/specs/001-read-my-project/`
**Prerequisites**: plan.md, research.md, data-model.md, contracts/, quickstart.md

## Execution Flow (main)
```
1. Load plan.md from feature directory
   → ✅ LOADED: Go 1.21+, NATS JetStream, Ginkgo v2, existing HarmonyLite codebase
2. Load optional design documents:
   → data-model.md: 8 entities (Change, Transaction, LSN, OpID, etc.)
   → contracts/: 3 files (parser-contract.md, replication-message-schema.json, health-api-spec.yaml)
   → quickstart.md: 7 acceptance test scenarios
3. Generate tasks by category:
   → Setup: Go module update, new directories, Makefile update
   → Tests: contract tests, integration tests, E2E tests (Ginkgo)
   → Core: WAL parser (NEW), checkpoint (NEW), replication (MODIFIED), dedup (NEW)
   → Integration: NATS JetStream, health checks (EXTEND), telemetry (EXTEND)
   → Polish: benchmarks, documentation
4. Apply task rules:
   → Different files = mark [P] for parallel
   → Same file = sequential (no [P])
   → Tests before implementation (TDD)
5. Number tasks sequentially (T001, T002...)
6. Return: SUCCESS (tasks ready for execution)
```

## Format: `[ID] [P?] Description`
- **[P]**: Can run in parallel (different files, no dependencies)
- Include exact file paths in descriptions

## Path Conventions
- Repository root: `/Users/markb/dev/harmonylite-cdc/`
- Source code at root level (existing HarmonyLite structure)
- New modules: `walparser/`, `checkpoint/`, `dedup/`
- Modified modules: `replication/` (adapt existing), `health/` (extend), `telemetry/` (extend)
- Tests: `tests/unit/`, `tests/integration/`, `tests/e2e/`

## Phase 3.1: Setup & Preparation

- [x] T001: Update go.mod with new dependencies (modernc.org/sqlite, nats.io/nats.go JetStream, ginkgo v2, gomega, prometheus client)
- [x] T002: [P] Create new directory structure (walparser/, checkpoint/, dedup/, tests/unit/, tests/integration/, tests/e2e/)
- [x] T003: Update Makefile with version injection for WAL parser components and new build targets
- [x] T004: [P] Create configuration structs in cfg/ for WAL parsing settings, checkpoint thresholds, deduplication retention (extend existing TOML config)

## Phase 3.2: Tests First (TDD) ⚠️ MUST COMPLETE BEFORE 3.3
**CRITICAL: These tests MUST be written and MUST FAIL before ANY implementation**

### Contract Tests (from contracts/)
- [x] T005: [P] WAL Parser contract test in tests/unit/walparser_contract_test.go (verify NextTransaction, GetLSN, Checkpoint interfaces per parser-contract.md)
- [x] T006: [P] Replication message schema validation test in tests/unit/replication_message_test.go (validate against replication-message-schema.json)
- [x] T007: [P] Health API contract test in tests/integration/health_api_test.go (verify /health/live, /health/ready, /health/status, /metrics endpoints per health-api-spec.yaml)

### Entity Validation Tests (from data-model.md)
- [x] T008: [P] Change entity validation test in tests/unit/change_test.go (OpID format, field requirements, Before/After logic for INSERT/UPDATE/DELETE)
- [x] T009: [P] Transaction entity validation test in tests/unit/transaction_test.go (atomicity, LSN ordering, TransactionID grouping)
- [x] T010: [P] LSN tracking test in tests/unit/lsn_test.go (monotonic increase, persistence to NATS metadata and local file)
- [x] T011: [P] OpID parsing and validation test in tests/unit/opid_test.go (format: node_id:lsn:timestamp, uniqueness)
- [x] T012: [P] Deduplication store test in tests/unit/dedup_store_test.go (OpID persistence, retention cleanup, restart survival)
- [x] T013: [P] Checkpoint state test in tests/unit/checkpoint_state_test.go (LSN tracking: parsed ≥ replicated ≥ checkpointed)

### Unit Tests for WAL Parsing
- [x] T014: [P] WAL frame decoding test in tests/unit/frame_decode_test.go (parse frame header, payload, checksum validation)
- [x] T015: [P] Transaction reconstruction test in tests/unit/transaction_reconstruction_test.go (buffer ops until commit, rollback handling, disk buffering for large transactions)
- [x] T016: [P] Checksum validation test in tests/unit/checksum_test.go (detect corruption, handle corrupted WAL gracefully)

### Integration Tests (Multi-Component)
- [x] T017: [P] WAL parser + checkpoint integration test in tests/integration/wal_checkpoint_test.go (parse → replicate → checkpoint cycle, LSN tracking)
- [x] T018: [P] NATS JetStream integration test in tests/integration/nats_jetstream_test.go (publish, subscribe, durable consumer, ack policy)
- [x] T019: [P] Deduplication + applicator integration test in tests/integration/dedup_applicator_test.go (skip duplicates, OpID cleanup)

### E2E Tests (from quickstart.md) - Using Ginkgo v2
- [x] T020: [P] E2E basic 3-node replication test in tests/e2e/basic_replication_test.go (Scenario 1: data appears on all nodes < 100ms, no triggers)
- [x] T021: [P] E2E transaction atomicity test in tests/e2e/transaction_atomicity_test.go (Scenario 2: multi-statement transaction replicates atomically)
- [x] T022: [P] E2E node crash recovery test in tests/e2e/crash_recovery_test.go (Scenario 3: node restart, catch-up, no data loss)
- [x] T023: [P] E2E NATS unavailability test in tests/e2e/nats_unavailability_test.go (Scenario 4: buffer locally, replicate on reconnect)
- [x] T024: [P] E2E conflict resolution test in tests/e2e/conflict_resolution_test.go (Scenario 5: concurrent writes, LWW strategy, consistent state)
- [x] T025: [P] E2E database pristine test in tests/e2e/database_pristine_test.go (Scenario 6: no triggers/views, works standalone)
- [x] T026: [P] E2E replication control test in tests/e2e/replication_control_test.go (Scenario 7: stop process disables replication, restart enables)

## Phase 3.3: Core Implementation (ONLY after tests are failing)

### WAL Parser Module (NEW)
- [x] T027: Implement WAL frame structure and decoder in walparser/frame.go (Frame header, payload parsing, checksum calculation)
- [x] T028: Implement transaction buffer in walparser/buffer.go (in-memory buffering up to 1000 ops, disk-based spillover for large transactions)
- [x] T029: Implement checksum validator in walparser/checksum.go (validate frame checksums, detect corruption)
- [ ] T030: Implement Parser main logic in walparser/parser.go (NextTransaction, GetLSN, Checkpoint methods per contract, fsnotify WAL file monitoring)

### Checkpoint Module (NEW)
- [ ] T031: Implement LSN tracker in checkpoint/lsn.go (track parsed/replicated/checkpointed LSN, monotonic increase validation)
- [ ] T032: Implement state persistence in checkpoint/state.go (persist to NATS JetStream metadata AND local JSON file, dual-write)
- [ ] T033: Implement checkpoint manager in checkpoint/manager.go (coordinate checkpoint after replication confirmed, detect external checkpoints, force checkpoint when WAL > 100MB)

### Replication Module (MODIFIED - adapt existing stream/)
- [ ] T034: Refactor existing NATS publisher in replication/publisher.go (adapt for ReplicationMessage schema, publish to JetStream with OpID/NodeID/Lamport clock)
- [ ] T035: Refactor existing NATS subscriber in replication/subscriber.go (durable pull consumer, AckExplicit policy, consume ReplicationMessage)
- [ ] T036: Implement change applicator in replication/applicator.go (apply remote changes to local SQLite, preserve transaction atomicity, handle Before/After)
- [ ] T037: Implement conflict resolution in replication/conflict.go (LWW strategy using Lamport clock + wall time, counter strategy, append-only strategy, per-table config)
- [ ] T038: Implement Lamport clock in replication/lamport.go (Time/Increment/Witness interface per HashiCorp Serf pattern, atomic-based)

### Deduplication Module (NEW)
- [ ] T039: [P] Implement deduplication store interface in dedup/store.go (Has, Add, Cleanup methods)
- [ ] T040: [P] Implement SQLite-based deduplication in dedup/sqlite.go (persistent OpID storage, retention window cleanup, survive restarts)

### Configuration Module (EXTEND existing cfg/)
- [ ] T041: Extend configuration structs in cfg/config.go (add WAL parser settings, checkpoint thresholds, deduplication retention, conflict resolution strategies per table, including validation test for per-table strategy config mismatch per FR-041)
- [ ] T042: Implement WAL mode auto-enable in db/wal_enable.go (check journal mode, execute PRAGMA journal_mode=WAL if not in WAL mode)

### Health & Telemetry (EXTEND existing health/ and telemetry/)
- [ ] T043: Extend health checks in health/handlers.go (add parser status, NATS connection status, LSN lag to existing endpoints)
- [ ] T044: [P] Extend Prometheus metrics in telemetry/metrics.go (add harmonylite_cdc_last_lsn, harmonylite_replication_lag_seconds, harmonylite_nats_unavailable, harmonylite_conflicts_resolved_total)
- [ ] T045: [P] Add checkpoint/conflict logging in telemetry/logging.go (log all checkpoint operations with LSN+duration, log all conflict resolutions with strategy)

### Main Entry Point (MODIFIED)
- [ ] T046: Update main entry point in harmonylite.go (integrate WAL parser, checkpoint manager, deduplication, remove trigger-based CDC from db/)

## Phase 3.4: Integration & Cleanup

- [ ] T047: Integration test execution - Run all tests in tests/integration/ and verify they pass
- [ ] T048: E2E test execution - Run Ginkgo E2E suite in tests/e2e/ and verify all 7 scenarios pass
- [ ] T049: Remove trigger-based CDC code from db/ (delete change_log.go, change_log_event.go, *.tmpl files - no longer needed)

## Phase 3.5: Polish & Documentation

- [ ] T050: [P] Performance benchmark in tests/benchmarks/latency_test.go (measure p50/p95/p99 replication latency, verify < 100ms p95)
- [ ] T051: [P] Throughput benchmark in tests/benchmarks/throughput_test.go (measure sustained ops/sec, verify ≥ 1000 ops/sec)
- [ ] T052: [P] Create deployment guide in docs/deployment.md (Docker Compose, Kubernetes, systemd setup)
- [ ] T053: [P] Create PocketBase integration guide in docs/pocketbase.md (step-by-step HA setup, load balancer config)
- [ ] T054: [P] Create configuration reference in docs/configuration.md (all YAML parameters documented with examples)
- [ ] T055: [P] Create troubleshooting guide in docs/troubleshooting.md (common issues, diagnostics, recovery procedures, include operational runbooks subdirectory)
- [ ] T056: Execute quickstart.md validation - Manually run all 7 test scenarios from quickstart.md and verify pass
- [ ] T057: Code review and refactoring - Review all new code for simplicity, remove duplication, ensure constitutional compliance

## Dependencies

**Setup Phase (T001-T004)**: Sequential
- T001 → T002 → T003 → T004

**Test Phase (T005-T026)**: All parallel [P] - different test files, no dependencies

**Implementation Dependencies**:
- WAL Parser: T027 → T028 → T029 → T030 (frame before buffer before parser)
- Checkpoint: T031 → T032 → T033 (LSN tracker before state before manager)
- Replication: T034, T035, T036, T037, T038 can start in parallel, but T036 depends on T037 completion
- Deduplication: T039 and T040 parallel
- Config: T041 → T042 (config before WAL enable)
- Observability: T043, T044, T045 parallel
- Main: T046 depends on T030, T033, T036, T040, T041, T043 completion

**Integration Phase (T047-T049)**: Sequential
- T047 → T048 → T049 (integration tests → E2E tests → cleanup)

**Polish Phase (T050-T057)**: All parallel [P] except T056-T057
- T050-T055 parallel
- T056 → T057 (quickstart before code review)

## Parallel Execution Examples

### Launch all test tasks together (T005-T026):
```bash
# Contract tests
Task: "WAL Parser contract test in tests/unit/walparser_contract_test.go"
Task: "Replication message schema validation test in tests/unit/replication_message_test.go"
Task: "Health API contract test in tests/integration/health_api_test.go"

# Entity tests
Task: "Change entity validation test in tests/unit/change_test.go"
Task: "Transaction entity validation test in tests/unit/transaction_test.go"
Task: "LSN tracking test in tests/unit/lsn_test.go"

# ... (all other test tasks)
```

### Launch deduplication tasks together (T039-T040):
```bash
Task: "Implement deduplication store interface in dedup/store.go"
Task: "Implement SQLite-based deduplication in dedup/sqlite.go"
```

### Launch observability tasks together (T043-T045):
```bash
Task: "Extend health checks in health/handlers.go"
Task: "Extend Prometheus metrics in telemetry/metrics.go"
Task: "Add checkpoint/conflict logging in telemetry/logging.go"
```

### Launch documentation tasks together (T050-T055):
```bash
Task: "Performance benchmark in tests/benchmarks/latency_test.go"
Task: "Throughput benchmark in tests/benchmarks/throughput_test.go"
Task: "Create deployment guide in docs/deployment.md"
Task: "Create PocketBase integration guide in docs/pocketbase.md"
Task: "Create configuration reference in docs/configuration.md"
Task: "Create troubleshooting guide in docs/troubleshooting.md"
```

## Notes

- **TDD Enforcement**: Tests (T005-T026) MUST be written and MUST FAIL before implementing T027-T046
- **Existing Code**: HarmonyLite already has health/, telemetry/, stream/, db/ - we EXTEND not replace (except db/change_log* which we delete)
- **File Paths**: All paths are absolute from repository root `/Users/markb/dev/harmonylite-cdc/`
- **Ginkgo Framework**: All E2E tests (T020-T026) use Ginkgo v2 with Eventually assertions per constitutional requirement
- **Commit Cadence**: Commit after each task completion (not after phase completion)
- **Constitutional Compliance**: All tasks must maintain Zero Database Modification, Read-Only WAL Access, TDD discipline

## Validation Checklist
*GATE: Verify before marking tasks complete*

- [x] All contracts have corresponding tests (T005-T007 cover all 3 contract files)
- [x] All entities have model/validation tests (T008-T013 cover all 8 entities)
- [x] All quickstart scenarios have E2E tests (T020-T026 cover all 7 scenarios)
- [x] Tests come before implementation (T005-T026 before T027-T046)
- [x] Parallel tasks truly independent (all [P] tasks use different files)
- [x] Each task specifies exact file path (✅ all tasks have file paths)
- [x] No task modifies same file as another [P] task (✅ verified)

## Task Execution Strategy

1. **Week 1**: Setup (T001-T004) + Write all tests (T005-T026) - verify all tests FAIL
2. **Week 2-3**: Core implementation (T027-T040) - WAL parser, checkpoint, replication, dedup
3. **Week 3**: Configuration and observability (T041-T045) - extend existing modules
4. **Week 4**: Integration (T046-T049) - wire everything together, cleanup triggers
5. **Week 5**: Polish and validation (T050-T057) - benchmarks, docs, quickstart verification

**Total Tasks**: 57
**Parallel Opportunities**: 35 tasks marked [P]
**Estimated Duration**: 5-6 weeks with TDD discipline and thorough testing
