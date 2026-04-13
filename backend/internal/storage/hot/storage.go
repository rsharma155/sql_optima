package hot

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Metric struct {
	CaptureTimestamp time.Time              `json:"capture_timestamp"`
	ServerName       string                 `json:"server_name"`
	MetricName       string                 `json:"metric_name"`
	MetricValue      float64                `json:"metric_value"`
	Tags             map[string]interface{} `json:"tags,omitempty"`
}

type Config struct {
	Host     string
	Port     string
	User     string
	Password string
	Database string
	SSLMode  string
	MaxConns int32
}

func (c *Config) connString() string {
	sslMode := c.SSLMode
	if sslMode == "" {
		sslMode = "disable"
	}
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s",
		c.User, c.Password, c.Host, c.Port, c.Database, sslMode)
}

func DefaultConfig() *Config {
	// Support both the newer TIMESCALEDB_* variables and the docker-compose DB_* variables.
	host := getEnv("TIMESCALEDB_HOST", "")
	if host == "" {
		host = getEnv("DB_HOST", "localhost")
	}
	port := getEnv("TIMESCALEDB_PORT", "")
	if port == "" {
		port = getEnv("DB_PORT", "5432")
	}
	return &Config{
		Host:     host,
		Port:     port,
		User:     getEnv("DB_USER", "dbmonitor"),
		Password: getEnv("DB_PASSWORD", ""),
		Database: getEnv("DB_NAME", "dbmonitor_metrics"),
		SSLMode:  getEnv("TIMESCALEDB_SSLMODE", "disable"),
		MaxConns: 50,
	}
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

type HotStorage struct {
	pool   *pgxpool.Pool
	config *Config
	mu     sync.RWMutex
}

func New(cfg *Config) (*HotStorage, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	log.Printf("[TimescaleDB] Attempting to connect (host=%s port=%s db=%s user_set=%v sslmode=%s)...",
		cfg.Host, cfg.Port, cfg.Database, strings.TrimSpace(cfg.User) != "", cfg.SSLMode)

	poolConfig, err := pgxpool.ParseConfig(cfg.connString())
	if err != nil {
		return nil, fmt.Errorf("failed to parse connection string: %w", err)
	}

	poolConfig.MaxConns = cfg.MaxConns
	poolConfig.MinConns = 5
	poolConfig.MaxConnLifetime = 30 * time.Minute
	poolConfig.MaxConnIdleTime = 10 * time.Minute
	poolConfig.HealthCheckPeriod = 30 * time.Second

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	hs := &HotStorage{pool: pool, config: cfg}

	if err := hs.RunMigrations(ctx); err != nil {
		log.Printf("[TimescaleDB] Migration warning: %v (continuing anyway)", err)
	}

	return hs, nil
}

func (s *HotStorage) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pool != nil {
		s.pool.Close()
		s.pool = nil
	}
}

func (s *HotStorage) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

func (s *HotStorage) Pool() *pgxpool.Pool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.pool
}

func (s *HotStorage) Stats() *pgxpool.Stat {
	return s.pool.Stat()
}

func (s *HotStorage) RunMigrations(ctx context.Context) error {
	log.Println("[TimescaleDB] Running migrations...")

	migrations := []string{
		`CREATE TABLE IF NOT EXISTS sqlserver_ag_health (
			capture_timestamp TIMESTAMPTZ NOT NULL,
			server_instance_name TEXT NOT NULL,
			ag_name TEXT,
			replica_server_name TEXT,
			database_name TEXT,
			replica_role TEXT,
			synchronization_state TEXT,
			synchronization_state_desc TEXT,
			is_primary_replica BOOLEAN,
			log_send_queue_kb BIGINT DEFAULT 0,
			redo_queue_kb BIGINT DEFAULT 0,
			log_send_rate_kb BIGINT DEFAULT 0,
			redo_rate_kb BIGINT DEFAULT 0,
			last_sent_time TIMESTAMPTZ,
			last_received_time TIMESTAMPTZ,
			last_hardened_time TIMESTAMPTZ,
			last_redone_time TIMESTAMPTZ,
			secondary_lag_seconds BIGINT DEFAULT 0,
			inserted_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS sqlserver_database_throughput (
			capture_timestamp TIMESTAMPTZ NOT NULL,
			server_instance_name TEXT NOT NULL,
			database_name TEXT NOT NULL,
			user_seeks BIGINT DEFAULT 0,
			user_scans BIGINT DEFAULT 0,
			user_lookups BIGINT DEFAULT 0,
			user_writes BIGINT DEFAULT 0,
			total_reads BIGINT DEFAULT 0,
			total_writes BIGINT DEFAULT 0,
			tps DOUBLE PRECISION DEFAULT 0,
			batch_requests_per_sec DOUBLE PRECISION DEFAULT 0,
			inserted_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS sqlserver_query_store_stats (
			capture_timestamp TIMESTAMPTZ NOT NULL,
			server_instance_name TEXT NOT NULL,
			database_name TEXT,
			query_hash TEXT,
			query_text TEXT,
			plan_id BIGINT,
			is_internal_query BOOLEAN DEFAULT FALSE,
			executions BIGINT DEFAULT 0,
			avg_duration_ms DOUBLE PRECISION DEFAULT 0,
			min_duration_ms DOUBLE PRECISION DEFAULT 0,
			max_duration_ms DOUBLE PRECISION DEFAULT 0,
			stddev_duration_ms DOUBLE PRECISION DEFAULT 0,
			avg_cpu_ms DOUBLE PRECISION DEFAULT 0,
			min_cpu_ms DOUBLE PRECISION DEFAULT 0,
			max_cpu_ms DOUBLE PRECISION DEFAULT 0,
			avg_logical_reads DOUBLE PRECISION DEFAULT 0,
			avg_physical_reads DOUBLE PRECISION DEFAULT 0,
			avg_rowcount DOUBLE PRECISION DEFAULT 0,
			total_cpu_ms DOUBLE PRECISION DEFAULT 0,
			total_duration_ms DOUBLE PRECISION DEFAULT 0,
			total_logical_reads DOUBLE PRECISION DEFAULT 0,
			total_physical_reads DOUBLE PRECISION DEFAULT 0,
			runtime_stats_interval_id BIGINT,
			inserted_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS sqlserver_top_queries (
			capture_timestamp TIMESTAMPTZ NOT NULL,
			server_instance_name TEXT NOT NULL,
			login_name TEXT,
			program_name TEXT,
			database_name TEXT,
			query_text TEXT,
			cpu_time_ms BIGINT DEFAULT 0,
			exec_time_ms BIGINT DEFAULT 0,
			logical_reads BIGINT DEFAULT 0,
			execution_count BIGINT DEFAULT 0,
			query_hash TEXT,
			inserted_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS postgres_bgwriter_stats (
			capture_timestamp TIMESTAMPTZ NOT NULL,
			server_instance_name TEXT NOT NULL,
			checkpoints_timed BIGINT DEFAULT 0,
			checkpoints_req BIGINT DEFAULT 0,
			checkpoint_write_time DOUBLE PRECISION DEFAULT 0,
			checkpoint_sync_time DOUBLE PRECISION DEFAULT 0,
			buffers_checkpoint BIGINT DEFAULT 0,
			buffers_clean BIGINT DEFAULT 0,
			maxwritten_clean BIGINT DEFAULT 0,
			buffers_backend BIGINT DEFAULT 0,
			buffers_alloc BIGINT DEFAULT 0,
			inserted_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS postgres_archiver_stats (
			capture_timestamp TIMESTAMPTZ NOT NULL,
			server_instance_name TEXT NOT NULL,
			archived_count BIGINT DEFAULT 0,
			failed_count BIGINT DEFAULT 0,
			last_archived_wal TEXT,
			last_failed_wal TEXT,
			failed_count_delta BIGINT DEFAULT 0,
			inserted_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS postgres_wait_event_stats (
			capture_timestamp TIMESTAMPTZ NOT NULL,
			server_instance_name TEXT NOT NULL,
			wait_event_type TEXT,
			wait_event TEXT,
			sessions_count INTEGER DEFAULT 0,
			inserted_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS postgres_db_io_stats (
			capture_timestamp TIMESTAMPTZ NOT NULL,
			server_instance_name TEXT NOT NULL,
			database_name TEXT NOT NULL,
			blks_read BIGINT DEFAULT 0,
			blks_hit BIGINT DEFAULT 0,
			temp_files BIGINT DEFAULT 0,
			temp_bytes BIGINT DEFAULT 0,
			inserted_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS postgres_settings_snapshot (
			capture_timestamp TIMESTAMPTZ NOT NULL,
			server_instance_name TEXT NOT NULL,
			name TEXT NOT NULL,
			setting TEXT,
			unit TEXT,
			source TEXT,
			inserted_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS postgres_query_dictionary (
			server_instance_name TEXT NOT NULL,
			query_id BIGINT NOT NULL,
			query_text TEXT,
			first_seen TIMESTAMPTZ NOT NULL,
			last_seen TIMESTAMPTZ NOT NULL,
			execution_count BIGINT DEFAULT 0,
			inserted_at TIMESTAMPTZ DEFAULT NOW(),
			PRIMARY KEY (server_instance_name, query_id)
		)`,
		`CREATE TABLE IF NOT EXISTS postgres_throughput_metrics (
			capture_timestamp TIMESTAMPTZ NOT NULL,
			server_instance_name TEXT NOT NULL,
			database_name TEXT,
			tps DOUBLE PRECISION DEFAULT 0,
			cache_hit_pct DOUBLE PRECISION DEFAULT 0,
			txn_delta BIGINT DEFAULT 0,
			blks_read_delta BIGINT DEFAULT 0,
			blks_hit_delta BIGINT DEFAULT 0,
			inserted_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS postgres_connection_stats (
			capture_timestamp TIMESTAMPTZ NOT NULL,
			server_instance_name TEXT NOT NULL,
			total_connections INTEGER DEFAULT 0,
			active_connections INTEGER DEFAULT 0,
			idle_connections INTEGER DEFAULT 0,
			inserted_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS postgres_replication_stats (
			capture_timestamp TIMESTAMPTZ NOT NULL,
			server_instance_name TEXT NOT NULL,
			is_primary BOOLEAN DEFAULT false,
			cluster_state TEXT,
			max_lag_mb DOUBLE PRECISION DEFAULT 0,
			wal_gen_rate_mbps DOUBLE PRECISION DEFAULT 0,
			bgwriter_eff_pct DOUBLE PRECISION DEFAULT 0,
			inserted_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS postgres_control_center_stats (
			capture_timestamp TIMESTAMPTZ NOT NULL,
			server_instance_name TEXT NOT NULL,
			wal_rate_mb_per_min DOUBLE PRECISION DEFAULT 0,
			wal_size_mb DOUBLE PRECISION DEFAULT 0,
			max_replication_lag_mb DOUBLE PRECISION DEFAULT 0,
			max_replication_lag_seconds DOUBLE PRECISION DEFAULT 0,
			checkpoint_req_ratio DOUBLE PRECISION DEFAULT 0,
			xid_age BIGINT DEFAULT 0,
			xid_wraparound_pct DOUBLE PRECISION DEFAULT 0,
			tps DOUBLE PRECISION DEFAULT 0,
			active_sessions INTEGER DEFAULT 0,
			waiting_sessions INTEGER DEFAULT 0,
			slow_queries_count INTEGER DEFAULT 0,
			blocking_sessions INTEGER DEFAULT 0,
			autovacuum_workers INTEGER DEFAULT 0,
			dead_tuple_ratio_pct DOUBLE PRECISION DEFAULT 0,
			health_score INTEGER DEFAULT 0,
			health_status TEXT,
			inserted_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS postgres_replication_lag_detail (
			capture_timestamp TIMESTAMPTZ NOT NULL,
			server_instance_name TEXT NOT NULL,
			replica_name TEXT NOT NULL,
			lag_mb DOUBLE PRECISION DEFAULT 0,
			state TEXT,
			sync_state TEXT,
			inserted_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS postgres_replication_slot_stats (
			capture_timestamp TIMESTAMPTZ NOT NULL,
			server_instance_name TEXT NOT NULL,
			slot_name TEXT NOT NULL,
			slot_type TEXT,
			active BOOLEAN DEFAULT false,
			temporary BOOLEAN DEFAULT false,
			retained_wal_mb DOUBLE PRECISION DEFAULT 0,
			restart_lsn TEXT,
			confirmed_flush_lsn TEXT,
			xmin_txid BIGINT,
			catalog_xmin_txid BIGINT,
			inserted_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS postgres_disk_stats (
			capture_timestamp TIMESTAMPTZ NOT NULL,
			server_instance_name TEXT NOT NULL,
			mount_name TEXT NOT NULL,
			path TEXT NOT NULL,
			total_bytes BIGINT DEFAULT 0,
			free_bytes BIGINT DEFAULT 0,
			avail_bytes BIGINT DEFAULT 0,
			used_pct DOUBLE PRECISION DEFAULT 0,
			inserted_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS postgres_backup_runs (
			capture_timestamp TIMESTAMPTZ NOT NULL,
			server_instance_name TEXT NOT NULL,
			tool TEXT NOT NULL,
			backup_type TEXT NOT NULL,
			status TEXT NOT NULL,
			started_at TIMESTAMPTZ,
			finished_at TIMESTAMPTZ,
			duration_seconds BIGINT DEFAULT 0,
			wal_archived_until TIMESTAMPTZ,
			repo TEXT,
			size_bytes BIGINT DEFAULT 0,
			error_message TEXT,
			metadata JSONB DEFAULT '{}'::jsonb,
			inserted_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS postgres_log_events (
			capture_timestamp TIMESTAMPTZ NOT NULL,
			server_instance_name TEXT NOT NULL,
			severity TEXT NOT NULL,
			sqlstate TEXT,
			message TEXT NOT NULL,
			user_name TEXT,
			database_name TEXT,
			application_name TEXT,
			client_addr TEXT,
			pid BIGINT,
			context TEXT,
			detail TEXT,
			hint TEXT,
			raw JSONB DEFAULT '{}'::jsonb,
			inserted_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS postgres_vacuum_progress (
			capture_timestamp TIMESTAMPTZ NOT NULL,
			server_instance_name TEXT NOT NULL,
			pid BIGINT,
			database_name TEXT,
			user_name TEXT,
			relation_name TEXT,
			phase TEXT,
			heap_blks_total BIGINT DEFAULT 0,
			heap_blks_scanned BIGINT DEFAULT 0,
			heap_blks_vacuumed BIGINT DEFAULT 0,
			index_vacuum_count BIGINT DEFAULT 0,
			max_dead_tuples BIGINT DEFAULT 0,
			num_dead_tuples BIGINT DEFAULT 0,
			inserted_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS postgres_table_maintenance_stats (
			capture_timestamp TIMESTAMPTZ NOT NULL,
			server_instance_name TEXT NOT NULL,
			schema_name TEXT NOT NULL,
			table_name TEXT NOT NULL,
			total_bytes BIGINT DEFAULT 0,
			live_tuples BIGINT DEFAULT 0,
			dead_tuples BIGINT DEFAULT 0,
			dead_pct DOUBLE PRECISION DEFAULT 0,
			seq_scans BIGINT DEFAULT 0,
			idx_scans BIGINT DEFAULT 0,
			last_vacuum TIMESTAMPTZ,
			last_autovacuum TIMESTAMPTZ,
			last_analyze TIMESTAMPTZ,
			last_autoanalyze TIMESTAMPTZ,
			inserted_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS postgres_session_state_counts (
			capture_timestamp TIMESTAMPTZ NOT NULL,
			server_instance_name TEXT NOT NULL,
			active_count INTEGER DEFAULT 0,
			idle_count INTEGER DEFAULT 0,
			idle_in_txn_count INTEGER DEFAULT 0,
			waiting_count INTEGER DEFAULT 0,
			total_count INTEGER DEFAULT 0,
			inserted_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS postgres_pooler_stats (
			capture_timestamp TIMESTAMPTZ NOT NULL,
			server_instance_name TEXT NOT NULL,
			pooler_type TEXT DEFAULT 'pgbouncer',
			cl_active INTEGER DEFAULT 0,
			cl_waiting INTEGER DEFAULT 0,
			sv_active INTEGER DEFAULT 0,
			sv_idle INTEGER DEFAULT 0,
			sv_used INTEGER DEFAULT 0,
			maxwait_seconds DOUBLE PRECISION DEFAULT 0,
			total_pools INTEGER DEFAULT 0,
			inserted_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS postgres_deadlock_stats (
			capture_timestamp TIMESTAMPTZ NOT NULL,
			server_instance_name TEXT NOT NULL,
			database_name TEXT NOT NULL,
			deadlocks_total BIGINT DEFAULT 0,
			deadlocks_delta BIGINT DEFAULT 0,
			inserted_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`ALTER TABLE postgres_control_center_stats ADD COLUMN IF NOT EXISTS blocking_sessions INTEGER DEFAULT 0`,
		`ALTER TABLE postgres_control_center_stats ADD COLUMN IF NOT EXISTS autovacuum_workers INTEGER DEFAULT 0`,
		`ALTER TABLE postgres_control_center_stats ADD COLUMN IF NOT EXISTS dead_tuple_ratio_pct DOUBLE PRECISION DEFAULT 0`,
		`ALTER TABLE postgres_control_center_stats ADD COLUMN IF NOT EXISTS health_score INTEGER DEFAULT 0`,
		`ALTER TABLE postgres_control_center_stats ADD COLUMN IF NOT EXISTS health_status TEXT`,
		`CREATE TABLE IF NOT EXISTS postgres_system_stats (
			capture_timestamp TIMESTAMPTZ NOT NULL,
			server_instance_name TEXT NOT NULL,
			cpu_usage DOUBLE PRECISION DEFAULT 0,
			memory_usage DOUBLE PRECISION DEFAULT 0,
			active_connections INTEGER DEFAULT 0,
			idle_connections INTEGER DEFAULT 0,
			total_connections INTEGER DEFAULT 0,
			inserted_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS sqlserver_job_metrics (
			capture_timestamp TIMESTAMPTZ NOT NULL,
			server_instance_name TEXT NOT NULL,
			total_jobs INTEGER DEFAULT 0,
			enabled_jobs INTEGER DEFAULT 0,
			disabled_jobs INTEGER DEFAULT 0,
			running_jobs INTEGER DEFAULT 0,
			failed_jobs_24h INTEGER DEFAULT 0,
			error_message TEXT,
			inserted_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS sqlserver_cpu_scheduler_stats (
			capture_timestamp TIMESTAMPTZ NOT NULL,
			server_instance_name TEXT NOT NULL,
			max_workers_count INTEGER DEFAULT 0,
			scheduler_count INTEGER DEFAULT 0,
			cpu_count INTEGER DEFAULT 0,
			total_runnable_tasks_count INTEGER DEFAULT 0,
			total_work_queue_count INTEGER DEFAULT 0,
			total_current_workers_count INTEGER DEFAULT 0,
			avg_runnable_tasks_count DOUBLE PRECISION DEFAULT 0,
			total_active_request_count INTEGER DEFAULT 0,
			total_queued_request_count INTEGER DEFAULT 0,
			total_blocked_task_count INTEGER DEFAULT 0,
			total_active_parallel_thread_count INTEGER DEFAULT 0,
			runnable_request_count INTEGER DEFAULT 0,
			total_request_count INTEGER DEFAULT 0,
			runnable_percent DOUBLE PRECISION DEFAULT 0,
			worker_thread_exhaustion_warning BOOLEAN DEFAULT false,
			runnable_tasks_warning BOOLEAN DEFAULT false,
			blocked_tasks_warning BOOLEAN DEFAULT false,
			queued_requests_warning BOOLEAN DEFAULT false,
			total_physical_memory_kb BIGINT DEFAULT 0,
			available_physical_memory_kb BIGINT DEFAULT 0,
			system_memory_state_desc TEXT,
			physical_memory_pressure_warning BOOLEAN DEFAULT false,
			total_node_count INTEGER DEFAULT 0,
			nodes_online_count INTEGER DEFAULT 0,
			offline_cpu_count INTEGER DEFAULT 0,
			offline_cpu_warning BOOLEAN DEFAULT false,
			inserted_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS sqlserver_server_properties (
			capture_timestamp TIMESTAMPTZ NOT NULL,
			server_instance_name TEXT NOT NULL,
			cpu_count INTEGER DEFAULT 0,
			hyperthread_ratio INTEGER DEFAULT 0,
			socket_count INTEGER DEFAULT 0,
			cores_per_socket INTEGER DEFAULT 0,
			physical_memory_gb DOUBLE PRECISION DEFAULT 0,
			virtual_memory_gb DOUBLE PRECISION DEFAULT 0,
			cpu_type TEXT,
			hyperthread_enabled BOOLEAN DEFAULT false,
			numa_nodes INTEGER DEFAULT 0,
			max_workers_count INTEGER DEFAULT 0,
			properties_hash TEXT,
			inserted_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS sqlserver_job_details (
			capture_timestamp TIMESTAMPTZ NOT NULL,
			server_instance_name TEXT NOT NULL,
			job_name TEXT,
			job_enabled BOOLEAN DEFAULT false,
			job_owner TEXT,
			created_date TIMESTAMPTZ,
			current_status TEXT,
			last_run_date TEXT,
			last_run_time TEXT,
			last_run_status TEXT,
			inserted_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS sqlserver_job_failures (
			capture_timestamp TIMESTAMPTZ NOT NULL,
			server_instance_name TEXT NOT NULL,
			job_name TEXT,
			step_name TEXT,
			instance_id BIGINT DEFAULT 0,
			error_message TEXT,
			run_status TEXT,
			run_date TEXT,
			run_time TEXT,
			inserted_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS query_store_stats (
			capture_timestamp TIMESTAMPTZ NOT NULL,
			server_name TEXT NOT NULL,
			database_name TEXT,
			query_hash TEXT,
			query_text TEXT,
			executions BIGINT DEFAULT 0,
			avg_duration_ms DOUBLE PRECISION DEFAULT 0,
			avg_cpu_ms DOUBLE PRECISION DEFAULT 0,
			avg_logical_reads DOUBLE PRECISION DEFAULT 0,
			total_cpu_ms DOUBLE PRECISION DEFAULT 0,
			inserted_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS sqlserver_long_running_queries (
			capture_timestamp TIMESTAMPTZ NOT NULL,
			server_instance_name TEXT NOT NULL,
			session_id INTEGER DEFAULT 0,
			request_id INTEGER DEFAULT 0,
			database_name TEXT,
			login_name TEXT,
			host_name TEXT,
			program_name TEXT,
			query_hash TEXT,
			query_text TEXT,
			wait_type TEXT,
			blocking_session_id INTEGER DEFAULT 0,
			status TEXT,
			cpu_time_ms BIGINT DEFAULT 0,
			total_elapsed_time_ms BIGINT DEFAULT 0,
			reads BIGINT DEFAULT 0,
			writes BIGINT DEFAULT 0,
			granted_query_memory_mb DOUBLE PRECISION DEFAULT 0,
			row_count BIGINT DEFAULT 0,
			inserted_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`ALTER TABLE IF EXISTS sqlserver_long_running_queries ADD COLUMN IF NOT EXISTS query_hash TEXT`,
		`CREATE TABLE IF NOT EXISTS sqlserver_file_io_latency (
			capture_timestamp TIMESTAMPTZ NOT NULL,
			server_instance_name TEXT NOT NULL,
			database_name TEXT,
			file_name TEXT,
			file_type TEXT,
			read_latency_ms DOUBLE PRECISION DEFAULT 0,
			write_latency_ms DOUBLE PRECISION DEFAULT 0,
			read_iops DOUBLE PRECISION DEFAULT 0,
			write_iops DOUBLE PRECISION DEFAULT 0,
			read_bytes_per_sec DOUBLE PRECISION DEFAULT 0,
			write_bytes_per_sec DOUBLE PRECISION DEFAULT 0,
			inserted_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS sqlserver_tempdb_files (
			capture_timestamp TIMESTAMPTZ NOT NULL,
			server_instance_name TEXT NOT NULL,
			database_name TEXT,
			file_name TEXT,
			file_type TEXT,
			allocated_mb DOUBLE PRECISION DEFAULT 0,
			used_mb DOUBLE PRECISION DEFAULT 0,
			free_mb DOUBLE PRECISION DEFAULT 0,
			max_size_mb DOUBLE PRECISION DEFAULT 0,
			growth_mb DOUBLE PRECISION DEFAULT 0,
			used_percent DOUBLE PRECISION DEFAULT 0,
			inserted_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS sqlserver_risk_health (
			capture_timestamp TIMESTAMPTZ NOT NULL,
			server_instance_name TEXT NOT NULL,
			blocking_sessions INTEGER DEFAULT 0,
			memory_grants_pending INTEGER DEFAULT 0,
			failed_logins_5m INTEGER DEFAULT 0,
			tempdb_used_percent DOUBLE PRECISION DEFAULT 0,
			max_log_db_name TEXT DEFAULT '',
			max_log_used_percent DOUBLE PRECISION DEFAULT 0,
			ple DOUBLE PRECISION DEFAULT 0,
			compilations_per_sec DOUBLE PRECISION DEFAULT 0,
			batch_requests_per_sec DOUBLE PRECISION DEFAULT 0,
			buffer_cache_hit_ratio DOUBLE PRECISION DEFAULT 0,
			inserted_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS sqlserver_waits_delta (
			capture_timestamp TIMESTAMPTZ NOT NULL,
			server_instance_name TEXT NOT NULL,
			wait_type TEXT NOT NULL,
			wait_category TEXT NOT NULL,
			wait_time_ms_delta DOUBLE PRECISION DEFAULT 0,
			inserted_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS sqlserver_performance_debt_findings (
			capture_timestamp TIMESTAMPTZ NOT NULL,
			server_instance_name TEXT NOT NULL,
			database_name TEXT NOT NULL DEFAULT 'master',
			section TEXT NOT NULL,
			finding_type TEXT NOT NULL,
			severity TEXT NOT NULL,
			title TEXT NOT NULL,
			object_name TEXT DEFAULT '',
			object_type TEXT DEFAULT '',
			finding_key TEXT NOT NULL,
			details JSONB NOT NULL DEFAULT '{}'::jsonb,
			recommendation TEXT DEFAULT '',
			fix_script TEXT DEFAULT '',
			inserted_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS sqlserver_plan_cache_health (
			capture_timestamp TIMESTAMPTZ NOT NULL,
			server_instance_name TEXT NOT NULL,
			total_cache_mb DOUBLE PRECISION DEFAULT 0,
			single_use_cache_mb DOUBLE PRECISION DEFAULT 0,
			single_use_cache_pct DOUBLE PRECISION DEFAULT 0,
			adhoc_cache_mb DOUBLE PRECISION DEFAULT 0,
			prepared_cache_mb DOUBLE PRECISION DEFAULT 0,
			proc_cache_mb DOUBLE PRECISION DEFAULT 0,
			inserted_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS sqlserver_memory_grant_waiters (
			capture_timestamp TIMESTAMPTZ NOT NULL,
			server_instance_name TEXT NOT NULL,
			session_id INTEGER,
			request_id INTEGER,
			database_name TEXT,
			login_name TEXT,
			requested_memory_kb BIGINT DEFAULT 0,
			granted_memory_kb BIGINT DEFAULT 0,
			required_memory_kb BIGINT DEFAULT 0,
			wait_time_ms BIGINT DEFAULT 0,
			dop INTEGER DEFAULT 1,
			query_text TEXT,
			inserted_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS sqlserver_tempdb_top_consumers (
			capture_timestamp TIMESTAMPTZ NOT NULL,
			server_instance_name TEXT NOT NULL,
			session_id INTEGER,
			database_name TEXT,
			login_name TEXT,
			host_name TEXT,
			program_name TEXT,
			tempdb_mb DOUBLE PRECISION DEFAULT 0,
			user_objects_mb DOUBLE PRECISION DEFAULT 0,
			internal_objects_mb DOUBLE PRECISION DEFAULT 0,
			query_text TEXT,
			inserted_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS sqlserver_query_stats_staging (
			capture_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			server_instance_name TEXT NOT NULL,
			database_name TEXT,
			login_name TEXT,
			client_app TEXT,
			query_hash TEXT,
			query_text TEXT,
			total_executions BIGINT DEFAULT 0,
			total_cpu_ms BIGINT DEFAULT 0,
			total_elapsed_ms BIGINT DEFAULT 0,
			total_logical_reads BIGINT DEFAULT 0,
			total_physical_reads BIGINT DEFAULT 0,
			total_rows BIGINT DEFAULT 0,
			inserted_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS sqlserver_query_stats_snapshot (
			capture_time TIMESTAMPTZ NOT NULL,
			server_instance_name TEXT NOT NULL,
			database_name TEXT,
			login_name TEXT,
			client_app TEXT,
			query_hash TEXT,
			query_text TEXT,
			total_executions BIGINT DEFAULT 0,
			total_cpu_ms BIGINT DEFAULT 0,
			total_elapsed_ms BIGINT DEFAULT 0,
			total_logical_reads BIGINT DEFAULT 0,
			total_physical_reads BIGINT DEFAULT 0,
			total_rows BIGINT DEFAULT 0,
			row_fingerprint TEXT,
			inserted_at TIMESTAMPTZ DEFAULT NOW(),
			PRIMARY KEY (server_instance_name, query_hash, database_name, login_name, client_app, capture_time)
		)`,
		`CREATE TABLE IF NOT EXISTS sqlserver_query_stats_interval (
			bucket_start TIMESTAMPTZ NOT NULL,
			bucket_end TIMESTAMPTZ NOT NULL,
			server_instance_name TEXT NOT NULL,
			database_name TEXT,
			login_name TEXT,
			client_app TEXT,
			query_hash TEXT,
			query_text TEXT,
			executions BIGINT DEFAULT 0,
			cpu_ms BIGINT DEFAULT 0,
			duration_ms BIGINT DEFAULT 0,
			logical_reads BIGINT DEFAULT 0,
			physical_reads BIGINT DEFAULT 0,
			rows BIGINT DEFAULT 0,
			avg_cpu_ms NUMERIC DEFAULT 0,
			avg_duration_ms NUMERIC DEFAULT 0,
			avg_reads NUMERIC DEFAULT 0,
			is_reset BOOLEAN DEFAULT FALSE,
			inserted_at TIMESTAMPTZ DEFAULT NOW(),
			PRIMARY KEY (bucket_end, query_hash, database_name, login_name, client_app, server_instance_name)
		)`,
	}

	for i, migration := range migrations {
		if _, err := s.pool.Exec(ctx, migration); err != nil {
			return fmt.Errorf("migration %d failed: %w", i+1, err)
		}
	}

	if err := s.createHypertables(ctx); err != nil {
		return fmt.Errorf("failed to create hypertables: %w", err)
	}

	log.Println("[TimescaleDB] Migrations completed successfully")
	return nil
}

func (s *HotStorage) createHypertables(ctx context.Context) error {
	hypertableMigrations := []struct {
		tableName  string
		timeColumn string
	}{
		{"sqlserver_ag_health", "capture_timestamp"},
		{"sqlserver_database_throughput", "capture_timestamp"},
		{"sqlserver_query_store_stats", "capture_timestamp"},
		{"sqlserver_top_queries", "capture_timestamp"},
		{"postgres_bgwriter_stats", "capture_timestamp"},
		{"postgres_archiver_stats", "capture_timestamp"},
		{"postgres_wait_event_stats", "capture_timestamp"},
		{"postgres_db_io_stats", "capture_timestamp"},
		{"postgres_settings_snapshot", "capture_timestamp"},
		{"postgres_throughput_metrics", "capture_timestamp"},
		{"postgres_connection_stats", "capture_timestamp"},
		{"postgres_replication_stats", "capture_timestamp"},
		{"postgres_control_center_stats", "capture_timestamp"},
		{"postgres_replication_lag_detail", "capture_timestamp"},
		{"postgres_replication_slot_stats", "capture_timestamp"},
		{"postgres_disk_stats", "capture_timestamp"},
		{"postgres_backup_runs", "capture_timestamp"},
		{"postgres_log_events", "capture_timestamp"},
		{"postgres_vacuum_progress", "capture_timestamp"},
		{"postgres_table_maintenance_stats", "capture_timestamp"},
		{"postgres_session_state_counts", "capture_timestamp"},
		{"postgres_pooler_stats", "capture_timestamp"},
		{"postgres_deadlock_stats", "capture_timestamp"},
		{"postgres_system_stats", "capture_timestamp"},
		{"sqlserver_job_metrics", "capture_timestamp"},
		{"sqlserver_cpu_scheduler_stats", "capture_timestamp"},
		{"sqlserver_server_properties", "capture_timestamp"},
		{"sqlserver_job_details", "capture_timestamp"},
		{"sqlserver_job_failures", "capture_timestamp"},
		{"query_store_stats", "capture_timestamp"},
		{"sqlserver_long_running_queries", "capture_timestamp"},
		{"sqlserver_file_io_latency", "capture_timestamp"},
		{"sqlserver_tempdb_files", "capture_timestamp"},
		{"sqlserver_risk_health", "capture_timestamp"},
		{"sqlserver_waits_delta", "capture_timestamp"},
		{"sqlserver_performance_debt_findings", "capture_timestamp"},
		{"sqlserver_plan_cache_health", "capture_timestamp"},
		{"sqlserver_memory_grant_waiters", "capture_timestamp"},
		{"sqlserver_tempdb_top_consumers", "capture_timestamp"},
		{"sqlserver_query_stats_snapshot", "capture_time"},
		{"sqlserver_query_stats_interval", "bucket_end"},
	}

	for _, ht := range hypertableMigrations {
		query := fmt.Sprintf(`SELECT create_hypertable('%s', '%s', chunk_time_interval => INTERVAL '1 day', if_not_exists => TRUE)`,
			ht.tableName, ht.timeColumn)
		if _, err := s.pool.Exec(ctx, query); err != nil {
			log.Printf("[TimescaleDB] Could not create hypertable %s (may already exist or TimescaleDB not installed): %v", ht.tableName, err)
		}
	}
	return nil
}

func (s *HotStorage) GetMetricsForArchive(ctx context.Context, cutoff time.Time, limit int) ([]*Metric, string, error) {
	var metrics []*Metric
	var servers []string
	serverSet := make(map[string]bool)

	tables := []string{
		"sqlserver_ag_health",
		"sqlserver_database_throughput",
		"sqlserver_query_store_stats",
		"sqlserver_top_queries",
		"postgres_bgwriter_stats",
		"postgres_archiver_stats",
	}

	for _, table := range tables {
		query := fmt.Sprintf(`
			SELECT capture_timestamp, server_instance_name, 
				   COALESCE($1::text, 'unknown_metric') as metric_name,
				   COALESCE($2::float, 0) as metric_value,
				   '{}'::jsonb as tags
			FROM %s
			WHERE capture_timestamp < $3
			LIMIT $4`,
			table)

		rows, err := s.pool.Query(ctx, query, table, 0, cutoff, limit/len(tables))
		if err != nil {
			log.Printf("[Archiver] Warning: failed to query %s: %v", table, err)
			continue
		}

		for rows.Next() {
			var m Metric
			if err := rows.Scan(&m.CaptureTimestamp, &m.ServerName, &m.MetricName, &m.MetricValue, nil); err != nil {
				continue
			}
			metrics = append(metrics, &m)
			if !serverSet[m.ServerName] {
				serverSet[m.ServerName] = true
				servers = append(servers, m.ServerName)
			}
		}
		rows.Close()
	}

	return metrics, strings.Join(servers, ","), nil
}

func (s *HotStorage) DeleteChunksOlderThan(ctx context.Context, duration time.Duration) error {
	tables := []string{
		"sqlserver_ag_health",
		"sqlserver_database_throughput",
		"sqlserver_query_store_stats",
		"sqlserver_top_queries",
		"postgres_bgwriter_stats",
		"postgres_archiver_stats",
	}

	for _, table := range tables {
		query := fmt.Sprintf(`SELECT drop_chunks('%s', older_than => INTERVAL '%d seconds')`,
			table, int(duration.Seconds()))
		if _, err := s.pool.Exec(ctx, query); err != nil {
			log.Printf("[Archiver] Warning: failed to drop chunks for %s: %v", table, err)
		}
	}
	return nil
}
