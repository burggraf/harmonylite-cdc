package walparser

import (
	"errors"
	"time"
)

// ChangeType represents the type of database operation
type ChangeType string

const (
	ChangeTypeInsert ChangeType = "INSERT"
	ChangeTypeUpdate ChangeType = "UPDATE"
	ChangeTypeDelete ChangeType = "DELETE"
)

// Change represents a logical database operation extracted from WAL
type Change struct {
	OpID          string                 // Unique operation ID (format: node_id:lsn:timestamp)
	NodeID        string                 // Originating node identifier
	LSN           int64                  // Log Sequence Number in WAL
	Table         string                 // Table name
	PrimaryKey    []interface{}          // Primary key value(s)
	Type          ChangeType             // Operation type
	Columns       map[string]interface{} // All column values
	Before        map[string]interface{} // Old values (UPDATE/DELETE)
	After         map[string]interface{} // New values (INSERT/UPDATE)
	TransactionID string                 // Groups operations in same transaction
	CommitLSN     int64                  // LSN of transaction commit frame
}

// CheckpointMode specifies how SQLite should perform checkpoint
type CheckpointMode int

const (
	CheckpointPassive CheckpointMode = iota // SQLITE_CHECKPOINT_PASSIVE
	CheckpointFull                          // SQLITE_CHECKPOINT_FULL
	CheckpointRestart                       // SQLITE_CHECKPOINT_RESTART
	CheckpointTruncate                      // SQLITE_CHECKPOINT_TRUNCATE
)

// Common errors
var (
	ErrWALNotFound      = errors.New("WAL file not found")
	ErrWALCorrupted     = errors.New("WAL checksum validation failed")
	ErrParserClosed     = errors.New("parser is closed")
	ErrCheckpointUnsafe = errors.New("checkpoint attempted before replication confirmed")
	ErrDatabaseBusy     = errors.New("database is locked")
	ErrCheckpointFailed = errors.New("checkpoint operation failed")
)

// WALHeader represents the 32-byte WAL file header
type WALHeader struct {
	Magic          uint32 // Magic number (0x377f0683 big-endian or 0x377f0682 little-endian)
	FileFormat     uint32 // File format version
	PageSize       uint32 // Database page size
	CheckpointSeq  uint32 // Checkpoint sequence number
	Salt1          uint32 // Salt-1
	Salt2          uint32 // Salt-2
	Checksum1      uint32 // Checksum-1
	Checksum2      uint32 // Checksum-2
	BigEndian      bool   // Byte order flag
}

// WALFrame represents a single frame in the WAL file
type WALFrame struct {
	PageNumber  uint32 // Database page number
	DBSize      uint32 // Database size after commit (0 for non-commit frames)
	Salt1       uint32 // Salt-1 (must match header)
	Salt2       uint32 // Salt-2 (must match header)
	Checksum1   uint32 // Frame checksum-1
	Checksum2   uint32 // Frame checksum-2
	Payload     []byte // Page data (renamed from PageData for consistency)
	PageData    []byte // Alias for Payload
	IsCommit    bool   // True if this is a commit frame (DBSize > 0)
	LSN         int64  // Position in WAL (frame number or byte offset)
	FrameNumber int64  // Alias for LSN
}

// Transaction represents a group of changes that must be applied atomically
type Transaction struct {
	ID        string    // Unique transaction identifier
	Changes   []Change  // Ordered list of changes
	CommitLSN int64     // LSN where transaction committed
	NodeID    string    // Originating node
	Timestamp time.Time // Wall clock time of commit
}

// ParserConfig holds configuration for WAL parser
type ParserConfig struct {
	DBPath              string // Path to SQLite database
	NodeID              string // Node identifier for OpID generation
	BufferSize          int    // In-memory transaction buffer size (operations)
	DiskBufferThreshold int    // When to spill to disk (operations)
	CheckpointThreshold int64  // WAL size in bytes that triggers checkpoint
	WatchInterval       int    // Filesystem watch debounce interval (milliseconds)
}
