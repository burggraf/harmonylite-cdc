package replication

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
	"github.com/wongfei2009/harmonylite/dedup"
)

// Subscriber receives changes from NATS JetStream
type Subscriber struct {
	natsConn       *nats.Conn
	js             nats.JetStreamContext
	dedupStore     dedup.Store
	lamportClock   LamportClock
	conflictResolver *ConflictResolver
	applicator     *Applicator
	nodeID         string
	streamName     string
	consumerName   string
	shardID        uint64
}

// SubscriberConfig holds subscriber configuration
type SubscriberConfig struct {
	NATSConn          *nats.Conn
	DedupStore        dedup.Store
	LamportClock      LamportClock
	ConflictResolver  *ConflictResolver
	Applicator        *Applicator
	NodeID            string
	StreamName        string
	ConsumerName      string
	ShardID           uint64
}

// NewSubscriber creates a new replication subscriber
func NewSubscriber(config SubscriberConfig) (*Subscriber, error) {
	if config.NATSConn == nil {
		return nil, fmt.Errorf("NATS connection is required")
	}
	if config.DedupStore == nil {
		return nil, fmt.Errorf("dedup store is required")
	}
	if config.LamportClock == nil {
		return nil, fmt.Errorf("lamport clock is required")
	}
	if config.ConflictResolver == nil {
		return nil, fmt.Errorf("conflict resolver is required")
	}
	if config.Applicator == nil {
		return nil, fmt.Errorf("applicator is required")
	}

	// Create JetStream context
	js, err := config.NATSConn.JetStream()
	if err != nil {
		return nil, fmt.Errorf("failed to create JetStream context: %w", err)
	}

	return &Subscriber{
		natsConn:         config.NATSConn,
		js:               js,
		dedupStore:       config.DedupStore,
		lamportClock:     config.LamportClock,
		conflictResolver: config.ConflictResolver,
		applicator:       config.Applicator,
		nodeID:           config.NodeID,
		streamName:       config.StreamName,
		consumerName:     config.ConsumerName,
		shardID:          config.ShardID,
	}, nil
}

// Run starts the subscriber loop
func (s *Subscriber) Run(ctx context.Context) error {
	log.Info().
		Str("node_id", s.nodeID).
		Uint64("shard", s.shardID).
		Str("stream", s.streamName).
		Msg("Subscriber started")

	// Create durable pull consumer
	_, err := s.createConsumer()
	if err != nil {
		return fmt.Errorf("failed to create consumer: %w", err)
	}

	// Get pull subscription
	sub, err := s.js.PullSubscribe(fmt.Sprintf("%s-%d", s.streamName, s.shardID), s.consumerName)
	if err != nil {
		return fmt.Errorf("failed to create pull subscription: %w", err)
	}
	defer sub.Unsubscribe()

	// Process messages
	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("Subscriber stopped")
			return ctx.Err()
		default:
		}

		// Fetch next batch of messages
		msgs, err := sub.Fetch(1, nats.Context(ctx))
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		if errors.Is(err, nats.ErrTimeout) {
			continue
		}
		if err != nil {
			log.Error().Err(err).Msg("Failed to fetch messages")
			time.Sleep(100 * time.Millisecond)
			continue
		}

		// Process each message
		for _, msg := range msgs {
			if err := s.processMessage(msg); err != nil {
				log.Error().Err(err).Msg("Failed to process message")
				msg.Nak()
				continue
			}

			// Acknowledge message
			if err := msg.Ack(); err != nil {
				log.Error().Err(err).Msg("Failed to acknowledge message")
			}
		}
	}
}

// createConsumer creates a durable pull consumer
func (s *Subscriber) createConsumer() (nats.JetStreamContext, error) {
	// Consumer configuration
	cfg := &nats.ConsumerConfig{
		Durable:       s.consumerName,
		FilterSubject: fmt.Sprintf("%s-%d", s.streamName, s.shardID),
		AckPolicy:     nats.AckExplicitPolicy,
		DeliverPolicy: nats.DeliverAllPolicy,
		MaxAckPending: 100,
		AckWait:       30 * time.Second,
	}

	// Create or update consumer
	_, err := s.js.AddConsumer(s.streamName, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to add consumer: %w", err)
	}

	return s.js, nil
}

// processMessage processes a single replication message
func (s *Subscriber) processMessage(msg *nats.Msg) error {
	// Deserialize message
	var replMsg ReplicationMessage
	if err := json.Unmarshal(msg.Data, &replMsg); err != nil {
		return fmt.Errorf("failed to unmarshal message: %w", err)
	}

	// Skip messages from this node (no self-replication)
	if replMsg.NodeID == s.nodeID {
		log.Debug().
			Str("op_id", replMsg.OpID).
			Msg("Skipping message from self")
		return nil
	}

	// Check deduplication
	seen, err := s.dedupStore.Has(replMsg.OpID)
	if err != nil {
		return fmt.Errorf("failed to check dedup: %w", err)
	}
	if seen {
		log.Debug().
			Str("op_id", replMsg.OpID).
			Msg("Duplicate message, skipping")
		return nil
	}

	// Update Lamport clock (witness remote timestamp)
	s.lamportClock.Witness(replMsg.LamportClock)

	// Apply transaction atomically
	if err := s.applicator.Apply(replMsg); err != nil {
		return fmt.Errorf("failed to apply changes: %w", err)
	}

	// Mark as seen in deduplication store
	if err := s.dedupStore.Add(replMsg.OpID, replMsg.WallTime); err != nil {
		return fmt.Errorf("failed to add to dedup store: %w", err)
	}

	log.Debug().
		Str("op_id", replMsg.OpID).
		Uint64("lamport_clock", replMsg.LamportClock).
		Int("changes", len(replMsg.Transaction)).
		Msg("Transaction applied")

	return nil
}
