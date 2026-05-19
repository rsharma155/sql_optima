package hot

import (
	"github.com/google/uuid"
	"time"
)

// SQLServerDatabaseCatalogRow is one row written to sqlserver_database_catalog.
type SQLServerDatabaseCatalogRow struct {
	CaptureTimestamp               time.Time `json:"capture_timestamp"`
	ServerID                       uuid.UUID `json:"server_id"`
	DatabaseID                     int       `json:"database_id"`
	DatabaseName                   string    `json:"database_name"`
	CreateDate                     time.Time `json:"create_date"`
	CompatibilityLevel             int       `json:"compatibility_level"`
	CollationName                  string    `json:"collation_name"`
	StateDesc                      string    `json:"state_desc"`
	UserAccessDesc                 string    `json:"user_access_desc"`
	IsReadOnly                     bool      `json:"is_read_only"`
	IsCleanlyShutdown              bool      `json:"is_cleanly_shutdown"`
	RecoveryModelDesc              string    `json:"recovery_model_desc"`
	LogReuseWaitDesc               string    `json:"log_reuse_wait_desc"`
	DelayedDurabilityDesc          string    `json:"delayed_durability_desc"`
	TargetRecoveryTimeInSeconds    int       `json:"target_recovery_time_in_seconds"`
	IsAcceleratedDatabaseRecovery  bool      `json:"is_accelerated_database_recovery_on"`
	IsAutoCloseOn                  bool      `json:"is_auto_close_on"`
	IsAutoShrinkOn                 bool      `json:"is_auto_shrink_on"`
	PageVerifyOptionDesc           string    `json:"page_verify_option_desc"`
	IsReadCommittedSnapshotOn      bool      `json:"is_read_committed_snapshot_on"`
	SnapshotIsolationStateDesc     string    `json:"snapshot_isolation_state_desc"`
	IsEncrypted                    bool      `json:"is_encrypted"`
	IsCdcEnabled                   bool      `json:"is_cdc_enabled"`
	IsBrokerEnabled                bool      `json:"is_broker_enabled"`
	IsFulltextEnabled              bool      `json:"is_fulltext_enabled"`
	IsMemoryOptimizedEnabled       bool      `json:"is_memory_optimized_enabled"`
	OwnerName                      *string   `json:"owner_name"`
	ContainmentDesc                string    `json:"containment_desc"`
	IsTrustworthyOn                bool      `json:"is_trustworthy_on"`
	IsPublished                    bool      `json:"is_published"`
	IsSubscribed                   bool      `json:"is_subscribed"`
	IsDistributor                  bool      `json:"is_distributor"`
	GroupDatabaseID                *string   `json:"group_database_id"`
}

type AGHealthRow struct {
	CaptureTimestamp     time.Time  `json:"capture_timestamp"`
	ServerID             uuid.UUID  `json:"server_id"`
	AGName               string     `json:"ag_name"`
	ReplicaServerName    string     `json:"replica_server_name"`
	DatabaseName         string     `json:"database_name"`
	ReplicaRole          string     `json:"replica_role"`
	OperationalState     string     `json:"operational_state"`
	ConnectedState       string     `json:"connected_state"`
	SynchronizationState string     `json:"synchronization_state"`
	SyncStateDesc        string     `json:"synchronization_state_desc"`
	IsPrimaryReplica     bool       `json:"is_primary_replica"`
	LogSendQueueKB       int64      `json:"log_send_queue_kb"`
	RedoQueueKB          int64      `json:"redo_queue_kb"`
	LogSendRateKB        int64      `json:"log_send_rate_kb"`
	RedoRateKB           int64      `json:"redo_rate_kb"`
	LastSentTime         *time.Time `json:"last_sent_time"`
	LastReceivedTime     *time.Time `json:"last_received_time"`
	LastHardenedTime     *time.Time `json:"last_hardened_time"`
	LastRedoneTime       *time.Time `json:"last_redone_time"`
	SecondaryLagSecs     int64      `json:"secondary_lag_seconds"`
}

type DatabaseThroughputRow struct {
	CaptureTimestamp    time.Time `json:"capture_timestamp"`
	ServerID            uuid.UUID `json:"server_id"`
	DatabaseName        string    `json:"database_name"`
	UserSeeks           int64     `json:"user_seeks"`
	UserScans           int64     `json:"user_scans"`
	UserLookups         int64     `json:"user_lookups"`
	UserWrites          int64     `json:"user_writes"`
	TotalReads          int64     `json:"total_reads"`
	TotalWrites         int64     `json:"total_writes"`
	TPS                 float64   `json:"tps"`
	BatchRequestsPerSec float64   `json:"batch_requests_per_sec"`
	Reads               int64     `json:"reads"`
	Writes              int64     `json:"writes"`
	BytesRead           int64     `json:"bytes_read"`
	BytesWritten        int64     `json:"bytes_written"`
	ReadLatencyMs       int64     `json:"read_latency_ms"`
	WriteLatencyMs      int64     `json:"write_latency_ms"`
}

type PostgresReplicationSlotRow struct {
	CaptureTimestamp  time.Time `json:"capture_timestamp"`
	ServerID          uuid.UUID `json:"server_id"`
	SlotName          string    `json:"slot_name"`
	SlotType          string    `json:"slot_type"`
	Active            bool      `json:"active"`
	Temporary         bool      `json:"temporary"`
	RetainedWalMB     float64   `json:"retained_wal_mb"`
	RestartLSN        string    `json:"restart_lsn"`
	ConfirmedFlushLSN string    `json:"confirmed_flush_lsn"`
	Xmin              *int64    `json:"xmin"`
	CatalogXmin       *int64    `json:"catalog_xmin"`
}

type PostgresQueryDictionaryRow struct {
	ServerID       uuid.UUID `json:"server_id"`
	QueryID        int64     `json:"query_id"`
	QueryText      string    `json:"query_text"`
	FirstSeen      time.Time `json:"first_seen"`
	LastSeen       time.Time `json:"last_seen"`
	ExecutionCount int64     `json:"execution_count"`
}

type PostgresQueryStatsSnapRow struct {
	QueryID           int64
	QueryText         string
	DbName            string
	UserName          string
	QueryType         string
	Calls             int64
	TotalTimeMs       float64
	MeanTimeMs        float64
	Rows              int64
	TempBlksRead      int64
	TempBlksWritten   int64
	BlkReadTimeMs     float64
	BlkWriteTimeMs    float64
	SharedBlksHit     int64
	SharedBlksRead    int64
	SharedBlksDirtied int64
	SharedBlksWritten int64
	WalBytes          int64
	WalRecords        int64
	WalFpi            int64
	TotalPlanTime     float64
	MeanPlanTime      float64
	Plans             int64
}

type PostgresLockRow struct {
	CollectedAt    time.Time
	ServerID       uuid.UUID
	PID            int
	LockType       string
	Mode           string
	Granted        bool
	RelationOID    uint32
	RelationName   string
	TransactionID  string
	WaitingSeconds float64
}

type PostgresStatStatementsDeltaRow struct {
	CaptureTimestamp       time.Time
	ServerID               uuid.UUID
	QueryID                int64
	DatabaseName           string
	UserName               string
	CallsDelta             int64
	TotalTimeDeltaMs       float64
	RowsDelta              int64
	SharedBlksHitDelta     int64
	SharedBlksReadDelta    int64
	SharedBlksDirtiedDelta int64
	SharedBlksWrittenDelta int64
	TempBlksReadDelta      int64
	TempBlksWrittenDelta   int64
	BlkReadTimeDeltaMs     float64
	BlkWriteTimeDeltaMs    float64
	WalBytesDelta          int64
	TotalPlanTimeDelta     float64
}

type LongRunningQueryRow struct {
	CaptureTimestamp     time.Time `json:"capture_timestamp"`
	ServerID             uuid.UUID `json:"server_id"`
	SessionID            int       `json:"session_id"`
	RequestID            int       `json:"request_id"`
	DatabaseName         string    `json:"database_name"`
	LoginName            string    `json:"login_name"`
	HostName             string    `json:"host_name"`
	ProgramName          string    `json:"program_name"`
	QueryHash            string    `json:"query_hash"`
	QueryText            string    `json:"query_text"`
	WaitType             string    `json:"wait_type"`
	BlockingSessionID    int       `json:"blocking_session_id"`
	Status               string    `json:"status"`
	CPUTimeMs            int       `json:"cpu_time_ms"`
	TotalElapsedTimeMs   int       `json:"total_elapsed_time_ms"`
	Reads                int       `json:"reads"`
	Writes               int       `json:"writes"`
	GrantedQueryMemoryMB int       `json:"granted_query_memory_mb"`
	RowCount             int       `json:"row_count"`
}

type QueryStoreStatsRow struct {
	CaptureTimestamp  time.Time `json:"capture_timestamp"`
	ServerID          uuid.UUID `json:"server_id"`
	ServerName        string    `json:"server_name"`
	DatabaseName      string    `json:"database_name"`
	QueryHash         string    `json:"query_hash"`
	QueryText         string    `json:"query_text"`
	PlanID            int64     `json:"plan_id"`
	IntervalID        int64     `json:"interval_id"`
	Executions        int64     `json:"executions"`
	AvgDurationMs     float64   `json:"avg_duration_ms"`
	AvgCpuMs          float64   `json:"avg_cpu_ms"`
	AvgLogicalReads   float64   `json:"avg_logical_reads"`
	TotalCpuMs        float64   `json:"total_cpu_ms"`
	LastExecutionTime time.Time `json:"last_execution_time"`
}

type PostgresDiskStatRow struct {
	CaptureTimestamp time.Time `json:"capture_timestamp"`
	ServerID         uuid.UUID `json:"server_id"`
	MountName        string    `json:"mount_name"`
	Path             string    `json:"path"`
	TotalBytes       int64     `json:"total_bytes"`
	FreeBytes        int64     `json:"free_bytes"`
	AvailBytes       int64     `json:"avail_bytes"`
	UsedPct          float64   `json:"used_pct"`
}

type PostgresSessionStateCountRow struct {
	CaptureTimestamp time.Time `json:"capture_timestamp"`
	ServerID         uuid.UUID `json:"server_id"`
	ActiveCount      int       `json:"active_count"`
	IdleCount        int       `json:"idle_count"`
	IdleInTxnCount   int       `json:"idle_in_txn_count"`
	WaitingCount     int       `json:"waiting_count"`
	TotalCount       int       `json:"total_count"`
}

type PostgresBGWriterRow struct {
	CaptureTimestamp      time.Time `json:"capture_timestamp"`
	ServerID              uuid.UUID `json:"server_id"`
	CheckpointsTimed      int64     `json:"checkpoints_timed"`
	CheckpointsReq        int64     `json:"checkpoints_req"`
	CheckpointWriteTime   float64   `json:"checkpoint_write_time"`
	CheckpointSyncTime    float64   `json:"checkpoint_sync_time"`
	BuffersCheckpoint     int64     `json:"buffers_checkpoint"`
	BuffersClean          int64     `json:"buffers_clean"`
	MaxwrittenClean       int64     `json:"maxwritten_clean"`
	BuffersBackend        int64     `json:"buffers_backend"`
	BuffersBackendFsync   int64     `json:"buffers_backend_fsync"`
	BuffersAlloc          int64     `json:"buffers_alloc"`
	StatsReset            time.Time `json:"stats_reset"`
}

type PostgresArchiverRow struct {
	CaptureTimestamp time.Time `json:"capture_timestamp"`
	ServerID         uuid.UUID `json:"server_id"`
	ArchivedCount    int64     `json:"archived_count"`
	LastArchivedWal  string    `json:"last_archived_wal"`
	LastArchivedTime time.Time `json:"last_archived_time"`
	FailedCount      int64     `json:"failed_count"`
	LastFailedWal    string    `json:"last_failed_wal"`
	LastFailedTime   time.Time `json:"last_failed_time"`
	StatsReset       time.Time `json:"stats_reset"`
}

type PostgresBackupDRRow struct {
	CaptureTimestamp      time.Time
	WalBytesTotal         int64
	WalRecordsTotal       int64
	WalFPITotal           int64
	ArchivedCount         int64
	ArchiveFailedCount    int64
	LastArchivedTime      *time.Time
	LastFailedTime        *time.Time
	CheckpointsTimed      int64
	CheckpointsReq        int64
	CheckpointWriteTimeMs float64
	CheckpointSyncTimeMs  float64
	IsInRecovery          bool
}

type PostgresQueryStatsDelta struct {
	CaptureTimestamp time.Time `json:"capture_timestamp"`
	ServerID         uuid.UUID `json:"server_id"`
	QueryID          int64     `json:"query_id"`
	CallsDelta       int64     `json:"calls_delta"`
	TotalTimeDeltaMs float64   `json:"total_time_delta_ms"`
}

type PerformanceDebtFindingRow struct {
	CaptureTimestamp time.Time `json:"capture_timestamp"`
	ServerID         uuid.UUID `json:"server_id"`
	DatabaseName     string    `json:"database_name"`
	Section          string    `json:"section"`
	FindingType      string    `json:"finding_type"`
	Severity         string    `json:"severity"`
	Title            string    `json:"title"`
	ObjectName       string    `json:"object_name"`
	ObjectType       string    `json:"object_type"`
	FindingKey       string    `json:"finding_key"`
	ImpactScore      float64   `json:"impact_score"`
	Details          string    `json:"details"`
	Recommendation   string    `json:"recommendation"`
	FixScript        string    `json:"fix_script"`
}
