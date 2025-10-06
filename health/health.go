package health

import (
	"time"
)

// DBChecker defines the interface for database health checks
type DBChecker interface {
	IsConnected() bool
	AreCDCHooksInstalled() bool
	GetTrackedTablesCount() int
	DB() interface{}
}

// ReplicationChecker defines the interface for replication health checks
type ReplicationChecker interface {
	IsConnected() bool
	GetLastReplicatedEventTime() time.Time
	GetLastPublishedEventTime() time.Time
}

// WALParserChecker defines the interface for WAL parser health checks
type WALParserChecker interface {
	IsRunning() bool
	GetCurrentLSN() int64
	GetLastError() error
}

// CheckpointChecker defines the interface for checkpoint health checks
type CheckpointChecker interface {
	GetParsedLSN() int64
	GetReplicatedLSN() int64
	GetCheckpointedLSN() int64
}

// Status represents the health status of the HarmonyLite node
type Status struct {
	Status                      string    `json:"status"`
	NodeID                      uint64    `json:"node_id"`
	UptimeSeconds               int64     `json:"uptime_seconds"`
	DBConnected                 bool      `json:"db_connected"`
	NatsConnected               bool      `json:"nats_connected"`
	CDCInstalled                bool      `json:"cdc_installed"`
	TablesTracked               int       `json:"tables_tracked"`
	LastReplicatedEventTime     time.Time `json:"last_replicated_event_timestamp,omitempty"`
	LastPublishedEventTime      time.Time `json:"last_published_event_timestamp,omitempty"`
	WALParserRunning            bool      `json:"wal_parser_running"`
	CurrentLSN                  int64     `json:"current_lsn,omitempty"`
	ParsedLSN                   int64     `json:"parsed_lsn,omitempty"`
	ReplicatedLSN               int64     `json:"replicated_lsn,omitempty"`
	CheckpointedLSN             int64     `json:"checkpointed_lsn,omitempty"`
	WALParserError              string    `json:"wal_parser_error,omitempty"`
	Version                     string    `json:"version"`
}

// HealthChecker provides methods to check the health of the HarmonyLite node
type HealthChecker struct {
	streamDB   DBChecker
	replicator ReplicationChecker
	walParser  WALParserChecker
	checkpoint CheckpointChecker
	nodeID     uint64
	startTime  time.Time
	version    string
}

// NewHealthChecker creates a new HealthChecker instance
func NewHealthChecker(streamDB DBChecker, replicator ReplicationChecker, nodeID uint64, version string) *HealthChecker {
	return &HealthChecker{
		streamDB:   streamDB,
		replicator: replicator,
		nodeID:     nodeID,
		startTime:  time.Now(),
		version:    version,
	}
}

// SetWALParserChecker sets the WAL parser checker (optional)
func (c *HealthChecker) SetWALParserChecker(checker WALParserChecker) {
	c.walParser = checker
}

// SetCheckpointChecker sets the checkpoint checker (optional)
func (c *HealthChecker) SetCheckpointChecker(checker CheckpointChecker) {
	c.checkpoint = checker
}

// Check performs a health check and returns the status
func (c *HealthChecker) Check() Status {
	dbConnected := c.checkDBConnection()
	natsConnected := c.checkNatsConnection()
	cdcInstalled := c.checkCDCHooks()
	walParserRunning := c.checkWALParser()

	healthy := dbConnected && natsConnected && (cdcInstalled || walParserRunning)
	status := "healthy"
	if !healthy {
		status = "unhealthy"
	}

	healthStatus := Status{
		Status:           status,
		NodeID:           c.nodeID,
		UptimeSeconds:    int64(time.Since(c.startTime).Seconds()),
		DBConnected:      dbConnected,
		NatsConnected:    natsConnected,
		CDCInstalled:     cdcInstalled,
		TablesTracked:    c.getTablesTrackedCount(),
		WALParserRunning: walParserRunning,
		Version:          c.version,
	}

	// Add timestamps if available
	if lastReplicated := c.getLastReplicatedEventTime(); !lastReplicated.IsZero() {
		healthStatus.LastReplicatedEventTime = lastReplicated
	}

	if lastPublished := c.getLastPublishedEventTime(); !lastPublished.IsZero() {
		healthStatus.LastPublishedEventTime = lastPublished
	}

	// Add WAL parser status
	if c.walParser != nil {
		healthStatus.CurrentLSN = c.walParser.GetCurrentLSN()
		if err := c.walParser.GetLastError(); err != nil {
			healthStatus.WALParserError = err.Error()
		}
	}

	// Add checkpoint status
	if c.checkpoint != nil {
		healthStatus.ParsedLSN = c.checkpoint.GetParsedLSN()
		healthStatus.ReplicatedLSN = c.checkpoint.GetReplicatedLSN()
		healthStatus.CheckpointedLSN = c.checkpoint.GetCheckpointedLSN()
	}

	return healthStatus
}

// checkWALParser checks if the WAL parser is running
func (c *HealthChecker) checkWALParser() bool {
	if c.walParser == nil {
		return false
	}
	return c.walParser.IsRunning()
}

// checkDBConnection checks if the database is connected and responsive
func (c *HealthChecker) checkDBConnection() bool {
	if c.streamDB == nil {
		return false
	}
	
	return c.streamDB.IsConnected()
}

// checkNatsConnection checks if the NATS connection is alive
func (c *HealthChecker) checkNatsConnection() bool {
	if c.replicator == nil {
		return false
	}
	
	return c.replicator.IsConnected()
}

// checkCDCHooks checks if CDC hooks are installed
func (c *HealthChecker) checkCDCHooks() bool {
	if c.streamDB == nil {
		return false
	}
	
	return c.streamDB.AreCDCHooksInstalled()
}

// getTablesTrackedCount returns the number of tables being tracked
func (c *HealthChecker) getTablesTrackedCount() int {
	if c.streamDB == nil {
		return 0
	}
	
	return c.streamDB.GetTrackedTablesCount()
}

// getLastReplicatedEventTime returns the timestamp of the last replicated event
func (c *HealthChecker) getLastReplicatedEventTime() time.Time {
	if c.replicator == nil {
		return time.Time{}
	}
	
	return c.replicator.GetLastReplicatedEventTime()
}

// getLastPublishedEventTime returns the timestamp of the last published event
func (c *HealthChecker) getLastPublishedEventTime() time.Time {
	if c.replicator == nil {
		return time.Time{}
	}
	
	return c.replicator.GetLastPublishedEventTime()
}
