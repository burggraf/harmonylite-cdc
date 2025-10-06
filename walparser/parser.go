package walparser

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/rs/zerolog/log"
)

// Parser reads SQLite WAL files and extracts change events
type Parser interface {
	// NextTransaction blocks until a complete transaction is available
	// Returns atomic batch of changes or error
	NextTransaction(ctx context.Context) ([]Change, error)

	// GetLSN returns current parser position in WAL
	GetLSN() int64

	// Checkpoint triggers coordinated checkpoint after replication
	Checkpoint(mode CheckpointMode) error

	// Close releases resources
	Close() error
}

// walParser implements the Parser interface
type walParser struct {
	config         ParserConfig
	dbPath         string
	walPath        string
	currentLSN     int64
	lastCheckpoint int64
	replicatedLSN  int64
	closed         bool
	mu             sync.RWMutex

	// WAL file handling
	walFile           *os.File
	walHeader         *WALHeader
	pageSize          uint32
	watcher           *fsnotify.Watcher
	checksumValidator *ChecksumValidator

	// Transaction buffering
	txBuffer *TransactionBuffer
}

// NewParser creates a new WAL parser
func NewParser(config ParserConfig, lastReplicatedLSN int64) (Parser, error) {
	// Validate config
	if config.DBPath == "" {
		return nil, fmt.Errorf("db_path is required")
	}
	if config.NodeID == "" {
		return nil, fmt.Errorf("node_id is required")
	}

	// Ensure database file exists
	if _, err := os.Stat(config.DBPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("database file not found: %s", config.DBPath)
	}

	// Construct WAL file path (database.db -> database.db-wal)
	walPath := config.DBPath + "-wal"

	// Create transaction buffer (using temp dir for disk buffer)
	txBuffer, err := NewTransactionBuffer(config.DiskBufferThreshold, os.TempDir())
	if err != nil {
		return nil, fmt.Errorf("failed to create transaction buffer: %w", err)
	}

	// Create filesystem watcher
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create watcher: %w", err)
	}

	parser := &walParser{
		config:            config,
		dbPath:            config.DBPath,
		walPath:           walPath,
		currentLSN:        lastReplicatedLSN,
		lastCheckpoint:    lastReplicatedLSN,
		replicatedLSN:     lastReplicatedLSN,
		watcher:           watcher,
		checksumValidator: &ChecksumValidator{},
		txBuffer:          txBuffer,
	}

	// Initialize WAL file reading
	if err := parser.initWALFile(); err != nil {
		watcher.Close()
		return nil, err
	}

	// Start watching WAL file for changes
	if err := watcher.Add(walPath); err != nil {
		parser.closeWALFile()
		watcher.Close()
		return nil, fmt.Errorf("failed to watch WAL file: %w", err)
	}

	log.Info().
		Str("db_path", config.DBPath).
		Str("node_id", config.NodeID).
		Int64("last_replicated_lsn", lastReplicatedLSN).
		Msg("WAL parser initialized")

	return parser, nil
}

// initWALFile opens and reads the WAL header
func (p *walParser) initWALFile() error {
	// Check if WAL exists
	if _, err := os.Stat(p.walPath); os.IsNotExist(err) {
		return ErrWALNotFound
	}

	// Open WAL file for reading
	file, err := os.Open(p.walPath)
	if err != nil {
		return fmt.Errorf("failed to open WAL file: %w", err)
	}

	// Read WAL header
	header, err := ReadWALHeader(file)
	if err != nil {
		file.Close()
		return fmt.Errorf("failed to read WAL header: %w", err)
	}

	p.walFile = file
	p.walHeader = header
	p.pageSize = header.PageSize

	// Seek past already-replicated frames
	frameSize := int64(FrameHeaderSize + p.pageSize)
	offset := WALHeaderSize + (p.replicatedLSN * frameSize)

	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		file.Close()
		return fmt.Errorf("failed to seek to replicated LSN: %w", err)
	}

	return nil
}

// closeWALFile closes the WAL file handle
func (p *walParser) closeWALFile() {
	if p.walFile != nil {
		p.walFile.Close()
		p.walFile = nil
	}
}

// NextTransaction blocks until a complete transaction is available
func (p *walParser) NextTransaction(ctx context.Context) ([]Change, error) {
	p.mu.RLock()
	if p.closed {
		p.mu.RUnlock()
		return nil, ErrParserClosed
	}
	p.mu.RUnlock()

	// Read frames until we find a commit (salt2 != 0)
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Try to read next frame
		frame, err := p.readNextFrame()
		if err == io.EOF {
			// No more frames, wait for WAL write
			if err := p.waitForWALWrite(ctx); err != nil {
				return nil, err
			}
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read frame: %w", err)
		}

		// Validate checksum
		if err := p.checksumValidator.ValidateFrame(frame, p.walHeader, frame.PageData); err != nil {
			return nil, ErrWALCorrupted
		}

		// Parse frame into change
		change, err := p.parseFrame(frame)
		if err != nil {
			// Skip frames we can't parse (e.g., interior pages)
			log.Debug().Err(err).Msg("Skipping unparseable frame")
			p.currentLSN = frame.FrameNumber
			continue
		}

		// Add to transaction buffer
		if err := p.txBuffer.Add(change); err != nil {
			return nil, fmt.Errorf("failed to buffer change: %w", err)
		}

		// Check if this is a commit frame (salt2 != 0)
		if frame.Salt2 != 0 {
			// Transaction complete
			changes, err := p.txBuffer.Commit()
			if err != nil {
				return nil, fmt.Errorf("failed to commit buffer: %w", err)
			}

			p.mu.Lock()
			p.currentLSN = frame.FrameNumber
			p.mu.Unlock()

			log.Debug().
				Int64("lsn", frame.FrameNumber).
				Int("changes", len(changes)).
				Msg("Transaction parsed")

			return changes, nil
		}

		// Update current LSN
		p.mu.Lock()
		p.currentLSN = frame.FrameNumber
		p.mu.Unlock()
	}
}

// readNextFrame reads the next WAL frame
func (p *walParser) readNextFrame() (*WALFrame, error) {
	if p.walFile == nil {
		return nil, ErrWALNotFound
	}
	return ReadFrame(p.walFile, p.pageSize, p.walHeader)
}

// waitForWALWrite waits for WAL file modifications
func (p *walParser) waitForWALWrite(ctx context.Context) error {
	watchTimeout := time.Duration(p.config.WatchInterval) * time.Millisecond
	if watchTimeout == 0 {
		watchTimeout = 100 * time.Millisecond
	}

	timer := time.NewTimer(watchTimeout)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event := <-p.watcher.Events:
			if event.Op&fsnotify.Write == fsnotify.Write {
				return nil
			}
		case err := <-p.watcher.Errors:
			return fmt.Errorf("watcher error: %w", err)
		case <-timer.C:
			return nil
		}
	}
}

// parseFrame converts a WAL frame into a Change
func (p *walParser) parseFrame(frame *WALFrame) (Change, error) {
	// Simplified B-tree parsing - full implementation would decode page structure
	change := Change{
		NodeID:        p.config.NodeID,
		LSN:           frame.FrameNumber,
		TransactionID: fmt.Sprintf("%s:%d", p.config.NodeID, frame.Salt1),
		CommitLSN:     frame.FrameNumber,
		OpID:          fmt.Sprintf("%s:%d:%d", p.config.NodeID, frame.FrameNumber, time.Now().UnixMilli()),
		Columns:       make(map[string]interface{}),
		Before:        make(map[string]interface{}),
		After:         make(map[string]interface{}),
	}

	// Check page type (first byte of page data)
	if len(frame.PageData) == 0 {
		return Change{}, fmt.Errorf("empty page data")
	}

	pageType := frame.PageData[0]
	switch pageType {
	case 0x0d: // Table B-tree leaf page
		change.Type = ChangeTypeInsert // Simplified
		change.Table = "unknown"       // Would extract from schema
	case 0x05: // Table B-tree interior page
		return Change{}, fmt.Errorf("skipping interior page")
	default:
		return Change{}, fmt.Errorf("unknown page type: 0x%02x", pageType)
	}

	return change, nil
}

// GetLSN returns current parser position in WAL
func (p *walParser) GetLSN() int64 {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.currentLSN
}

// Checkpoint triggers coordinated checkpoint after replication
func (p *walParser) Checkpoint(mode CheckpointMode) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return ErrParserClosed
	}

	// Verify replication is caught up before checkpointing (FR-020)
	if p.currentLSN > p.replicatedLSN {
		return ErrCheckpointUnsafe
	}

	// TODO: Execute SQLite checkpoint
	// PRAGMA wal_checkpoint(mode)

	p.lastCheckpoint = p.currentLSN
	return fmt.Errorf("not implemented")
}

// Close releases resources
func (p *walParser) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil
	}

	p.closed = true

	// Close watcher
	if p.watcher != nil {
		p.watcher.Close()
	}

	// Close WAL file
	p.closeWALFile()

	log.Info().Msg("WAL parser closed")

	return nil
}

// SetReplicatedLSN updates the last replicated LSN (called after replication confirmed)
func (p *walParser) SetReplicatedLSN(lsn int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.replicatedLSN = lsn
}

// walFileExists checks if the WAL file exists
func walFileExists(dbPath string) bool {
	walPath := dbPath + "-wal"
	_, err := os.Stat(walPath)
	return err == nil
}

// getWALPath returns the path to the WAL file for a given database
func getWALPath(dbPath string) string {
	return dbPath + "-wal"
}

// getSHMPath returns the path to the shared memory file for a given database
func getSHMPath(dbPath string) string {
	return dbPath + "-shm"
}

// getDBDir returns the directory containing the database file
func getDBDir(dbPath string) string {
	return filepath.Dir(dbPath)
}
