package coordinator

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
	"github.com/wongfei2009/harmonylite/cfg"
	"github.com/wongfei2009/harmonylite/checkpoint"
	"github.com/wongfei2009/harmonylite/db"
	"github.com/wongfei2009/harmonylite/dedup"
	"github.com/wongfei2009/harmonylite/replication"
	"github.com/wongfei2009/harmonylite/stream"
	"github.com/wongfei2009/harmonylite/telemetry"
	"github.com/wongfei2009/harmonylite/walparser"
)

// Coordinator manages all WAL-based replication components
type Coordinator struct {
	config           *cfg.Configuration
	db               *sql.DB
	natsConn         *nats.Conn
	parser           walparser.Parser
	checkpointMgr    *checkpoint.Manager
	dedupStore       dedup.Store
	lamportClock     replication.LamportClock
	conflictResolver *replication.ConflictResolver
	publisher        *replication.Publisher
	subscribers      []*replication.Subscriber
	applicator       *replication.Applicator

	// Metrics
	walMetrics         *telemetry.WALMetrics
	replicationMetrics *telemetry.ReplicationMetrics
	checkpointMetrics  *telemetry.CheckpointMetrics

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewCoordinator creates a new replication coordinator
func NewCoordinator(config *cfg.Configuration, database *sql.DB) (*Coordinator, error) {
	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	coord := &Coordinator{
		config: config,
		db:     database,
		ctx:    ctx,
		cancel: cancel,
	}

	// Initialize metrics
	coord.walMetrics = telemetry.NewWALMetrics()
	coord.replicationMetrics = telemetry.NewReplicationMetrics()
	coord.checkpointMetrics = telemetry.NewCheckpointMetrics()

	// Initialize components
	if err := coord.initializeComponents(); err != nil {
		cancel()
		return nil, err
	}

	return coord, nil
}

// initializeComponents initializes all replication components
func (c *Coordinator) initializeComponents() error {
	// Step 1: Enable WAL mode
	if err := db.EnableWALMode(c.db); err != nil {
		return fmt.Errorf("failed to enable WAL mode: %w", err)
	}

	// Step 2: Connect to NATS
	natsConn, err := stream.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to NATS: %w", err)
	}
	c.natsConn = natsConn

	// Step 3: Initialize checkpoint manager
	checkpointMgr, err := checkpoint.NewManager(c.db, checkpoint.Config{
		DBPath:               c.config.DBPath,
		StateFile:            c.config.Checkpoint.StateFile,
		NodeID:               strconv.FormatUint(c.config.NodeID, 10),
		ForceCheckpointWALMB: c.config.Checkpoint.ForceCheckpointWALMB,
		DisableAutoCheckpoint: c.config.Checkpoint.DisableAutoCheckpoint,
	})
	if err != nil {
		return fmt.Errorf("failed to initialize checkpoint manager: %w", err)
	}
	c.checkpointMgr = checkpointMgr

	// Step 4: Initialize deduplication store
	dedupStore, err := dedup.NewSQLiteStore(c.config.Deduplication.DBPath)
	if err != nil {
		return fmt.Errorf("failed to initialize dedup store: %w", err)
	}
	c.dedupStore = dedupStore

	// Step 5: Initialize Lamport clock
	c.lamportClock = replication.NewMemClock()

	// Step 6: Initialize conflict resolver
	tableStrategies := make(map[string]replication.ConflictStrategy)
	for table, strategy := range c.config.ConflictResolution.TableStrategies {
		tableStrategies[table] = replication.ConflictStrategy(strategy)
	}
	c.conflictResolver = replication.NewConflictResolver(
		replication.ConflictStrategy(c.config.ConflictResolution.DefaultStrategy),
		tableStrategies,
	)

	// Step 7: Initialize WAL parser
	_, _, lastCheckpointed := c.checkpointMgr.GetTracker().GetAll()
	parser, err := walparser.NewParser(walparser.ParserConfig{
		DBPath:              c.config.DBPath,
		NodeID:              strconv.FormatUint(c.config.NodeID, 10),
		BufferSize:          c.config.WALParser.BufferSize,
		DiskBufferThreshold: c.config.WALParser.DiskBufferThreshold,
		CheckpointThreshold: c.config.WALParser.CheckpointThreshold,
		WatchInterval:       c.config.WALParser.WatchInterval,
	}, lastCheckpointed)
	if err != nil {
		return fmt.Errorf("failed to initialize WAL parser: %w", err)
	}
	c.parser = parser

	// Step 8: Initialize applicator
	applicator, err := replication.NewApplicator(replication.ApplicatorConfig{
		DB:               c.db,
		ConflictResolver: c.conflictResolver,
	})
	if err != nil {
		return fmt.Errorf("failed to initialize applicator: %w", err)
	}
	c.applicator = applicator

	// Step 9: Initialize publisher (if publishing enabled)
	if c.config.Publish {
		publisher, err := replication.NewPublisher(replication.PublisherConfig{
			Parser:        c.parser,
			LamportClock:  c.lamportClock,
			NATSConn:      c.natsConn,
			NodeID:        strconv.FormatUint(c.config.NodeID, 10),
			StreamPrefix:  c.config.NATS.StreamPrefix,
			SubjectPrefix: c.config.NATS.SubjectPrefix,
			Shards:        c.config.ReplicationLog.Shards,
		})
		if err != nil {
			return fmt.Errorf("failed to initialize publisher: %w", err)
		}
		c.publisher = publisher
	}

	// Step 10: Initialize subscribers (if replication enabled)
	if c.config.Replicate {
		c.subscribers = make([]*replication.Subscriber, 0, c.config.ReplicationLog.Shards)
		for i := uint64(0); i < c.config.ReplicationLog.Shards; i++ {
			shardID := i + 1
			streamName := fmt.Sprintf("%s-%d", c.config.NATS.StreamPrefix, shardID)
			consumerName := fmt.Sprintf("harmonylite-consumer-%d", c.config.NodeID)

			subscriber, err := replication.NewSubscriber(replication.SubscriberConfig{
				NATSConn:         c.natsConn,
				DedupStore:       c.dedupStore,
				LamportClock:     c.lamportClock,
				ConflictResolver: c.conflictResolver,
				Applicator:       c.applicator,
				NodeID:           strconv.FormatUint(c.config.NodeID, 10),
				StreamName:       streamName,
				ConsumerName:     consumerName,
				ShardID:          shardID,
			})
			if err != nil {
				return fmt.Errorf("failed to initialize subscriber for shard %d: %w", shardID, err)
			}
			c.subscribers = append(c.subscribers, subscriber)
		}
	}

	log.Info().
		Str("db_path", c.config.DBPath).
		Uint64("node_id", c.config.NodeID).
		Bool("publish", c.config.Publish).
		Bool("replicate", c.config.Replicate).
		Msg("Coordinator initialized")

	return nil
}

// Start starts all replication components
func (c *Coordinator) Start() error {
	// Start publisher
	if c.publisher != nil {
		c.wg.Add(1)
		go func() {
			defer c.wg.Done()
			if err := c.publisher.Run(c.ctx); err != nil && err != context.Canceled {
				log.Error().Err(err).Msg("Publisher error")
			}
		}()
		log.Info().Msg("Publisher started")
	}

	// Start subscribers
	for i, subscriber := range c.subscribers {
		c.wg.Add(1)
		shardID := i + 1
		go func(sub *replication.Subscriber, shard int) {
			defer c.wg.Done()
			if err := sub.Run(c.ctx); err != nil && err != context.Canceled {
				log.Error().Err(err).Int("shard", shard).Msg("Subscriber error")
			}
		}(subscriber, shardID)
		log.Info().Int("shard", shardID).Msg("Subscriber started")
	}

	// Start checkpoint monitoring
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.checkpointMonitor()
	}()

	// Start deduplication cleanup
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.dedupCleanup()
	}()

	log.Info().Msg("All replication components started")
	return nil
}

// Stop stops all replication components
func (c *Coordinator) Stop() error {
	log.Info().Msg("Stopping coordinator...")

	// Cancel context to stop all goroutines
	c.cancel()

	// Wait for all goroutines to finish
	c.wg.Wait()

	// Close parser
	if c.parser != nil {
		if err := c.parser.Close(); err != nil {
			log.Warn().Err(err).Msg("Error closing parser")
		}
	}

	// Close dedup store
	if c.dedupStore != nil {
		if err := c.dedupStore.Close(); err != nil {
			log.Warn().Err(err).Msg("Error closing dedup store")
		}
	}

	// Close NATS connection
	if c.natsConn != nil {
		c.natsConn.Close()
	}

	log.Info().Msg("Coordinator stopped")
	return nil
}

// checkpointMonitor periodically checks if checkpoint is needed
func (c *Coordinator) checkpointMonitor() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			should, reason := c.checkpointMgr.ShouldCheckpoint()
			if should {
				log.Info().Str("reason", reason).Msg("Executing checkpoint")
				if err := c.checkpointMgr.ExecuteCheckpoint(c.ctx, int(walparser.CheckpointPassive)); err != nil {
					log.Error().Err(err).Msg("Checkpoint failed")
					c.checkpointMetrics.CheckpointErrors.Inc()
				} else {
					c.checkpointMetrics.CheckpointsExecuted.Inc()
				}
			}

			// Update metrics
			parsed, replicated, checkpointed := c.checkpointMgr.GetTracker().GetAll()
			c.walMetrics.ParsedLSN.Set(float64(parsed))
			c.walMetrics.ReplicatedLSN.Set(float64(replicated))
			c.walMetrics.CheckpointedLSN.Set(float64(checkpointed))
		}
	}
}

// dedupCleanup periodically cleans up old OpIDs from dedup store
func (c *Coordinator) dedupCleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			retentionWindow := time.Duration(c.config.Deduplication.RetentionWindow) * time.Millisecond
			count, err := c.dedupStore.Cleanup(retentionWindow)
			if err != nil {
				log.Error().Err(err).Msg("Dedup cleanup failed")
			} else {
				log.Debug().Int("count", count).Msg("Dedup cleanup complete")
			}
		}
	}
}

// GetCheckpointManager returns the checkpoint manager for health checks
func (c *Coordinator) GetCheckpointManager() *checkpoint.Manager {
	return c.checkpointMgr
}
