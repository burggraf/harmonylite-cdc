package cfg

import (
	"flag"
	"fmt"
	"hash/fnv"
	"os"
	"path"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/denisbrodbeck/machineid"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

type SnapshotStoreType string

const NodeNamePrefix = "harmonylite-node"
const EmbeddedClusterName = "e-harmonylite"
const (
	Nats   SnapshotStoreType = "nats"
	S3     SnapshotStoreType = "s3"
	WebDAV SnapshotStoreType = "webdav"
	SFTP   SnapshotStoreType = "sftp"
)

type ReplicationLogConfiguration struct {
	Shards         uint64 `toml:"shards"`
	MaxEntries     int64  `toml:"max_entries"`
	Replicas       int    `toml:"replicas"`
	Compress       bool   `toml:"compress"`
	UpdateExisting bool   `toml:"update_existing"`
}

type WebDAVConfiguration struct {
	Url string `toml:"url"`
}

type SFTPConfiguration struct {
	Url string `toml:"url"`
}

type S3Configuration struct {
	DirPath      string `toml:"path"`
	Endpoint     string `toml:"endpoint"`
	AccessKey    string `toml:"access_key"`
	SecretKey    string `toml:"secret"`
	SessionToken string `toml:"session_token"`
	Bucket       string `toml:"bucket"`
	UseSSL       bool   `toml:"use_ssl"`
}

type ObjectStoreConfiguration struct {
	Replicas   int    `toml:"replicas"`
	BucketName string `toml:"bucket"`
}

type SnapshotConfiguration struct {
	Enable    bool                     `toml:"enabled"`
	Interval  uint32                   `toml:"interval"`
	StoreType SnapshotStoreType        `toml:"store"`
	Nats      ObjectStoreConfiguration `toml:"nats"`
	S3        S3Configuration          `toml:"s3"`
	WebDAV    WebDAVConfiguration      `toml:"webdav"`
	SFTP      SFTPConfiguration        `toml:"sftp"`
}

type NATSConfiguration struct {
	URLs                 []string `toml:"urls"`
	SubjectPrefix        string   `toml:"subject_prefix"`
	StreamPrefix         string   `toml:"stream_prefix"`
	ServerConfigFile     string   `toml:"server_config"`
	SeedFile             string   `toml:"seed_file"`
	CredsUser            string   `toml:"user_name"`
	CredsPassword        string   `toml:"user_password"`
	CAFile               string   `toml:"ca_file"`
	CertFile             string   `toml:"cert_file"`
	KeyFile              string   `toml:"key_file"`
	BindAddress          string   `toml:"bind_address"`
	ConnectRetries       int      `toml:"connect_retries"`
	ReconnectWaitSeconds int      `toml:"reconnect_wait_seconds"`
}

type LoggingConfiguration struct {
	Verbose bool   `toml:"verbose"`
	Format  string `toml:"format"`
}

type PrometheusConfiguration struct {
	Bind      string `toml:"bind"`
	Enable    bool   `toml:"enable"`
	Namespace string `toml:"namespace"`
	Subsystem string `toml:"subsystem"`
}

type HealthCheckConfiguration struct {
	Enable   bool   `toml:"enable"`
	Bind     string `toml:"bind"`
	Path     string `toml:"path"`
	Detailed bool   `toml:"detailed"`
}

// WALParserConfiguration defines settings for WAL file parsing
type WALParserConfiguration struct {
	BufferSize           int    `toml:"buffer_size"`            // In-memory transaction buffer size (operations)
	DiskBufferThreshold  int    `toml:"disk_buffer_threshold"`  // When to spill to disk (operations)
	CheckpointThreshold  int64  `toml:"checkpoint_threshold"`   // WAL size in bytes that triggers checkpoint
	WatchInterval        int    `toml:"watch_interval"`         // Filesystem watch debounce interval (milliseconds)
}

// CheckpointConfiguration defines checkpoint coordination settings
type CheckpointConfiguration struct {
	DisableAutoCheckpoint bool   `toml:"disable_auto_checkpoint"` // Disable SQLite's automatic checkpoint
	StateFile             string `toml:"state_file"`              // Path to local LSN state file
	ForceCheckpointWALMB  int    `toml:"force_checkpoint_wal_mb"` // Force checkpoint when WAL exceeds this size (MB)
}

// DeduplicationConfiguration defines OpID deduplication settings
type DeduplicationConfiguration struct {
	RetentionWindow int    `toml:"retention_window"` // OpID retention window in milliseconds
	DBPath          string `toml:"db_path"`          // Path to deduplication database
}

// ConflictResolutionStrategy defines how to resolve concurrent writes
type ConflictResolutionStrategy string

const (
	StrategyLWW        ConflictResolutionStrategy = "lww"         // Last-Write-Wins
	StrategyCounter    ConflictResolutionStrategy = "counter"     // Sum increments
	StrategyAppendOnly ConflictResolutionStrategy = "append-only" // Append all writes
)

// ConflictResolutionConfiguration defines conflict resolution settings
type ConflictResolutionConfiguration struct {
	DefaultStrategy ConflictResolutionStrategy            `toml:"default_strategy"` // Default strategy for all tables
	TableStrategies map[string]ConflictResolutionStrategy `toml:"table_strategies"` // Per-table strategy overrides
}

type Configuration struct {
	SeqMapPath      string `toml:"seq_map_path"`
	DBPath          string `toml:"db_path"`
	NodeID          uint64 `toml:"node_id"`
	Publish         bool   `toml:"publish"`
	Replicate       bool   `toml:"replicate"`
	ScanMaxChanges  uint32 `toml:"scan_max_changes"`
	CleanupInterval uint32 `toml:"cleanup_interval"`
	SleepTimeout    uint32 `toml:"sleep_timeout"`
	PollingInterval uint32 `toml:"polling_interval"`

	Snapshot           SnapshotConfiguration            `toml:"snapshot"`
	ReplicationLog     ReplicationLogConfiguration      `toml:"replication_log"`
	NATS               NATSConfiguration                `toml:"nats"`
	Logging            LoggingConfiguration             `toml:"logging"`
	Prometheus         PrometheusConfiguration          `toml:"prometheus"`
	HealthCheck        *HealthCheckConfiguration        `toml:"health_check"`
	WALParser          WALParserConfiguration           `toml:"wal_parser"`
	Checkpoint         CheckpointConfiguration          `toml:"checkpoint"`
	Deduplication      DeduplicationConfiguration       `toml:"deduplication"`
	ConflictResolution ConflictResolutionConfiguration  `toml:"conflict_resolution"`
}

var ConfigPathFlag = flag.String("config", "", "Path to configuration file")
var CleanupFlag = flag.Bool("cleanup", false, "Only cleanup harmonylite triggers and changelogs")
var SaveSnapshotFlag = flag.Bool("save-snapshot", false, "Only take snapshot and upload")
var ClusterAddrFlag = flag.String("cluster-addr", "", "Cluster listening address")
var ClusterPeersFlag = flag.String("cluster-peers", "", "Comma separated list of clusters")
var LeafServerFlag = flag.String("leaf-servers", "", "Comma separated list of leaf servers")
var ProfServer = flag.String("pprof", "", "PProf listening address")
var NodeIDFlag = flag.Uint64("node-id", 0, "Override node ID from config file")

var DataRootDir = os.TempDir()
var Config = &Configuration{
	SeqMapPath:      path.Join(DataRootDir, "seq-map.cbor"),
	DBPath:          path.Join(DataRootDir, "harmonylite.db"),
	NodeID:          0,
	Publish:         true,
	Replicate:       true,
	ScanMaxChanges:  512,
	CleanupInterval: 5000,
	SleepTimeout:    0,
	PollingInterval: 0,

	Snapshot: SnapshotConfiguration{
		Enable:    true,
		Interval:  0,
		StoreType: Nats,
		Nats: ObjectStoreConfiguration{
			Replicas: 1,
		},
		S3:     S3Configuration{},
		WebDAV: WebDAVConfiguration{},
		SFTP:   SFTPConfiguration{},
	},

	ReplicationLog: ReplicationLogConfiguration{
		Shards:         1,
		MaxEntries:     1024,
		Replicas:       1,
		Compress:       true,
		UpdateExisting: false,
	},

	NATS: NATSConfiguration{
		URLs:                 []string{},
		SubjectPrefix:        "harmonylite-change-log",
		StreamPrefix:         "harmonylite-changes",
		ServerConfigFile:     "",
		SeedFile:             "",
		CredsPassword:        "",
		CredsUser:            "",
		BindAddress:          ":-1",
		ConnectRetries:       5,
		ReconnectWaitSeconds: 2,
	},

	Logging: LoggingConfiguration{
		Verbose: false,
		Format:  "console",
	},

	Prometheus: PrometheusConfiguration{
		Bind:      ":3010",
		Enable:    false,
		Namespace: "harmonylite",
		Subsystem: "",
	},
	
	HealthCheck: &HealthCheckConfiguration{
		Enable:   false,  // Disabled by default
		Bind:     "0.0.0.0:8090",
		Path:     "/health",
		Detailed: true,
	},

	WALParser: WALParserConfiguration{
		BufferSize:          1000,                      // 1000 operations in memory
		DiskBufferThreshold: 1000,                      // Spill to disk after 1000 ops
		CheckpointThreshold: 104857600,                 // 100MB WAL size
		WatchInterval:       100,                       // 100ms debounce
	},

	Checkpoint: CheckpointConfiguration{
		DisableAutoCheckpoint: true,                    // Disable SQLite auto-checkpoint
		StateFile:             path.Join(DataRootDir, "checkpoint-state.json"),
		ForceCheckpointWALMB:  100,                     // Force checkpoint at 100MB
	},

	Deduplication: DeduplicationConfiguration{
		RetentionWindow: 3600000,                       // 1 hour in milliseconds
		DBPath:          path.Join(DataRootDir, "dedup.db"),
	},

	ConflictResolution: ConflictResolutionConfiguration{
		DefaultStrategy: StrategyLWW,                   // Last-Write-Wins default
		TableStrategies: make(map[string]ConflictResolutionStrategy),
	},
}

func init() {
	if Config.NodeID == 0 {
		id, err := machineid.ID()
		if err != nil {
			log.Warn().Err(err).Msg("⚠️⚠️⚠️ Unable to read machine ID from OS, generating random ID ⚠️⚠️⚠️")
			id = uuid.NewString()
		}

		hasher := fnv.New64()
		_, err = hasher.Write([]byte(id))
		if err != nil {
			panic(err)
		}

		Config.NodeID = hasher.Sum64()
	}
}

func Load(filePath string) error {
	_, err := toml.DecodeFile(filePath, Config)
	if os.IsNotExist(err) {
		return nil
	}

	if err != nil {
		return err
	}

	if *NodeIDFlag != 0 {
		Config.NodeID = *NodeIDFlag
	}

	DataRootDir, err = filepath.Abs(path.Dir(Config.DBPath))
	if err != nil {
		return err
	}

	if Config.SeqMapPath == "" {
		Config.SeqMapPath = path.Join(DataRootDir, "seq-map.cbor")
	}

	return nil
}

func (c *Configuration) SnapshotStorageType() SnapshotStoreType {
	return c.Snapshot.StoreType
}

func (c *Configuration) NodeName() string {
	return fmt.Sprintf("%s-%d", NodeNamePrefix, c.NodeID)
}

// Validate validates the configuration
func (c *Configuration) Validate() error {
	// Validate DB path
	if c.DBPath == "" {
		return fmt.Errorf("db_path is required")
	}

	// Validate node ID
	if c.NodeID == 0 {
		return fmt.Errorf("node_id is required")
	}

	// Validate WAL parser configuration
	if c.WALParser.BufferSize <= 0 {
		return fmt.Errorf("wal_parser.buffer_size must be > 0")
	}
	if c.WALParser.DiskBufferThreshold <= 0 {
		return fmt.Errorf("wal_parser.disk_buffer_threshold must be > 0")
	}
	if c.WALParser.CheckpointThreshold <= 0 {
		return fmt.Errorf("wal_parser.checkpoint_threshold must be > 0")
	}

	// Validate checkpoint configuration
	if c.Checkpoint.ForceCheckpointWALMB <= 0 {
		return fmt.Errorf("checkpoint.force_checkpoint_wal_mb must be > 0")
	}
	if c.Checkpoint.StateFile == "" {
		return fmt.Errorf("checkpoint.state_file is required")
	}

	// Validate deduplication configuration
	if c.Deduplication.RetentionWindow <= 0 {
		return fmt.Errorf("deduplication.retention_window must be > 0")
	}
	if c.Deduplication.DBPath == "" {
		return fmt.Errorf("deduplication.db_path is required")
	}

	// Validate conflict resolution strategy
	validStrategies := map[ConflictResolutionStrategy]bool{
		StrategyLWW:        true,
		StrategyCounter:    true,
		StrategyAppendOnly: true,
	}
	if !validStrategies[c.ConflictResolution.DefaultStrategy] {
		return fmt.Errorf("conflict_resolution.default_strategy must be one of: lww, counter, append-only")
	}

	// Validate table-specific strategies
	for table, strategy := range c.ConflictResolution.TableStrategies {
		if !validStrategies[strategy] {
			return fmt.Errorf("conflict_resolution.table_strategies[%s] must be one of: lww, counter, append-only", table)
		}
	}

	// Validate replication log configuration
	if c.ReplicationLog.Shards <= 0 {
		return fmt.Errorf("replication_log.shards must be > 0")
	}

	// Validate NATS configuration
	if len(c.NATS.URLs) == 0 && c.NATS.ServerConfigFile == "" {
		return fmt.Errorf("nats.urls or nats.server_config must be specified")
	}

	return nil
}
