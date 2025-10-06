# Research: WAL-Based SQLite Replication System

## 1. SQLite WAL File Format Specification

**Decision**: Use official SQLite documentation (sqlite.org/walformat.html) as the authoritative reference for WAL format implementation.

**Rationale**: The official SQLite documentation provides complete, low-level details on the WAL file format, including header structure, frame format, checksum algorithm, and locking mechanisms. The documentation is current (last updated May 2025) and maintained by the SQLite team.

**Key Details**:
- WAL file consists of a 32-byte header followed by zero or more frames
- Each frame records the revised content of a single page from the database
- Checksum algorithm uses Fibonacci-weighted sums: iterates through 32-bit integers, computing `s0 += x(i) + s1` and `s1 += x(i+1) + s0`
- Byte order determined by magic number: 0x377f0683 (big-endian) or 0x377f0682 (little-endian)
- WAL-index (SHM file) contains critical metadata:
  - mxFrame: number of valid committed frames in WAL (offset 16)
  - nBackfill: number of frames backfilled to main DB (offset 96)
  - Read-marks and locks for coordination (bytes 100-127)
- WAL file is always located in same directory as database with "-wal" suffix

**Alternatives**: Third-party documentation sources (runebook.dev, Medium articles) exist but lack the depth and authority of the official specification.

---

## 2. Go SQLite Libraries

**Decision**: Use modernc.org/sqlite for pure Go implementation.

**Rationale**: While mattn/go-sqlite3 offers better performance (10-50% faster), modernc.org/sqlite provides a CGo-free implementation that simplifies cross-compilation and deployment. Both libraries support full PRAGMA functionality with feature parity to the C SQLite library. The performance difference is acceptable for a replication system where network I/O will likely be the bottleneck.

**Key Details**:
- modernc.org/sqlite registers driver name "sqlite" by default
- Full PRAGMA support including critical settings:
  - `PRAGMA journal_mode = WAL` for enabling WAL mode
  - `PRAGMA synchronous = NORMAL` (safe with WAL)
  - `PRAGMA busy_timeout = 5000` for handling contention
  - `PRAGMA foreign_keys = ON` for referential integrity
- Use `RegisterConnectionHook()` to set PRAGMAs on every connection from pool
- CGo-free simplifies deployment across platforms
- Performance: 2-10x slower than mattn/go-sqlite3 but acceptable for CDC use case

**Alternatives**: mattn/go-sqlite3 requires CGo but offers superior performance. Consider if performance profiling reveals SQLite operations as a bottleneck.

---

## 3. NATS JetStream Best Practices

**Decision**: Use durable pull consumers with AckExplicit policy and persistent stream configuration for reliable CDC event delivery.

**Rationale**: Pull consumers provide fine-grained control over message processing, prevent slow consumer issues, and support horizontal scaling. Durable consumers maintain server-side state, ensuring no messages are lost across restarts. AckExplicit provides the strongest delivery guarantee, essential for maintaining data consistency.

**Key Details**:

**Consumer Configuration**:
- Create durable consumers with explicit names: `ConsumerConfig{Durable: "cdc-consumer", AckPolicy: jetstream.AckExplicitPolicy}`
- Set `MaxAckPending` to control outstanding unacknowledged messages (default: 1000)
- Configure `AckWait` for redelivery timeout (time to acknowledge before redelivery)
- Use `MaxDeliver` to limit redelivery attempts

**LSN Persistence Approach**:
- Store last processed LSN (mxFrame from WAL) in consumer's FilterSubject or as application-level metadata
- Use consumer's built-in durable state for message position tracking
- JetStream automatically maintains consumer position server-side
- On restart, consumer resumes from last acknowledged message

**Pull Consumer Patterns**:
- Use `Consume()` for continuous callback-based processing
- Use `Messages()` for iterator-based control (better for work queues)
- Use `Fetch(n)` for batch processing with explicit batch sizes
- Configure `PullHeartbeat` to detect stalled connections (idle heartbeat duration)

**Stream Configuration**:
- Use `Limits` to prevent unbounded growth: `MaxAge`, `MaxBytes`, `MaxMsgs`
- Set `Retention: WorkQueuePolicy` for work queue semantics or `Retention: LimitsPolicy` for replay capability
- Configure `Replicas: 3` for high availability in clustered NATS

**Alternatives**: Push consumers are simpler but less flexible and harder to scale horizontally. Not recommended for new CDC implementations.

---

## 4. Lamport Clock Implementation

**Decision**: Adopt HashiCorp Serf's atomic-based Lamport clock pattern with Time(), Increment(), and Witness() methods.

**Rationale**: This pattern is proven in production distributed systems (Consul, Nomad), uses efficient atomic operations for performance, and provides a clean interface for causality tracking. The implementation is simple, thread-safe, and suitable for tracking event ordering in CDC.

**Key Details**:

**Interface Design**:
```go
type LamportClock interface {
    Time() uint64           // Returns current clock value
    Increment() uint64      // Increments and returns new value
    Witness(uint64) uint64  // Updates clock based on received timestamp
}
```

**Implementation Pattern**:
```go
type MemClock struct {
    counter uint64
}

func (c *MemClock) Time() uint64 {
    return atomic.LoadUint64(&c.counter)
}

func (c *MemClock) Increment() uint64 {
    return atomic.AddUint64(&c.counter, 1)
}

func (c *MemClock) Witness(v uint64) uint64 {
    for {
        cur := atomic.LoadUint64(&c.counter)
        next := v
        if cur > v {
            next = cur
        }
        next++
        if atomic.CompareAndSwapUint64(&c.counter, cur, next) {
            return next
        }
    }
}
```

**Usage in CDC**:
- Increment clock when capturing local WAL changes
- Witness clock when receiving remote events via NATS
- Include Lamport timestamp in every CDC event message
- Use for total ordering of events across replicas

**Alternatives**: Vector clocks provide better causality tracking but require O(N) space for N nodes and more complex conflict resolution. Hybrid Logical Clocks (HLC) combine physical and logical time but add complexity. Lamport clocks are sufficient for most CDC ordering requirements.

---

## 5. fsnotify Library for Filesystem Notifications

**Decision**: Use fsnotify to watch the WAL file's parent directory, filtering events by filename to detect WAL changes.

**Rationale**: fsnotify is the de facto standard for filesystem notifications in Go, with cross-platform support (Linux inotify, macOS FSEvents, Windows ReadDirectoryChangesW). The library is mature, well-maintained, and handles platform-specific edge cases.

**Key Details**:

**Watch Pattern**:
- Watch the parent directory of the database file, not the WAL file directly
- Reason: Many tools update files atomically (write to temp, then rename), which would break file-level watches
- Filter events by `Event.Name` to match WAL filename pattern (e.g., "mydb.db-wal")

**Event Handling**:
- Listen for `fsnotify.Write` events primarily
- Ignore `fsnotify.Chmod` events (often noisy and not useful for CDC)
- Handle `fsnotify.Remove` events to detect WAL deletion/checkpoint

**Best Practices**:
- Set `FSNOTIFY_DEBUG=1` environment variable for troubleshooting
- Be aware of system limits (Linux: `fs.inotify.max_user_watches` sysctl)
- Watches are automatically removed when watched path is deleted/renamed
- Cannot watch NFS/SMB mounts (no network-level notification support)
- Cannot watch paths that don't exist yet; watch parent and add child when created

**Debouncing**:
- Implement a short debounce (100-500ms) to coalesce rapid successive writes
- WAL writes can be frequent; avoid processing every individual write event

**Alternatives**: Polling with stat() is simpler but less efficient and introduces latency. OS-specific APIs (inotify, kqueue) offer more control but lose cross-platform compatibility.

---

## 6. Deduplication Store Options

**Decision**: Use SQLite for the deduplication store.

**Rationale**: SQLite provides the best balance of simplicity, durability, and query flexibility for a CDC system. Reusing SQLite means one less dependency, consistent tooling, and the ability to perform complex queries (e.g., time-based expiration). For a CDC system already dependent on SQLite, adding another embedded database adds unnecessary complexity.

**Key Details**:

**SQLite Deduplication Store**:
- Store message IDs with timestamp: `CREATE TABLE dedup (msg_id TEXT PRIMARY KEY, seen_at INTEGER)`
- Use WAL mode for the dedup database as well for consistency
- Implement TTL-based cleanup: `DELETE FROM dedup WHERE seen_at < ?`
- Simple SQL queries for checking and inserting: `INSERT OR IGNORE INTO dedup VALUES (?, ?)`
- Leverage ACID transactions for consistency
- Disk-backed with automatic persistence

**Performance Characteristics**:
- SQLite: Excellent read performance, good write performance with WAL mode
- Space overhead: Minimal (primary key index + timestamp column)
- Cleanup: Periodic DELETE operations with VACUUM to reclaim space

**Alternatives Considered**:
- **BoltDB (bbolt)**: 10-20x slower writes than Badger but excellent read performance and low memory usage. Good choice for memory-constrained environments but SQLite already provides similar benefits.
- **BadgerDB**: 10-22x faster writes than BoltDB, best for high-throughput scenarios. However, significantly higher memory usage (major drawback). Recommended only if profiling shows deduplication as a write bottleneck and memory is plentiful.
- In-memory map with periodic snapshots: Risky for durability; deduplication state loss could cause duplicate event processing.

---

## 7. Chaos Engineering Tools

**Decision**: Use Toxiproxy for network fault injection during E2E testing.

**Rationale**: Toxiproxy is purpose-built for testing distributed systems under adverse network conditions. Written in Go, it integrates seamlessly with Go test suites via the official Testcontainers module. Toxiproxy provides a clean HTTP API for controlling network conditions dynamically during tests without requiring root privileges or iptables manipulation.

**Key Details**:

**Toxiproxy Architecture**:
- TCP proxy that sits between application components
- Configure application to connect through proxy instead of direct endpoints
- Apply "toxics" to simulate network failures: latency, bandwidth limits, disconnections, timeouts

**Available Toxics**:
- `latency`: Add fixed or jittered latency
- `down`: Stop all data flow (simulate total network partition)
- `bandwidth`: Limit bandwidth (bytes per second)
- `slow_close`: Delay connection close
- `timeout`: Stop all data after a timeout
- `reset_peer`: Simulate TCP RST
- `limit_data`: Close connection after N bytes
- `slicer`: Slice data into smaller packets

**Go Integration**:
```go
import (
    "github.com/testcontainers/testcontainers-go/modules/toxiproxy"
)

// Create Toxiproxy container
toxiproxyContainer, _ := toxiproxy.RunContainer(ctx)

// Create proxy for NATS
proxy, _ := toxiproxyContainer.CreateProxy(ctx, "nats", "nats:4222")

// Add toxic to simulate network partition
proxy.AddToxic(ctx, "latency_down", "latency", "downstream", 1.0,
    toxiproxy.LatencyToxicAttrs{Latency: 1000, Jitter: 0})

// Remove toxic to restore connection
proxy.RemoveToxic(ctx, "latency_down")
```

**Testing Scenarios**:
- NATS partition: Verify buffering and reconnection logic
- WAL file read delays: Test timeout handling
- Slow replication lag: Validate backpressure mechanisms
- Split-brain scenarios: Test conflict resolution

**Alternatives**:
- iptables/tc (Linux traffic control): Requires root privileges, complex setup, not portable to CI environments
- Manual mock implementations: Time-consuming, incomplete coverage of real network behavior
- Docker network manipulation: Less granular control, harder to coordinate timing

---

## 8. Prometheus Go Client

**Decision**: Use prometheus/client_golang with custom registry for CDC-specific metrics.

**Rationale**: prometheus/client_golang is the official Prometheus instrumentation library for Go, providing a clean API for exposing metrics. Using a custom registry allows selective metric exposure, avoiding clutter from default Go runtime metrics unless explicitly needed.

**Key Details**:

**Basic Setup**:
```go
import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
    "github.com/prometheus/client_golang/prometheus/promhttp"
)

// Expose /metrics endpoint
http.Handle("/metrics", promhttp.Handler())
```

**Metric Types for CDC**:
- **Counter**: `replication_events_total{type="insert|update|delete"}` - Total events processed
- **Gauge**: `replication_lag_seconds` - Current replication lag
- **Histogram**: `event_processing_duration_seconds` - Event processing latency distribution
- **Gauge**: `wal_frame_position` - Current WAL frame number (LSN)
- **Counter**: `deduplication_hits_total` - Duplicate events detected
- **Gauge**: `nats_connection_status{status="connected|disconnected"}` - Connection health

**Using promauto for Auto-Registration**:
```go
var (
    eventsProcessed = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "cdc_events_processed_total",
        Help: "Total number of CDC events processed",
    }, []string{"type"})

    replicationLag = promauto.NewGauge(prometheus.GaugeOpts{
        Name: "cdc_replication_lag_seconds",
        Help: "Current replication lag in seconds",
    })
)

// Usage
eventsProcessed.WithLabelValues("insert").Inc()
replicationLag.Set(0.042)
```

**Custom Registry Pattern** (to disable default metrics):
```go
reg := prometheus.NewRegistry()
counter := prometheus.NewCounter(prometheus.CounterOpts{
    Name: "cdc_events_total",
    Help: "Total CDC events",
})
reg.MustRegister(counter)

// Expose only custom metrics
http.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
```

**Alternatives**: OpenTelemetry provides more comprehensive observability but adds complexity. For metrics-only requirements, prometheus/client_golang is simpler and more focused.

---

## 9. Structured Logging: zerolog vs zap

**Decision**: Use zerolog for high-throughput structured logging.

**Rationale**: Zerolog is the fastest structured logging library for Go, with zero allocations in most scenarios and minimal memory footprint (40 B/op vs zap's 168 B/op). For a CDC system with high event throughput, zerolog's performance advantage is significant. While zap offers more customization options, zerolog's simplicity and speed make it the better choice for this use case.

**Key Details**:

**Performance Comparison**:
- **Speed**: zerolog is fastest in all scenarios (per Uber-go benchmarks)
- **Memory**: zerolog allocates 40 B/op, zap allocates 168 B/op
- **Allocations**: zerolog has 1 alloc/op, zap has 3 allocs/op
- **Zero allocation**: zerolog achieves true zero-allocation logging in hot paths

**Basic Usage**:
```go
import (
    "github.com/rs/zerolog"
    "github.com/rs/zerolog/log"
)

// Global logger
log.Info().
    Str("wal_file", "mydb.db-wal").
    Uint64("frame", 42).
    Msg("Processing WAL frame")

// Context logger
logger := log.With().
    Str("component", "wal-watcher").
    Logger()

logger.Error().
    Err(err).
    Msg("Failed to read WAL file")
```

**Structured Fields for CDC**:
- `component`: "wal-watcher", "nats-publisher", "dedup-store"
- `event_type`: "insert", "update", "delete"
- `wal_frame`: Frame sequence number
- `lamport_ts`: Lamport timestamp
- `duration_ms`: Operation duration
- `error`: Error information

**Best Practices**:
- Use `zerolog.Logger` as a struct field to maintain context
- Enable sampling for high-frequency debug logs: `logger.Sample(&zerolog.BurstSampler{Burst: 5, Period: 1*time.Second})`
- Use `log.Ctx(ctx)` to extract logger from context in HTTP handlers
- Set global level: `zerolog.SetGlobalLevel(zerolog.InfoLevel)`

**Alternatives**: zap is more feature-rich with better customization (custom encoders, advanced sampling). Consider zap if you need these features and performance difference is not critical. Standard library slog (Go 1.21+) is a good middle ground but slower than both zerolog and zap.

---

## 10. Ginkgo v2 Testing Framework

**Decision**: Use Ginkgo v2 with Gomega for E2E testing, following Kubernetes project patterns.

**Rationale**: Ginkgo v2 provides a mature, feature-rich BDD testing framework with excellent support for parallel execution, sophisticated lifecycle hooks, and clear test organization. The Kubernetes project's extensive use of Ginkgo for E2E tests demonstrates its suitability for complex distributed system testing. Gomega's matcher library provides expressive assertions.

**Key Details**:

**Test Organization**:
```go
import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

var _ = Describe("CDC Replication", func() {
    var (
        sourceDB *sql.DB
        replicaDB *sql.DB
        natsConn *nats.Conn
    )

    BeforeEach(func() {
        // Setup test fixtures
        sourceDB = setupDatabase()
        replicaDB = setupDatabase()
        natsConn = setupNATS()
    })

    AfterEach(func() {
        // Cleanup
        sourceDB.Close()
        replicaDB.Close()
        natsConn.Close()
    })

    Context("when source database is updated", func() {
        It("should replicate changes to replica", func() {
            // Insert data on source
            _, err := sourceDB.Exec("INSERT INTO users VALUES (1, 'Alice')")
            Expect(err).ToNot(HaveOccurred())

            // Verify replication
            Eventually(func() int {
                var count int
                replicaDB.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
                return count
            }, "10s", "100ms").Should(Equal(1))
        })
    })
})
```

**E2E Testing Patterns**:

1. **Use Eventually for Asynchronous Assertions**:
   - `Eventually(func() bool { ... }, timeout, interval).Should(BeTrue())`
   - Essential for testing eventual consistency in replication

2. **Context-Based Test Organization**:
   - `Describe` for feature-level grouping
   - `Context` for scenario variations
   - `It` for individual test cases

3. **DeferCleanup Pattern** (Ginkgo v2):
   - Use `DeferCleanup(func() { ... })` instead of AfterEach for cleanup
   - Important: Don't reuse ctx passed to `ginkgo.It` in DeferCleanup (it gets canceled)

4. **Parallel Execution**:
   - Run specs with `ginkgo -p` for parallelization
   - Use `Ordered` container for tests that must run sequentially
   - Design tests to be independent and reproducible

5. **Framework-Specific Assertions**:
   - Use `framework.ExpectNoError(err)` pattern (from Kubernetes) for better error logging
   - Logs full error details before failure
   - More informative than `Expect(err).ToNot(HaveOccurred())`

**Integration with Testing Tools**:
```go
// Testcontainers for infrastructure
Describe("Full E2E Test", func() {
    var toxiproxy *toxiproxy.ToxiproxyContainer

    BeforeEach(func() {
        toxiproxy, _ = toxiproxy.RunContainer(ctx)
        DeferCleanup(func() {
            toxiproxy.Terminate(context.Background())
        })
    })

    It("should handle network partitions", func() {
        // Create partition
        proxy, _ := toxiproxy.CreateProxy(ctx, "nats", "nats:4222")
        proxy.AddToxic(ctx, "down", "down", "downstream", 1.0, nil)

        // Test behavior under partition
        // ...

        // Restore connection
        proxy.RemoveToxic(ctx, "down")

        // Verify recovery
        Eventually(func() bool {
            return checkReplicationHealth()
        }).Should(BeTrue())
    })
})
```

**Best Practices**:
- Use `GinkgoWriter` for test output (properly synchronized with test execution)
- Use `By()` to document test steps: `By("inserting test data")`
- Set appropriate timeouts with `SetDefaultEventuallyTimeout()`
- Use `Focus` (`FDescribe`, `FIt`) for running specific tests during development
- Use labels for selective test execution: `It("test", Label("slow"), func() { ... })`

**Alternatives**: Standard Go testing is simpler but lacks BDD structure and built-in parallel execution support. Testify provides assertions but not test organization. For complex E2E scenarios with multiple moving parts, Ginkgo's organization and lifecycle management are valuable.

---

## Summary

This research provides a comprehensive technical foundation for implementing a WAL-based SQLite replication system in Go. Key decisions favor:

1. **Simplicity**: Pure Go dependencies (modernc.org/sqlite, fsnotify) over CGo where acceptable
2. **Reliability**: Durable pull consumers, explicit acknowledgments, deduplication
3. **Performance**: zerolog for logging, atomic-based Lamport clocks, efficient notification patterns
4. **Testability**: Ginkgo v2 + Toxiproxy for comprehensive E2E testing under adverse conditions
5. **Observability**: Prometheus metrics and structured logging for operational visibility

All choices prioritize correctness and operational simplicity over maximum theoretical performance, appropriate for a CDC system where data consistency is paramount.
