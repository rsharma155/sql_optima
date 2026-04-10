package config

import "time"

var (
	// Metrics collection interval - base interval for all collectors (15 seconds)
	// Matches Performance Monitor's "Aggressive" preset for high-frequency monitoring
	DefaultMetricsInterval = 15 * time.Second

	// High-frequency collectors (1 minute in Performance Monitor)
	// These use the DefaultMetricsInterval (15 sec) for more granular data
	HighFrequencyCollectors = []string{
		"wait_stats",
		"query_stats",
		"memory_stats",
		"blocking",
		"deadlocks",
		"cpu_utilization",
		"perfmon_stats",
		"file_io_stats",
		"memory_grant_stats",
		"cpu_scheduler_stats",
		"latch_stats",
		"spinlock_stats",
		"tempdb_stats",
		"session_stats",
		"waiting_tasks",
		"running_jobs",
	}

	// Medium-frequency collectors (2-5 minutes in Performance Monitor)
	// Collected less frequently to reduce overhead
	MediumFrequencyCollectors = []string{
		"query_store",
		"procedure_stats",
		"memory_clerks_stats",
		"plan_cache_stats",
		"query_snapshots",
	}

	// Low-frequency collectors (hourly or daily in Performance Monitor)
	// Server configuration, database configuration, server properties
	LowFrequencyCollectors = []string{
		"server_configuration",
		"database_configuration",
		"server_properties",
		"database_size_stats",
		"trace_management",
		"default_trace",
		"cpu_server_properties",
	}

	// Query Store collection interval (every 15 minutes)
	// This is separate from the main metrics collection
	QueryStoreCollectionInterval = 15 * time.Minute

	// Data retention periods (days)
	DefaultRetentionDays       = 30
	QueryStatsRetentionDays    = 30
	WaitStatsRetentionDays     = 30
	MemoryStatsRetentionDays   = 30
	BlockingRetentionDays      = 30
	CollectionLogRetentionDays = 7
)

const (
	// Database connection pool settings
	MaxOpenConnections = 5
	MaxIdleConnections = 2
	ConnMaxLifetime    = 10 * time.Minute

	// Query limits
	MaxTopQueries     = 10
	MaxCPUHistorySize = 256

	// API defaults
	DefaultPort         = "8080"
	DefaultQueryTimeout = 30 * time.Second

	// Extended Events file target polling
	DefaultXeFileTargetInterval = 10 * time.Second
	DefaultXeSQLitePath         = "xevents_state.sqlite"
	DefaultXeFileTargetPattern  = "GoMonitor_HighValueEvents*.xel"
)
