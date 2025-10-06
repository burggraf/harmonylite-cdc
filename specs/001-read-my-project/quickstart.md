# Quickstart: WAL-Based SQLite Replication System

## Overview

This quickstart guide validates the core replication scenarios from the feature specification. These tests should be executed manually after implementation to verify the system meets acceptance criteria.

## Prerequisites

- 3 nodes with HarmonyLite CDC installed
- NATS JetStream cluster running
- PocketBase installed (for integration testing)
- Network access between all nodes
- Linux environment with iptables (for partition testing)

## Test Scenario 1: Basic 3-Node Replication

**Acceptance Criteria**: Data written to node 1 appears on nodes 2 and 3 within 100ms without database triggers

### Setup

```bash
# Terminal 1: Start NATS JetStream
docker run -d --name nats -p 4222:4222 nats:latest -js

# Terminal 2: Start Node 1
./harmonylite-cdc --config node1.yaml

# Terminal 3: Start Node 2
./harmonylite-cdc --config node2.yaml

# Terminal 4: Start Node 3
./harmonylite-cdc --config node3.yaml
```

### Test Steps

1. **Write data on Node 1**:
   ```bash
   sqlite3 /data/node1/test.db "INSERT INTO users (id, name) VALUES ('user-1', 'Alice');"
   ```

2. **Verify on Node 2** (within 100ms):
   ```bash
   sqlite3 /data/node2/test.db "SELECT * FROM users WHERE id = 'user-1';"
   # Expected: user-1|Alice
   ```

3. **Verify on Node 3** (within 100ms):
   ```bash
   sqlite3 /data/node3/test.db "SELECT * FROM users WHERE id = 'user-1';"
   # Expected: user-1|Alice
   ```

4. **Check no triggers created**:
   ```bash
   sqlite3 /data/node2/test.db "SELECT name FROM sqlite_master WHERE type='trigger';"
   # Expected: (empty result)
   ```

### Success Criteria
- ✅ Data appears on all nodes within 100ms
- ✅ No triggers exist in database schema
- ✅ Prometheus metric `harmonylite_replication_latency_seconds` p95 < 0.1

## Test Scenario 2: Transaction Atomicity

**Acceptance Criteria**: Multi-statement transaction replicates atomically (all or nothing)

### Test Steps

1. **Begin transaction on Node 1**:
   ```bash
   sqlite3 /data/node1/test.db <<EOF
   BEGIN TRANSACTION;
   INSERT INTO users (id, name) VALUES ('user-2', 'Bob');
   INSERT INTO users (id, name) VALUES ('user-3', 'Charlie');
   UPDATE users SET name = 'Robert' WHERE id = 'user-2';
   COMMIT;
   EOF
   ```

2. **Verify atomicity on Node 2**:
   ```bash
   sqlite3 /data/node2/test.db "SELECT * FROM users WHERE id IN ('user-2', 'user-3') ORDER BY id;"
   # Expected: user-2|Robert
   #           user-3|Charlie
   ```

3. **Check transaction ID grouping in logs**:
   ```bash
   grep "transaction_id" /var/log/harmonylite-cdc/node2.log | tail -3
   # Expected: All 3 operations share same transaction_id
   ```

### Success Criteria
- ✅ All operations appear together (no partial state)
- ✅ Operations applied in correct order
- ✅ Same transaction_id in logs for all operations

## Test Scenario 3: Node Crash Recovery

**Acceptance Criteria**: Node restarts after crash and automatically catches up without data loss

### Test Steps

1. **Kill Node 2 mid-replication**:
   ```bash
   # Write 1000 operations on Node 1
   for i in {1..1000}; do
     sqlite3 /data/node1/test.db "INSERT INTO events (id, data) VALUES ($i, 'event-$i');"
   done &

   # Kill Node 2 after ~500 operations
   sleep 1
   kill -9 $(pgrep -f "harmonylite-cdc.*node2")
   ```

2. **Restart Node 2**:
   ```bash
   ./harmonylite-cdc --config node2.yaml
   ```

3. **Wait for catch-up** (check logs):
   ```bash
   tail -f /var/log/harmonylite-cdc/node2.log | grep "catch-up complete"
   ```

4. **Verify all 1000 operations applied**:
   ```bash
   sqlite3 /data/node2/test.db "SELECT COUNT(*) FROM events;"
   # Expected: 1000
   ```

### Success Criteria
- ✅ Node 2 resumes from last replicated LSN
- ✅ All missed operations applied
- ✅ No duplicate operations (deduplication working)
- ✅ Recovery time < 5 seconds

## Test Scenario 4: NATS Unavailability

**Acceptance Criteria**: Nodes buffer changes locally when NATS unavailable, replicate on reconnect

### Test Steps

1. **Stop NATS server**:
   ```bash
   docker stop nats
   ```

2. **Write data on Node 1** (NATS unavailable):
   ```bash
   for i in {1..100}; do
     sqlite3 /data/node1/test.db "INSERT INTO events (id, data) VALUES ($i, 'buffered-$i');"
   done
   ```

3. **Check local buffering**:
   ```bash
   curl http://localhost:8080/health/status | jq '.nats.connected'
   # Expected: false

   tail /var/log/harmonylite-cdc/node1.log | grep "buffering locally"
   # Expected: log entries showing local buffering active
   ```

4. **Restart NATS**:
   ```bash
   docker start nats
   ```

5. **Wait for reconnection**:
   ```bash
   tail -f /var/log/harmonylite-cdc/node1.log | grep "NATS reconnected"
   ```

6. **Verify buffered changes replicated**:
   ```bash
   sqlite3 /data/node2/test.db "SELECT COUNT(*) FROM events WHERE data LIKE 'buffered-%';"
   # Expected: 100
   ```

### Success Criteria
- ✅ Changes buffered locally during NATS outage
- ✅ All buffered changes replicated on reconnect
- ✅ No data loss
- ✅ Metric `harmonylite_nats_unavailable` == 0 after reconnect

## Test Scenario 5: Conflict Resolution (LWW)

**Acceptance Criteria**: Concurrent writes to same row produce consistent state via conflict resolution

### Test Steps

1. **Partition network** (isolate Node 1 from Node 2):
   ```bash
   # On Node 1
   sudo iptables -A INPUT -s <node2-ip> -j DROP
   sudo iptables -A OUTPUT -d <node2-ip> -j DROP
   ```

2. **Write to same row on both nodes**:
   ```bash
   # Node 1
   sqlite3 /data/node1/test.db "UPDATE users SET name = 'Alice-Node1' WHERE id = 'user-1';"

   # Node 2
   sqlite3 /data/node2/test.db "UPDATE users SET name = 'Alice-Node2' WHERE id = 'user-1';"
   ```

3. **Heal partition**:
   ```bash
   sudo iptables -D INPUT -s <node2-ip> -j DROP
   sudo iptables -D OUTPUT -d <node2-ip> -j DROP
   ```

4. **Wait for reconciliation**:
   ```bash
   sleep 5
   ```

5. **Verify consistent state** (both nodes same):
   ```bash
   # Node 1
   sqlite3 /data/node1/test.db "SELECT name FROM users WHERE id = 'user-1';"

   # Node 2
   sqlite3 /data/node2/test.db "SELECT name FROM users WHERE id = 'user-1';"

   # Expected: Same value on both (winner determined by Lamport clock + wall time)
   ```

6. **Check conflict resolution logs**:
   ```bash
   grep "conflict resolved" /var/log/harmonylite-cdc/*.log
   # Expected: Log entries showing LWW strategy applied
   ```

### Success Criteria
- ✅ Both nodes converge to same value
- ✅ Lamport clock determines winner
- ✅ Conflict logged with strategy used (FR-062)
- ✅ Metric `harmonylite_conflicts_resolved_total{strategy="lww"}` incremented

## Test Scenario 6: Database Pristine (No Modifications)

**Acceptance Criteria**: Database contains no replication artifacts, usable independently

### Test Steps

1. **Inspect schema**:
   ```bash
   sqlite3 /data/node1/test.db ".schema"
   # Expected: Only application tables, no _replication_* tables
   ```

2. **Check for triggers**:
   ```bash
   sqlite3 /data/node1/test.db "SELECT name FROM sqlite_master WHERE type='trigger';"
   # Expected: (empty)
   ```

3. **Check for views**:
   ```bash
   sqlite3 /data/node1/test.db "SELECT name FROM sqlite_master WHERE type='view' AND name LIKE '%repl%';"
   # Expected: (empty)
   ```

4. **Copy database file**:
   ```bash
   cp /data/node1/test.db /tmp/standalone.db
   sqlite3 /tmp/standalone.db "SELECT COUNT(*) FROM users;"
   # Expected: Works perfectly without replication
   ```

### Success Criteria
- ✅ No triggers in schema (FR-001)
- ✅ No replication tables/views
- ✅ Database file works standalone
- ✅ No cleanup required to disable replication

## Test Scenario 7: Replication Disable/Enable

**Acceptance Criteria**: Process restart required to disable replication (per clarification)

### Test Steps

1. **Stop replication process**:
   ```bash
   pkill -f "harmonylite-cdc.*node1"
   ```

2. **Write data without replication**:
   ```bash
   sqlite3 /data/node1/test.db "INSERT INTO users (id, name) VALUES ('local-only', 'Local User');"
   ```

3. **Verify not replicated**:
   ```bash
   sqlite3 /data/node2/test.db "SELECT * FROM users WHERE id = 'local-only';"
   # Expected: (empty - not replicated)
   ```

4. **Restart replication**:
   ```bash
   ./harmonylite-cdc --config node1.yaml
   ```

5. **Verify catch-up**:
   ```bash
   sleep 2
   sqlite3 /data/node2/test.db "SELECT * FROM users WHERE id = 'local-only';"
   # Expected: local-only|Local User (replicated after restart)
   ```

### Success Criteria
- ✅ Stopping process disables replication
- ✅ Database remains functional without replication
- ✅ Restart enables replication again
- ✅ Missed operations replicated on restart

## Performance Validation

### Latency Test

```bash
# Write 1000 operations, measure replication latency
./scripts/benchmark-latency.sh --nodes 3 --operations 1000

# Expected output:
# p50: <50ms
# p95: <100ms
# p99: <200ms
```

### Throughput Test

```bash
# Sustained write load
./scripts/benchmark-throughput.sh --duration 60s --target 1000

# Expected output:
# Sustained: >1000 ops/sec
# CPU overhead: <10%
# Memory: <100MB base
```

## Cleanup

```bash
# Stop all nodes
pkill -f harmonylite-cdc

# Stop NATS
docker stop nats

# Clean data
rm -rf /data/node{1,2,3}
```

## Troubleshooting

### Replication Lag High
```bash
# Check parser lag
curl http://localhost:8080/health/status | jq '.parser.lag_lsn'

# Check NATS connection
curl http://localhost:8080/health/status | jq '.nats.connected'

# Review logs
tail -100 /var/log/harmonylite-cdc/node1.log
```

### WAL File Growing
```bash
# Check WAL size
curl http://localhost:8080/health/status | jq '.wal.size_bytes'

# Force checkpoint (if safe)
sqlite3 /data/node1/test.db "PRAGMA wal_checkpoint(TRUNCATE);"
```

### NATS Unavailable
```bash
# Check NATS logs
docker logs nats

# Verify connectivity
telnet <nats-host> 4222
```

## Next Steps

After all quickstart scenarios pass:
1. Run full E2E test suite (Ginkgo)
2. Execute chaos engineering tests
3. Perform load testing with production-like data
4. Deploy to staging environment
5. Monitor for 24 hours before production
