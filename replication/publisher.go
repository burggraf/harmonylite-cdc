package replication

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
	"github.com/wongfei2009/harmonylite/walparser"
)

// Publisher publishes changes to NATS JetStream
type Publisher struct {
	parser        walparser.Parser
	lamportClock  LamportClock
	natsConn      *nats.Conn
	js            nats.JetStreamContext
	nodeID        string
	streamPrefix  string
	subjectPrefix string
	shards        uint64
}

// PublisherConfig holds publisher configuration
type PublisherConfig struct {
	Parser        walparser.Parser
	LamportClock  LamportClock
	NATSConn      *nats.Conn
	NodeID        string
	StreamPrefix  string
	SubjectPrefix string
	Shards        uint64
}

// ReplicationMessage is the message format published to NATS
type ReplicationMessage struct {
	OpID         string                 `json:"op_id"`
	NodeID       string                 `json:"node_id"`
	LamportClock uint64                 `json:"lamport_clock"`
	WallTime     time.Time              `json:"wall_time"`
	Transaction  []ChangeMessage        `json:"transaction"`
}

// ChangeMessage represents a single change in the transaction
type ChangeMessage struct {
	Table      string                 `json:"table"`
	Type       string                 `json:"type"` // INSERT, UPDATE, DELETE
	PrimaryKey []interface{}          `json:"primary_key"`
	Before     map[string]interface{} `json:"before,omitempty"`
	After      map[string]interface{} `json:"after,omitempty"`
}

// NewPublisher creates a new replication publisher
func NewPublisher(config PublisherConfig) (*Publisher, error) {
	if config.Parser == nil {
		return nil, fmt.Errorf("parser is required")
	}
	if config.LamportClock == nil {
		return nil, fmt.Errorf("lamport clock is required")
	}
	if config.NATSConn == nil {
		return nil, fmt.Errorf("NATS connection is required")
	}

	// Create JetStream context
	js, err := config.NATSConn.JetStream()
	if err != nil {
		return nil, fmt.Errorf("failed to create JetStream context: %w", err)
	}

	return &Publisher{
		parser:        config.Parser,
		lamportClock:  config.LamportClock,
		natsConn:      config.NATSConn,
		js:            js,
		nodeID:        config.NodeID,
		streamPrefix:  config.StreamPrefix,
		subjectPrefix: config.SubjectPrefix,
		shards:        config.Shards,
	}, nil
}

// Run starts the publisher loop
func (p *Publisher) Run(ctx context.Context) error {
	log.Info().
		Str("node_id", p.nodeID).
		Msg("Publisher started")

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("Publisher stopped")
			return ctx.Err()
		default:
		}

		// Get next transaction from parser
		changes, err := p.parser.NextTransaction(ctx)
		if err != nil {
			if err == context.Canceled || err == context.DeadlineExceeded {
				return err
			}
			log.Error().Err(err).Msg("Failed to get next transaction")
			time.Sleep(100 * time.Millisecond)
			continue
		}

		if len(changes) == 0 {
			continue
		}

		// Publish transaction
		if err := p.publishTransaction(changes); err != nil {
			log.Error().Err(err).Msg("Failed to publish transaction")
			// TODO: Implement retry logic with backoff
			continue
		}
	}
}

// publishTransaction publishes a transaction to NATS
func (p *Publisher) publishTransaction(changes []walparser.Change) error {
	// Increment Lamport clock
	clock := p.lamportClock.Increment()

	// Build message
	msg := ReplicationMessage{
		OpID:         changes[0].OpID, // First change's OpID represents transaction
		NodeID:       p.nodeID,
		LamportClock: clock,
		WallTime:     time.Now(),
		Transaction:  make([]ChangeMessage, 0, len(changes)),
	}

	for _, change := range changes {
		changeMsg := ChangeMessage{
			Table:      change.Table,
			Type:       string(change.Type),
			PrimaryKey: change.PrimaryKey,
			Before:     change.Before,
			After:      change.After,
		}
		msg.Transaction = append(msg.Transaction, changeMsg)
	}

	// Serialize message
	payload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Determine shard (hash of OpID)
	hash := hashOpID(msg.OpID)
	shardID := (hash % p.shards) + 1
	subject := fmt.Sprintf("%s-%d", p.subjectPrefix, shardID)

	// Publish to NATS JetStream
	ack, err := p.js.Publish(subject, payload)
	if err != nil {
		return fmt.Errorf("failed to publish to NATS: %w", err)
	}

	// Log publication
	log.Debug().
		Str("op_id", msg.OpID).
		Uint64("lamport_clock", msg.LamportClock).
		Uint64("shard", shardID).
		Uint64("stream_seq", ack.Sequence).
		Int("changes", len(changes)).
		Msg("Transaction published")

	return nil
}

// hashOpID creates a hash from OpID for sharding
func hashOpID(opID string) uint64 {
	var hash uint64
	for i := 0; i < len(opID); i++ {
		hash = hash*31 + uint64(opID[i])
	}
	return hash
}
