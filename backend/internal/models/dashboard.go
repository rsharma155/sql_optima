// Package models defines domain entities and data structures.
// It contains all the data models used throughout the application.
// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Core dashboard data models including DashboardMetrics, DiskStat, LockStat, QueryStat, MemoryStat, WaitStat, and BlockingNode structures.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package models

// DashboardMetrics precisely caches all Prometheus-styled queries
// mapped perfectly to the UI widgets dropping all Javascript Math.random() usage safely.
type DashboardMetrics struct {
	InstanceName string `json:"instance_name"`

	// Top Header KPIs
	AvgCPULoad  float64 `json:"avg_cpu_load"`
	MemoryUsage float64 `json:"memory_usage"`
	ActiveUsers int     `json:"active_users"`
	TotalLocks  int     `json:"total_locks"`
	Deadlocks   int     `json:"deadlocks"`

	LocksByDB map[string]LockStat `json:"locks_by_db"`
	DiskByDB  map[string]DiskStat `json:"disk_by_db"`

	// Timeseries Buffers
	CPUHistory  []CPUTick        `json:"cpu_history"`
	MemHistory  []float64        `json:"mem_history"`
	PLEHistory  []float64        `json:"ple_history"`
	WaitHistory []WaitSnapshot   `json:"wait_history"`
	FileHistory []FileIOSnapshot `json:"file_history"`
	FileStats   []FileIOStat     `json:"file_stats"`

	// Internal State tracking for Cumulative Deltas
	PrevWaitStats map[string]float64    `json:"-"`
	PrevFileStats map[string]FileIOStat `json:"-"`

	// Doughnut / Structure Charts
	DiskUsage       DiskStat         `json:"disk_usage"`
	TopQueries      []QueryStat      `json:"top_queries"`
	MemoryClerks    []MemoryStat     `json:"memory_clerks"`
	ActiveBlocks    []BlockStat      `json:"active_blocks"`
	ConnectionStats []ConnectionStat `json:"connection_stats"`

	// Extended Events Metrics
	XEventMetrics *XEventMetrics `json:"xevent_metrics,omitempty"`

	// Dynamic Full-Scale Telemetry directly from queries.yml
	PrometheusData map[string][]map[string]interface{} `json:"prometheus_data,omitempty"`

	Timestamp string `json:"timestamp"`
}

type DiskStat struct {
	DataMB float64 `json:"data_mb"`
	LogMB  float64 `json:"log_mb"`
	FreeMB float64 `json:"free_mb"`
}

type LockStat struct {
	TotalLocks int `json:"total_locks"`
	Deadlocks  int `json:"deadlocks"`
}

type QueryStat struct {
	LoginName      string  `json:"login_name"`
	ProgramName    string  `json:"program_name"`
	DatabaseName   string  `json:"database_name"`
	QueryText      string  `json:"query_text"`
	WaitType       string  `json:"wait_type"`
	CPUTimeMs      float64 `json:"cpu_time_ms"`
	ExecTimeMs     float64 `json:"exec_time_ms"`
	LogicalReads   int64   `json:"logical_reads"`
	ExecutionCount int64   `json:"execution_count"`
}

type MemoryStat struct {
	Type   string  `json:"type"`
	SizeMB float64 `json:"size_mb"`
}

type WaitStat struct {
	WaitType     string  `json:"wait_type"`
	WaitTimeMs   float64 `json:"wait_time_ms"`
	SignalWaitMs float64 `json:"signal_wait_ms"`
	WaitingTasks int64   `json:"waiting_tasks"`
}

type BlockingNode struct {
	SessionID          int    `json:"session_id"`
	BlockingSessionID  int    `json:"blocking_session_id"`
	LoginName          string `json:"login_name"`
	HostName           string `json:"host_name"`
	ProgramName        string `json:"program_name"`
	DatabaseName       string `json:"database_name"`
	QueryText          string `json:"query_text"`
	Status             string `json:"status"`
	Command            string `json:"command"`
	WaitType           string `json:"wait_type"`
	WaitTimeMs         int64  `json:"wait_time_ms"`
	CPUTimeMs          int64  `json:"cpu_time_ms"`
	TotalElapsedTimeMs int64  `json:"total_elapsed_time_ms"`
	RowCount           int64  `json:"row_count"`
	Level              int    `json:"level"`
}

type ActiveQuery struct {
	SessionID          int    `json:"session_id"`
	RequestID          int    `json:"request_id"`
	DatabaseName       string `json:"database_name"`
	LoginName          string `json:"login_name"`
	HostName           string `json:"host_name"`
	ProgramName        string `json:"program_name"`
	QueryText          string `json:"query_text"`
	Status             string `json:"status"`
	Command            string `json:"command"`
	WaitType           string `json:"wait_type"`
	WaitTimeMs         int64  `json:"wait_time_ms"`
	CPUTimeMs          int64  `json:"cpu_time_ms"`
	TotalElapsedTimeMs int64  `json:"total_elapsed_time_ms"`
	Reads              int64  `json:"reads"`
	Writes             int64  `json:"writes"`
	GrantedMemoryMB    int    `json:"granted_memory_mb"`
	RowCount           int64  `json:"row_count"`
	PercentComplete    string `json:"percent_complete"`
}

type LongRunningQuery struct {
	SessionID            int    `json:"session_id"`
	RequestID            int    `json:"request_id"`
	DatabaseName         string `json:"database_name"`
	LoginName            string `json:"login_name"`
	HostName             string `json:"host_name"`
	ProgramName          string `json:"program_name"`
	QueryText            string `json:"query_text"`
	WaitType             string `json:"wait_type"`
	BlockingSessionID    int    `json:"blocking_session_id"`
	Status               string `json:"status"`
	CPUTimeMs            int64  `json:"cpu_time_ms"`
	TotalElapsedTimeMs   int64  `json:"total_elapsed_time_ms"`
	Reads                int64  `json:"reads"`
	Writes               int64  `json:"writes"`
	GrantedQueryMemoryMB int    `json:"granted_query_memory_mb"`
	RowCount             int64  `json:"row_count"`
}

type TempDBStats struct {
	DatabaseName      string  `json:"database_name"`
	TotalDataFiles    int     `json:"total_data_files"`
	TotalSizeMB       int     `json:"total_size_mb"`
	UsedSpaceMB       int     `json:"used_space_mb"`
	PFSContentionPct  float64 `json:"pfs_contention_pct"`
	GAMContentionPct  float64 `json:"gam_contention_pct"`
	SGAMContentionPct float64 `json:"sgam_contention_pct"`
}

type MemoryStats struct {
	AvailableMB  int     `json:"available_mb"`
	TotalMB      int     `json:"total_mb"`
	UsagePercent float64 `json:"usage_percent"`
	CapturedAt   string  `json:"captured_at"`
}

type CPUTick struct {
	SQLProcess   float64 `json:"sql_process"`
	SystemIdle   float64 `json:"system_idle"`
	OtherProcess float64 `json:"other_process"`
	EventTime    string  `json:"event_time"`
}

type BlockStat struct {
	BlockedSessionID  int    `json:"blocked_session_id"`
	BlockingSessionID int    `json:"blocking_session_id"`
	DatabaseName      string `json:"database_name"`
	WaitType          string `json:"wait_type"`
	WaitTimeMs        int    `json:"wait_time_ms"`
	QueryText         string `json:"query_text"`
	Status            string `json:"status"`
	HostName          string `json:"host_name"`
	ProgramName       string `json:"program_name"`
}

type ConnectionStat struct {
	LoginName         string `json:"login_name"`
	DatabaseName      string `json:"database_name"`
	ActiveConnections int    `json:"active_connections"`
	ActiveRequests    int    `json:"active_requests"`
}

type WaitSnapshot struct {
	Timestamp   string  `json:"timestamp"`
	DiskRead    float64 `json:"disk_read"`
	Blocking    float64 `json:"blocking"`
	Parallelism float64 `json:"parallelism"`
	Other       float64 `json:"other"`
}

type FileIOStat struct {
	DatabaseName   string  `json:"database_name"`
	PhysicalName   string  `json:"physical_name"`
	FileType       string  `json:"file_type"`
	ReadLatencyMs  float64 `json:"read_latency_ms"`
	WriteLatencyMs float64 `json:"write_latency_ms"`

	// Internal Tracking Pointers
	NumOfReads     int64 `json:"num_of_reads"`
	NumOfWrites    int64 `json:"num_of_writes"`
	IoStallReadMs  int64 `json:"io_stall_read_ms"`
	IoStallWriteMs int64 `json:"io_stall_write_ms"`
}

type FileIOSnapshot struct {
	Timestamp string       `json:"timestamp"`
	Files     []FileIOStat `json:"files"`
}

// XEventMetrics provides aggregated extended events data for dashboard display
type XEventMetrics struct {
	ServerInstanceName  string             `json:"server_instance_name"`
	TotalEventsLastHour int                `json:"total_events_last_hour"`
	EventCounts         map[string]int     `json:"event_counts"`
	RecentEvents        []SqlServerXeEvent `json:"recent_events"`
	Timestamp           string             `json:"timestamp"`
}

// CPUSchedulerStats represents CPU scheduler and workload group metrics with pressure warnings
type CPUSchedulerStats struct {
	CaptureTimestamp               string  `json:"capture_timestamp"`
	ServerInstanceName             string  `json:"server_instance_name"`
	MaxWorkersCount                int     `json:"max_workers_count"`
	SchedulerCount                 int     `json:"scheduler_count"`
	CPUCount                       int     `json:"cpu_count"`
	TotalRunnableTasksCount        int     `json:"total_runnable_tasks_count"`
	TotalWorkQueueCount            int64   `json:"total_work_queue_count"`
	TotalCurrentWorkersCount       int     `json:"total_current_workers_count"`
	AvgRunnableTasksCount          float64 `json:"avg_runnable_tasks_count"`
	TotalActiveRequestCount        int     `json:"total_active_request_count"`
	TotalQueuedRequestCount        int     `json:"total_queued_request_count"`
	TotalBlockedTaskCount          int     `json:"total_blocked_task_count"`
	TotalActiveParallelThreadCount int64   `json:"total_active_parallel_thread_count"`
	RunnableRequestCount           int     `json:"runnable_request_count"`
	TotalRequestCount              int     `json:"total_request_count"`
	RunnablePercent                float64 `json:"runnable_percent"`
	WorkerThreadExhaustionWarning  bool    `json:"worker_thread_exhaustion_warning"`
	RunnableTasksWarning           bool    `json:"runnable_tasks_warning"`
	BlockedTasksWarning            bool    `json:"blocked_tasks_warning"`
	QueuedRequestsWarning          bool    `json:"queued_requests_warning"`
	TotalPhysicalMemoryKB          int64   `json:"total_physical_memory_kb"`
	AvailablePhysicalMemoryKB      int64   `json:"available_physical_memory_kb"`
	SystemMemoryStateDesc          string  `json:"system_memory_state_desc"`
	PhysicalMemoryPressureWarning  bool    `json:"physical_memory_pressure_warning"`
	TotalNodeCount                 int     `json:"total_node_count"`
	NodesOnlineCount               int     `json:"nodes_online_count"`
	OfflineCPUCount                int     `json:"offline_cpu_count"`
	OfflineCPUWarning              bool    `json:"offline_cpu_warning"`
}

// ServerProperties represents server hardware properties
type ServerProperties struct {
	CaptureTimestamp   string  `json:"capture_timestamp"`
	ServerInstanceName string  `json:"server_instance_name"`
	CPUCount           int     `json:"cpu_count"`
	HyperthreadRatio   int     `json:"hyperthread_ratio"`
	SocketCount        int     `json:"socket_count"`
	CoresPerSocket     int     `json:"cores_per_socket"`
	PhysicalMemoryGB   float64 `json:"physical_memory_gb"`
	VirtualMemoryGB    float64 `json:"virtual_memory_gb"`
	CPUType            string  `json:"cpu_type"`
	HyperthreadEnabled bool    `json:"hyperthread_enabled"`
	NUMANodes          int     `json:"numa_nodes"`
	MaxWorkersCount    int     `json:"max_workers_count"`
	PropertiesHash     string  `json:"properties_hash"`
}

// CPULastCollected tracks the last collection timestamps for deduplication
type CPULastCollected struct {
	ServerInstanceName string `json:"server_instance_name"`
	LastCollectionTime string `json:"last_collection_time"`
	LastCPUHistoryTime string `json:"last_cpu_history_time"`
	LastSchedulerTime  string `json:"last_scheduler_time"`
	LastPropertiesTime string `json:"last_properties_time"`
	UpdatedAt          string `json:"updated_at"`
}
