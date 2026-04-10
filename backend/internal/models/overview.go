package models

// InstanceOverview is a compact summary for /api/postgres/overview and /api/mssql/overview.
type InstanceOverview struct {
	InstanceName string `json:"instance_name"`
	Engine       string `json:"engine"` // "postgres" | "sqlserver"
	Timestamp    string `json:"timestamp"`

	// Postgres (from cache + throughput response)
	DatabaseCount   int     `json:"database_count,omitempty"`
	LastTps         float64 `json:"last_tps,omitempty"`
	LastCacheHitPct float64 `json:"last_cache_hit_pct,omitempty"`
	ActiveConns     int     `json:"active_connections,omitempty"`
	IdleConns       int     `json:"idle_connections,omitempty"`
	TotalConns      int     `json:"total_connections,omitempty"`
	ReplicationLag  float64 `json:"replication_lag_mb,omitempty"`
	ReplicationStatus string `json:"replication_status,omitempty"`

	// SQL Server (from cached dashboard)
	AvgCPULoad    float64 `json:"avg_cpu_load,omitempty"`
	MemoryUsage   float64 `json:"memory_usage,omitempty"`
	ActiveUsers   int     `json:"active_users,omitempty"`
	TotalLocks    int     `json:"total_locks,omitempty"`
	Deadlocks     int     `json:"deadlocks,omitempty"`
	TopQueryCount int     `json:"top_query_count,omitempty"`
}
