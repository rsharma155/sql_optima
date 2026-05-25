// SQL Optima — https://github.com/rsharma155/sql_optima
//
// File: backend/internal/storage/cold/schemas/sqlserver_extended.go
// Purpose: Typed Parquet schemas for extended SQL Server metrics (Risk Health, Memory Metrics).
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package schemas

// SQLServerDiskRow is the Parquet schema for sqlserver_disk_history.
type SQLServerDiskRow struct {
	CaptureTimestampMs int64   `parquet:"name=capture_timestamp_ms, type=INT64"`
	ServerID           string  `parquet:"name=server_id,            type=BYTE_ARRAY, converted=STRING"`
	DatabaseName       string  `parquet:"name=database_name,       type=BYTE_ARRAY, converted=STRING"`
	DataMB             float64 `parquet:"name=data_mb,             type=DOUBLE"`
	LogMB              float64 `parquet:"name=log_mb,              type=DOUBLE"`
	FreeMB             float64 `parquet:"name=free_mb,             type=DOUBLE"`
	DeltaDataMB        float64 `parquet:"name=delta_data_mb,       type=DOUBLE"`
	DeltaLogMB         float64 `parquet:"name=delta_log_mb,        type=DOUBLE"`
}

// SQLServerThroughputRow is the Parquet schema for sqlserver_database_throughput.
type SQLServerThroughputRow struct {
	CaptureTimestampMs int64   `parquet:"name=capture_timestamp_ms, type=INT64"`
	ServerID           string  `parquet:"name=server_id,            type=BYTE_ARRAY, converted=STRING"`
	DatabaseName       string  `parquet:"name=database_name,       type=BYTE_ARRAY, converted=STRING"`
	UserSeeks          int64   `parquet:"name=user_seeks,           type=INT64"`
	UserScans          int64   `parquet:"name=user_scans,           type=INT64"`
	UserLookups        int64   `parquet:"name=user_lookups,         type=INT64"`
	UserWrites         int64   `parquet:"name=user_writes,          type=INT64"`
	TotalReads         int64   `parquet:"name=total_reads,          type=INT64"`
	TotalWrites        int64   `parquet:"name=total_writes,         type=INT64"`
	TPS                float64 `parquet:"name=tps,                  type=DOUBLE"`
	BatchRequests      float64 `parquet:"name=batch_requests,       type=DOUBLE"`
	Reads              int64   `parquet:"name=reads,                type=INT64"`
	Writes             int64   `parquet:"name=writes,               type=INT64"`
	BytesRead          int64   `parquet:"name=bytes_read,           type=INT64"`
	BytesWritten       int64   `parquet:"name=bytes_written,        type=INT64"`
	ReadLatencyMs      int64   `parquet:"name=read_latency_ms,      type=INT64"`
	WriteLatencyMs     int64   `parquet:"name=write_latency_ms,     type=INT64"`
}

// SQLServerAGHealthRow is the Parquet schema for sqlserver_ag_health.
type SQLServerAGHealthRow struct {
	CaptureTimestampMs      int64  `parquet:"name=capture_timestamp_ms,      type=INT64"`
	ServerID                string `parquet:"name=server_id,                 type=BYTE_ARRAY, converted=STRING"`
	AGName                  string `parquet:"name=ag_name,                   type=BYTE_ARRAY, converted=STRING"`
	ReplicaServerName       string `parquet:"name=replica_server_name,       type=BYTE_ARRAY, converted=STRING"`
	DatabaseName            string `parquet:"name=database_name,             type=BYTE_ARRAY, converted=STRING"`
	ReplicaRole             string `parquet:"name=replica_role,              type=BYTE_ARRAY, converted=STRING"`
	OperationalState        string `parquet:"name=operational_state,         type=BYTE_ARRAY, converted=STRING"`
	ConnectedState          string `parquet:"name=connected_state,           type=BYTE_ARRAY, converted=STRING"`
	SynchronizationState    string `parquet:"name=synchronization_state,     type=BYTE_ARRAY, converted=STRING"`
	SyncStateDesc           string `parquet:"name=sync_state_desc,           type=BYTE_ARRAY, converted=STRING"`
	IsPrimaryReplica        bool   `parquet:"name=is_primary_replica,        type=BOOLEAN"`
	LogSendQueueKB          int64  `parquet:"name=log_send_queue_kb,         type=INT64"`
	RedoQueueKB             int64  `parquet:"name=redo_queue_kb,              type=INT64"`
	LogSendRateKB           int64  `parquet:"name=log_send_rate_kb,          type=INT64"`
	RedoRateKB              int64  `parquet:"name=redo_rate_kb,               type=INT64"`
	SecondaryLagSeconds     int64  `parquet:"name=secondary_lag_seconds,      type=INT64"`
}

// SQLServerRiskHealthRow is the Parquet schema for sqlserver_risk_health.
type SQLServerRiskHealthRow struct {
	CaptureTimestampMs   int64   `parquet:"name=capture_timestamp_ms,    type=INT64"`
	ServerID             string  `parquet:"name=server_id,               type=BYTE_ARRAY, converted=STRING"`
	BlockingSessions     int32   `parquet:"name=blocking_sessions,       type=INT32"`
	MemoryGrantsPending  int32   `parquet:"name=memory_grants_pending,   type=INT32"`
	FailedLogins5m       int32   `parquet:"name=failed_logins_5m,        type=INT32"`
	TempdbUsedPercent    float64 `parquet:"name=tempdb_used_percent,     type=DOUBLE"`
	MaxLogDBName         string  `parquet:"name=max_log_db_name,         type=BYTE_ARRAY, converted=STRING"`
	MaxLogUsedPercent    float64 `parquet:"name=max_log_used_percent,    type=DOUBLE"`
	PLE                  float64 `parquet:"name=ple,                     type=DOUBLE"`
	CompilationsPerSec   float64 `parquet:"name=compilations_per_sec,    type=DOUBLE"`
	BatchRequestsPerSec  float64 `parquet:"name=batch_requests_per_sec,  type=DOUBLE"`
	BufferCacheHitRatio  float64 `parquet:"name=buffer_cache_hit_ratio,  type=DOUBLE"`
}

// SQLServerMemoryMetricsRow is the Parquet schema for sqlserver_memory_metrics.
type SQLServerMemoryMetricsRow struct {
	CaptureTimestampMs   int64   `parquet:"name=capture_timestamp_ms,    type=INT64"`
	ServerID             string  `parquet:"name=server_id,               type=BYTE_ARRAY, converted=STRING"`
	SQLMemoryUsedMB      int64   `parquet:"name=sql_memory_used_mb,      type=INT64"`
	SQLMemoryTargetMB    int64   `parquet:"name=sql_memory_target_mb,    type=INT64"`
	OSTotalMemoryMB      int64   `parquet:"name=os_total_memory_mb,      type=INT64"`
	OSAvailableMemoryMB  int64   `parquet:"name=os_available_memory_mb,  type=INT64"`
	ProcessPhysicalLow   bool    `parquet:"name=process_physical_low,    type=BOOLEAN"`
	ProcessVirtualLow    bool    `parquet:"name=process_virtual_low,     type=BOOLEAN"`
	MemoryGrantsPending  int32   `parquet:"name=memory_grants_pending,   type=INT32"`
	ActiveMemoryGrants   int32   `parquet:"name=active_memory_grants,    type=INT32"`
	WaitingMemoryGrants  int32   `parquet:"name=waiting_memory_grants,   type=INT32"`
	GrantedWorkspaceMB   int64   `parquet:"name=granted_workspace_mb,    type=INT64"`
	RequestedWorkspaceMB int64   `parquet:"name=requested_workspace_mb,  type=INT64"`
	PLESeconds           int64   `parquet:"name=ple_seconds,             type=INT64"`
	PlanCacheMB          int64   `parquet:"name=plan_cache_mb,           type=INT64"`
	SortWarningsTotal    int64   `parquet:"name=sort_warnings_total,     type=INT64"`
	HashWarningsTotal    int64   `parquet:"name=hash_warnings_total,     type=INT64"`
	SortWarningsPerSec   float64 `parquet:"name=sort_warnings_per_sec,   type=DOUBLE"`
	HashWarningsPerSec   float64 `parquet:"name=hash_warnings_per_sec,   type=DOUBLE"`
}
