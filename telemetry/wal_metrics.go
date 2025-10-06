package telemetry

// WALMetrics holds Prometheus metrics for WAL parser
type WALMetrics struct {
	FramesParsed         Counter
	TransactionsPublished Counter
	ChecksumErrors       Counter
	ParseErrors          Counter
	CurrentLSN           Gauge
	ParsedLSN            Gauge
	ReplicatedLSN        Gauge
	CheckpointedLSN      Gauge
	TransactionSize      Histogram
	ParseLatency         Histogram
}

// NewWALMetrics creates a new set of WAL parser metrics
func NewWALMetrics() *WALMetrics {
	return &WALMetrics{
		FramesParsed: NewCounter(
			"wal_frames_parsed_total",
			"Total number of WAL frames parsed",
		),
		TransactionsPublished: NewCounter(
			"wal_transactions_published_total",
			"Total number of transactions published to NATS",
		),
		ChecksumErrors: NewCounter(
			"wal_checksum_errors_total",
			"Total number of WAL checksum validation errors",
		),
		ParseErrors: NewCounter(
			"wal_parse_errors_total",
			"Total number of WAL parsing errors",
		),
		CurrentLSN: NewGauge(
			"wal_current_lsn",
			"Current LSN position in WAL file",
		),
		ParsedLSN: NewGauge(
			"wal_parsed_lsn",
			"Latest LSN successfully parsed",
		),
		ReplicatedLSN: NewGauge(
			"wal_replicated_lsn",
			"Latest LSN confirmed replicated to NATS",
		),
		CheckpointedLSN: NewGauge(
			"wal_checkpointed_lsn",
			"Latest LSN checkpointed to database",
		),
		TransactionSize: NewHistogram(
			"wal_transaction_size_operations",
			"Number of operations per transaction",
		),
		ParseLatency: NewHistogram(
			"wal_parse_latency_seconds",
			"Time taken to parse a transaction",
		),
	}
}

// ReplicationMetrics holds Prometheus metrics for replication
type ReplicationMetrics struct {
	MessagesReceived      Counter
	MessagesApplied       Counter
	MessagesDuplicate     Counter
	ApplyErrors           Counter
	ConflictsResolved     Counter
	LamportClock          Gauge
	ReplicationLag        Histogram
	ApplyLatency          Histogram
}

// NewReplicationMetrics creates a new set of replication metrics
func NewReplicationMetrics() *ReplicationMetrics {
	return &ReplicationMetrics{
		MessagesReceived: NewCounter(
			"replication_messages_received_total",
			"Total number of replication messages received",
		),
		MessagesApplied: NewCounter(
			"replication_messages_applied_total",
			"Total number of replication messages successfully applied",
		),
		MessagesDuplicate: NewCounter(
			"replication_messages_duplicate_total",
			"Total number of duplicate messages skipped",
		),
		ApplyErrors: NewCounter(
			"replication_apply_errors_total",
			"Total number of errors applying replication messages",
		),
		ConflictsResolved: NewCounter(
			"replication_conflicts_resolved_total",
			"Total number of conflicts resolved",
		),
		LamportClock: NewGauge(
			"replication_lamport_clock",
			"Current Lamport clock value",
		),
		ReplicationLag: NewHistogram(
			"replication_lag_seconds",
			"Time between message publication and application",
		),
		ApplyLatency: NewHistogram(
			"replication_apply_latency_seconds",
			"Time taken to apply a replication message",
		),
	}
}

// CheckpointMetrics holds Prometheus metrics for checkpoint operations
type CheckpointMetrics struct {
	CheckpointsExecuted Counter
	CheckpointErrors    Counter
	WALSizeBytes        Gauge
	CheckpointDuration  Histogram
}

// NewCheckpointMetrics creates a new set of checkpoint metrics
func NewCheckpointMetrics() *CheckpointMetrics {
	return &CheckpointMetrics{
		CheckpointsExecuted: NewCounter(
			"checkpoint_executed_total",
			"Total number of checkpoints executed",
		),
		CheckpointErrors: NewCounter(
			"checkpoint_errors_total",
			"Total number of checkpoint errors",
		),
		WALSizeBytes: NewGauge(
			"checkpoint_wal_size_bytes",
			"Current WAL file size in bytes",
		),
		CheckpointDuration: NewHistogram(
			"checkpoint_duration_seconds",
			"Time taken to execute checkpoint",
		),
	}
}
