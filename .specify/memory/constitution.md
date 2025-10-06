<!--
Sync Impact Report:
Version: NONE → 1.0.0
Rationale: Initial constitution creation based on HarmonyLite development practices
Modified Principles: N/A (initial creation)
Added Sections:
  - Core Principles (7 principles)
  - Quality Standards
  - Development Workflow
  - Governance
Removed Sections: N/A
Templates Status:
  - ✅ .specify/templates/plan-template.md (references constitution check)
  - ✅ .specify/templates/spec-template.md (aligned with requirements approach)
  - ✅ .specify/templates/tasks-template.md (aligned with TDD and testing)
Follow-up TODOs: None
-->

# HarmonyLite CDC Constitution

## Core Principles

### I. Zero Database Modification (NON-NEGOTIABLE)

The replication system MUST operate without modifying the SQLite database schema, structure, or
contents. No triggers, views, or CDC infrastructure may be added to the target database. This
principle ensures trust, transparency, and clean migration paths for users.

**Rationale**: Users must be able to verify that their database remains pristine. Database copies
and backups should not contain replication artifacts. Removing replication should be trivial
(stop the process).

### II. WAL-Based Change Detection

All change detection MUST be performed by parsing SQLite Write-Ahead Log (WAL) files externally.
The parser operates with read-only access to WAL files and never writes to .db, .db-wal, or
.db-shm files.

**Rationale**: WAL mode provides a non-invasive alternative to trigger-based CDC. External parsing
eliminates per-write overhead and maintains database integrity guarantees.

### III. Test-First Development (NON-NEGOTIABLE)

All code MUST follow Test-Driven Development (TDD): write tests, get user approval, verify tests
fail, then implement. The Red-Green-Refactor cycle is strictly enforced.

**Testing Requirements**:
- Unit tests for WAL parsing, frame decoding, transaction reconstruction
- Integration tests for multi-node replication scenarios
- End-to-End tests using Ginkgo framework (following HarmonyLite practices)
- Chaos engineering tests for network partitions and failures
- Performance benchmarks with published results

**Rationale**: Distributed replication systems have complex failure modes. Comprehensive testing
is the only way to ensure correctness and prevent data loss.

### IV. Transaction Atomicity

Multi-statement transactions MUST replicate atomically across all nodes. The system MUST detect
transaction boundaries via WAL commit frames and MUST NOT replicate partial transactions.

**Requirements**:
- Buffer operations until commit frame detected
- Discard buffered operations on rollback
- Apply remote transactions atomically (all or nothing)
- Support unlimited transaction size via disk-based buffering

**Rationale**: Breaking transaction atomicity leads to inconsistent state across nodes and violates
fundamental database guarantees.

### V. Observability and Operations

The system MUST provide comprehensive observability through metrics, logs, and health checks to
enable production operations.

**Requirements**:
- Prometheus metrics on /metrics endpoint (LSN positions, lag, throughput, errors)
- Structured JSON logging with appropriate levels (DEBUG, INFO, WARN, ERROR)
- Health check endpoints: /health/live, /health/ready, /health/status
- All checkpoint operations logged with LSN and duration
- All conflict resolutions logged with strategy used

**Rationale**: Distributed systems fail in complex ways. Operators need visibility to detect issues
before data loss occurs and to troubleshoot problems effectively.

### VI. Performance and Scalability

The system MUST achieve production-grade performance with minimal overhead compared to
non-replicated SQLite.

**Targets**:
- p95 replication latency < 100ms for typical workloads
- Sustained throughput ≥ 1000 ops/sec per node
- CPU overhead ≤ 10% vs non-replicated SQLite
- Memory usage ≤ 100MB base + transaction buffers

**Rationale**: Replication that degrades application performance to unacceptable levels will not
be adopted. Performance targets reflect real-world PocketBase deployment requirements.

### VII. Simplicity and Minimal Configuration

Following HarmonyLite's philosophy, the system MUST be designed for simplicity with minimal
configuration required for common use cases.

**Requirements**:
- Sensible defaults for all configuration parameters
- Single TOML or YAML configuration file
- Clear error messages with actionable guidance
- Documentation that enables self-service deployment
- No unnecessary abstractions or complexity

**Rationale**: Complex systems are harder to operate, debug, and maintain. Users should be able to
deploy successfully without deep expertise in distributed systems.

## Quality Standards

### Error Handling Priorities

Error handling MUST be prioritized by likelihood and impact:

1. **Priority 1 (HIGHEST)**: Network issues (NATS unavailable, partitions, slow network)
2. **Priority 2 (HIGH)**: Node crashes (parser, application, full node)
3. **Priority 3 (MEDIUM)**: Data issues (large BLOBs, write bursts, clock skew)
4. **Priority 4 (LOW)**: Storage issues (disk full, WAL corruption, filesystem errors)

All Priority 1-2 scenarios MUST have automated recovery without manual intervention.
Priority 3-4 scenarios MUST emit clear alerts and provide documented recovery procedures.

### Security Requirements

- Parser MUST run with read-only access to WAL files
- No writes to database files (.db, .db-wal, .db-shm)
- NATS authentication via credentials file supported
- TLS for NATS connections supported
- Sensitive data (credentials, tokens) MUST NEVER be logged

### Compatibility and Versioning

- Support SQLite 3.37+ (document minimum version)
- Support Go 1.21+ (use latest stable)
- Support NATS 2.9+ with JetStream enabled
- Follow semantic versioning: MAJOR.MINOR.PATCH
- Maintain backward compatibility for NATS message formats

## Development Workflow

### Build and Version Management

- Use Makefile for build automation (HarmonyLite practice)
- Automatic version injection during build:
  - Semantic version number
  - Git commit hash
  - Build timestamp
  - Go version and platform
- Build command must succeed on clean checkout

### Code Organization

Project MUST follow modular structure:
- `walparser/`: WAL parsing library (standalone, independently testable)
- `checkpoint/`: Checkpoint coordination logic
- `replication/`: NATS integration and message handling
- `dedup/`: OpID deduplication store
- `config/`: Configuration loading and validation
- `health/`: Health check endpoints
- `telemetry/`: Metrics and logging
- `tests/e2e/`: End-to-end tests using Ginkgo

### Documentation Standards

Documentation MUST enable self-service deployment:
- Deployment guide (Docker Compose, Kubernetes, systemd)
- PocketBase integration guide with step-by-step instructions
- Configuration reference with all parameters documented
- Troubleshooting guide and operational runbooks
- API documentation (GoDoc) for all public interfaces

### Git and Release Practices

- Commit messages: conventional commits format
- Branch naming: feature/*, bugfix/*, release/*
- Pull requests require passing tests and code review
- Releases tagged with semantic version
- Release notes document changes and migration steps

## Governance

### Constitutional Authority

This constitution supersedes all other development practices and guidelines. When conflicts arise
between this document and other documentation, the constitution takes precedence.

### Amendment Process

Amendments to this constitution require:
1. Documented proposal with rationale
2. Impact analysis on existing code and workflows
3. Migration plan if breaking changes introduced
4. Approval process (to be defined based on project governance model)

### Compliance Verification

All pull requests and code reviews MUST verify compliance with constitutional principles.
Violations require explicit justification documented in Complexity Tracking section of plan.md.

Use `.specify/memory/constitution.md` as authoritative source for all planning and implementation
decisions. Templates in `.specify/templates/` reference this constitution for validation gates.

### Version Control

Version increments follow semantic versioning:
- **MAJOR**: Backward incompatible principle removals or redefinitions
- **MINOR**: New principle added or materially expanded guidance
- **PATCH**: Clarifications, wording, typo fixes, non-semantic refinements

**Version**: 1.0.0 | **Ratified**: 2025-10-06 | **Last Amended**: 2025-10-06
