package hot

import (
	"context"
	"database/sql"
	"log"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type TimescaleLogger struct {
	pool                *pgxpool.Pool
	mu                  sync.RWMutex
	prevDiskHistory     map[string]map[string]interface{}
	prevJobDetails      map[string]map[string]interface{}
	prevJobFailures     map[string]string
	prevLongRunningHash map[string]int64
	prevMemoryPLE       float64
	prevWaitHistory     map[string]map[string]float64
	prevQueryStoreStats       map[string]int64
	prevSchedulerStats        map[string]uint64
	prevTopQueries            map[string]map[string]int64
	prevEnterpriseBatchHash   map[string]uint64
	// Postgres Control Center dedup/delta state
	prevPgWalBytesTotal       map[string]uint64
	prevPgControlCenterHash   map[string]uint64
	prevPgSystemStatsHash     map[string]uint64
	prevPgConnectionStatsHash map[string]uint64
	prevPgReplicationSlotsHash map[string]uint64
	prevPgDeadlocksTotal      map[string]map[string]int64 // instance -> db -> last total
	prevPgWaitEventsHash      map[string]uint64
	prevPgDbIOHash            map[string]uint64
	prevPgSettingsHash        map[string]uint64
}

func NewTimescaleLogger(pool *pgxpool.Pool) *TimescaleLogger {
	return &TimescaleLogger{
		pool:                pool,
		prevDiskHistory:     make(map[string]map[string]interface{}),
		prevJobDetails:      make(map[string]map[string]interface{}),
		prevJobFailures:     make(map[string]string),
		prevLongRunningHash: make(map[string]int64),
		prevMemoryPLE:       -1,
		prevWaitHistory:     make(map[string]map[string]float64),
		prevQueryStoreStats:       make(map[string]int64),
		prevSchedulerStats:        make(map[string]uint64),
		prevTopQueries:            make(map[string]map[string]int64),
		prevEnterpriseBatchHash:   make(map[string]uint64),
		prevPgWalBytesTotal:       make(map[string]uint64),
		prevPgControlCenterHash:   make(map[string]uint64),
		prevPgSystemStatsHash:     make(map[string]uint64),
		prevPgConnectionStatsHash: make(map[string]uint64),
		prevPgReplicationSlotsHash: make(map[string]uint64),
		prevPgDeadlocksTotal:      make(map[string]map[string]int64),
		prevPgWaitEventsHash:      make(map[string]uint64),
		prevPgDbIOHash:            make(map[string]uint64),
		prevPgSettingsHash:        make(map[string]uint64),
	}
}

func (tl *TimescaleLogger) Ping(ctx context.Context) error {
	return tl.pool.Ping(ctx)
}

func (tl *TimescaleLogger) Close() error {
	tl.pool.Close()
	return nil
}

type SQLServerMetricRow struct {
	CaptureTimestamp time.Time `json:"capture_timestamp"`
	ServerName       string    `json:"server_name"`
	AvgCpuLoad       float64   `json:"avg_cpu_load"`
	MemoryUsage      float64   `json:"memory_usage"`
	ActiveUsers      int       `json:"active_users"`
	TotalLocks       int       `json:"total_locks"`
	Deadlocks        int       `json:"deadlocks"`
	DataDiskMB       float64   `json:"data_disk_mb"`
	LogDiskMB        float64   `json:"log_disk_mb"`
	FreeDiskMB       float64   `json:"free_disk_mb"`
	CpuWait          float64   `json:"cpu_wait"`
	DiskWait         float64   `json:"disk_wait"`
	LockWait         float64   `json:"lock_wait"`
	NetworkWait      float64   `json:"network_wait"`
}

type PostgresThroughputRow struct {
	CaptureTimestamp time.Time `json:"capture_timestamp"`
	ServerName       string    `json:"server_name"`
	DatabaseName     string    `json:"database_name"`
	Tps              float64   `json:"tps"`
	CacheHitPct      float64   `json:"cache_hit_pct"`
	TxnDelta         int64     `json:"txn_delta"`
	BlksReadDelta    int64     `json:"blks_read_delta"`
	BlksHitDelta     int64     `json:"blks_hit_delta"`
}

type PostgresConnectionRow struct {
	CaptureTimestamp  time.Time `json:"capture_timestamp"`
	ServerName        string    `json:"server_name"`
	TotalConnections  int       `json:"total_connections"`
	ActiveConnections int       `json:"active_connections"`
	IdleConnections   int       `json:"idle_connections"`
}

type PostgresSystemStatsRow struct {
	CaptureTimestamp  time.Time `json:"capture_timestamp"`
	ServerName        string    `json:"server_name"`
	CPUUsage          float64   `json:"cpu_usage"`
	MemoryUsage       float64   `json:"memory_usage"`
	ActiveConnections int       `json:"active_connections"`
	IdleConnections   int       `json:"idle_connections"`
	TotalConnections  int       `json:"total_connections"`
}

type PostgresReplicationSlotRow struct {
	CaptureTimestamp    time.Time `json:"capture_timestamp"`
	ServerInstanceName  string    `json:"server_instance_name"`
	SlotName            string    `json:"slot_name"`
	SlotType            string    `json:"slot_type"`
	Active              bool      `json:"active"`
	Temporary           bool      `json:"temporary"`
	RetainedWalMB       float64   `json:"retained_wal_mb"`
	RestartLSN          string    `json:"restart_lsn"`
	ConfirmedFlushLSN   string    `json:"confirmed_flush_lsn"`
	Xmin                *int64    `json:"xmin,omitempty"`
	CatalogXmin         *int64    `json:"catalog_xmin,omitempty"`
}

type QueryStoreStatsRow struct {
	CaptureTimestamp time.Time `json:"capture_timestamp"`
	ServerName       string    `json:"server_name"`
	DatabaseName     string    `json:"database_name"`
	QueryHash        string    `json:"query_hash"`
	QueryText        string    `json:"query_text"`
	Executions       int64     `json:"executions"`
	AvgDurationMs    float64   `json:"avg_duration_ms"`
	AvgCpuMs         float64   `json:"avg_cpu_ms"`
	AvgLogicalReads  float64   `json:"avg_logical_reads"`
	TotalCpuMs       float64   `json:"total_cpu_ms"`
}

type LongRunningQueryRow struct {
	CaptureTimestamp     time.Time `json:"capture_timestamp"`
	ServerInstanceName   string    `json:"server_instance_name"`
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
	CPUTimeMs            int64     `json:"cpu_time_ms"`
	TotalElapsedTimeMs   int64     `json:"total_elapsed_time_ms"`
	Reads                int64     `json:"reads"`
	Writes               int64     `json:"writes"`
	GrantedQueryMemoryMB int       `json:"granted_query_memory_mb"`
	RowCount             int64     `json:"row_count"`
}

type AGHealthRow struct {
	CaptureTimestamp     time.Time    `json:"capture_timestamp"`
	ServerInstanceName   string       `json:"server_instance_name"`
	AGName               string       `json:"ag_name"`
	ReplicaServerName    string       `json:"replica_server_name"`
	DatabaseName         string       `json:"database_name"`
	ReplicaRole          string       `json:"replica_role"`
	SyncState            string       `json:"sync_state"`
	SynchronizationState string       `json:"synchronization_state"`
	SyncStateDesc        string       `json:"sync_state_desc"`
	IsPrimaryReplica     bool         `json:"is_primary_replica"`
	LogSendQueueKB       int64        `json:"log_send_queue_kb"`
	RedoQueueKB          int64        `json:"redo_queue_kb"`
	LogSendRateKB        int64        `json:"log_send_rate_kb"`
	RedoRateKB           int64        `json:"redo_rate_kb"`
	LastSentTime         sql.NullTime `json:"last_sent_time"`
	LastReceivedTime     sql.NullTime `json:"last_received_time"`
	LastHardenedTime     sql.NullTime `json:"last_hardened_time"`
	LastRedoneTime       sql.NullTime `json:"last_redone_time"`
	SecondaryLagSecs     int64        `json:"secondary_lag_secs"`
}

type DatabaseThroughputRow struct {
	CaptureTimestamp    time.Time `json:"capture_timestamp"`
	ServerInstanceName  string    `json:"server_instance_name"`
	DatabaseName        string    `json:"database_name"`
	UserSeeks           int64     `json:"user_seeks"`
	UserScans           int64     `json:"user_scans"`
	UserLookups         int64     `json:"user_lookups"`
	UserWrites          int64     `json:"user_writes"`
	TotalReads          int64     `json:"total_reads"`
	TotalWrites         int64     `json:"total_writes"`
	TPS                 float64   `json:"tps"`
	BatchRequestsPerSec float64   `json:"batch_requests_per_sec"`
}

type PostgresBGWriterRow struct {
	CaptureTimestamp    time.Time `json:"capture_timestamp"`
	ServerInstanceName  string    `json:"server_instance_name"`
	CheckpointsTimed    int64     `json:"checkpoints_timed"`
	CheckpointsReq      int64     `json:"checkpoints_req"`
	CheckpointWriteTime float64   `json:"checkpoint_write_time"`
	CheckpointSyncTime  float64   `json:"checkpoint_sync_time"`
	BuffersCheckpoint   int64     `json:"buffers_checkpoint"`
	BuffersClean        int64     `json:"buffers_clean"`
	MaxwrittenClean     int64     `json:"maxwritten_clean"`
	BuffersBackend      int64     `json:"buffers_backend"`
	BuffersAlloc        int64     `json:"buffers_alloc"`
}

type PostgresArchiverRow struct {
	CaptureTimestamp   time.Time      `json:"capture_timestamp"`
	ServerInstanceName string         `json:"server_instance_name"`
	ArchivedCount      int64          `json:"archived_count"`
	FailedCount        int64          `json:"failed_count"`
	LastArchivedWal    sql.NullString `json:"last_archived_wal"`
	LastFailedWal      sql.NullString `json:"last_failed_wal"`
	FailedCountDelta   int64          `json:"failed_count_delta"`
}

type PostgresQueryDictionaryRow struct {
	CaptureTimestamp   time.Time `json:"capture_timestamp"`
	ServerInstanceName string    `json:"server_instance_name"`
	QueryID            int64     `json:"query_id"`
	QueryText          string    `json:"query_text"`
	Encoding           string    `json:"encoding"`
	FirstSeen          time.Time `json:"first_seen"`
	LastSeen           time.Time `json:"last_seen"`
	ExecutionCount     int64     `json:"execution_count"`
}

// PostgresQueryStatsSnapRow is one row in postgres_query_stats (snapshot of pg_stat_statements counters).
type PostgresQueryStatsSnapRow struct {
	QueryID         int64
	QueryText       string
	Calls           int64
	TotalTimeMs     float64
	MeanTimeMs      float64
	Rows            int64
	TempBlksRead    int64
	TempBlksWritten int64
	BlkReadTimeMs   float64
	BlkWriteTimeMs  float64
}

// PostgresQueryStatsDelta is per-query activity derived from two snapshots (end minus baseline).
type PostgresQueryStatsDelta struct {
	QueryID         int64
	QueryText       string
	Calls           int64
	TotalTimeMs     float64
	MeanTimeMs      float64
	Rows            int64
	TempBlksRead    int64
	TempBlksWritten int64
	BlkReadTimeMs   float64
	BlkWriteTimeMs  float64
}

type CPUSchedulerStatsRow struct {
	CaptureTimestamp         time.Time `json:"capture_timestamp"`
	ServerInstanceName       string    `json:"server_instance_name"`
	MaxWorkersCount          int       `json:"max_workers_count"`
	SchedulerCount           int       `json:"scheduler_count"`
	CPUCount                 int       `json:"cpu_count"`
	TotalRunnableTasksCount  int       `json:"total_runnable_tasks_count"`
	TotalWorkQueueCount      int       `json:"total_work_queue_count"`
	TotalCurrentWorkersCount int       `json:"total_current_workers_count"`
	AvgRunnableTasksCount    float64   `json:"avg_runnable_tasks_count"`
	TotalActiveRequestCount  int       `json:"total_active_request_count"`
	TotalQueuedRequestCount  int       `json:"total_queued_request_count"`
	TotalBlockedTaskCount    int       `json:"total_blocked_task_count"`
	RunnablePercent          float64   `json:"runnable_percent"`
	WorkerThreadExhaustionWarning bool `json:"worker_thread_exhaustion_warning"`
	RunnableTasksWarning     bool      `json:"runnable_tasks_warning"`
	BlockedTasksWarning      bool      `json:"blocked_tasks_warning"`
	QueuedRequestsWarning    bool      `json:"queued_requests_warning"`
	TotalPhysicalMemoryKB    int       `json:"total_physical_memory_kb"`
	AvailablePhysicalMemoryKB int      `json:"available_physical_memory_kb"`
	SystemMemoryStateDesc    string    `json:"system_memory_state_desc"`
	PhysicalMemoryPressureWarning bool `json:"physical_memory_pressure_warning"`
	TotalNodeCount           int       `json:"total_node_count"`
	NodesOnlineCount         int       `json:"nodes_online_count"`
	OfflineCPUCount          int       `json:"offline_cpu_count"`
	OfflineCPUWarning        bool      `json:"offline_cpu_warning"`
}

type ServerPropertiesRow struct {
	CaptureTimestamp   time.Time `json:"capture_timestamp"`
	ServerInstanceName string    `json:"server_instance_name"`
	CPUCount           int       `json:"cpu_count"`
	HyperthreadRatio   float64   `json:"hyperthread_ratio"`
	SocketCount        int       `json:"socket_count"`
	CoresPerSocket     int       `json:"cores_per_socket"`
	PhysicalMemoryGB   float64   `json:"physical_memory_gb"`
	VirtualMemoryGB    float64   `json:"virtual_memory_gb"`
	CPUType            string    `json:"cpu_type"`
	HyperthreadEnabled bool      `json:"hyperthread_enabled"`
	NUMANodes          int       `json:"numa_nodes"`
	MaxWorkersCount    int       `json:"max_workers_count"`
	PropertiesHash     string    `json:"properties_hash"`
}

func (tl *TimescaleLogger) GetLatestMetrics(ctx context.Context, instanceName string, dbType string) (map[string]interface{}, error) {
	if dbType == "postgres" {
		query := `SELECT capture_timestamp, cpu_usage, memory_usage, active_connections, total_connections
			FROM postgres_system_stats
			WHERE server_instance_name = $1
			ORDER BY capture_timestamp DESC LIMIT 1`
		var ts time.Time
		var cpu, mem float64
		var active, total int
		err := tl.pool.QueryRow(ctx, query, instanceName).Scan(&ts, &cpu, &mem, &active, &total)
		if err != nil && err != pgx.ErrNoRows {
			return nil, err
		}
		return map[string]interface{}{
			"cpu_usage":          cpu,
			"memory_usage":       mem,
			"active_connections": active,
			"total_connections":  total,
			"capture_timestamp":  ts,
		}, nil
	}

	query := `SELECT capture_timestamp, avg_cpu_load, memory_usage, active_users, total_locks, deadlocks
		FROM sqlserver_metrics
		WHERE server_instance_name = $1
		ORDER BY capture_timestamp DESC LIMIT 1`
	var ts time.Time
	var cpu, mem float64
	var users, locks, deadlocks int
	err := tl.pool.QueryRow(ctx, query, instanceName).Scan(&ts, &cpu, &mem, &users, &locks, &deadlocks)
	if err != nil && err != pgx.ErrNoRows {
		return nil, err
	}
	return map[string]interface{}{
		"avg_cpu_load": cpu,
		"memory_usage": mem,
		"active_users": users,
		"total_locks":  locks,
		"deadlocks":    deadlocks,
		"timestamp":    ts,
	}, nil
}

func (tl *TimescaleLogger) LogAllMetrics(ctx context.Context, instanceName string, metrics interface{}) error {
	if err := tl.LogSystemMetrics(ctx, instanceName, metrics); err != nil {
		log.Printf("[TSLogger] Failed to log system metrics: %v", err)
	}
	return nil
}

func (tl *TimescaleLogger) LogSystemMetrics(ctx context.Context, instanceName string, metrics interface{}) error {
	return nil
}

func (tl *TimescaleLogger) ensureTimescaleHypertable(ctx context.Context, tableName string) error {
	var exists bool
	err := tl.pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM timescaledb_information.hypertables WHERE table_name = $1)", tableName).Scan(&exists)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return err
	}
	if !exists {
		log.Printf("[TSLogger] Warning: %s is not a hypertable", tableName)
	}
	return nil
}

func parseTimeRange(from, to string) (time.Time, time.Time, error) {
	now := time.Now()
	var start, end time.Time
	var err error

	if from != "" {
		start, err = time.Parse(time.RFC3339, from)
		if err != nil {
			start = now.Add(-1 * time.Hour)
		}
	} else {
		start = now.Add(-1 * time.Hour)
	}

	if to != "" {
		end, err = time.Parse(time.RFC3339, to)
		if err != nil {
			end = now
		}
	} else {
		end = now
	}

	return start, end, nil
}
