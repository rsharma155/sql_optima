// SQL Optima — https://github.com/rsharma155/sql_optima
//
// File: backend/internal/storage/cold/schemas/sqlserver_misc.go
// Purpose: Typed Parquet schemas for various SQL Server diagnostic metrics (Buffer Pool, Scheduler, Latches, Spinlocks).
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package schemas

// SQLServerBufferPoolRow is the Parquet schema for sqlserver_buffer_pool_db.
type SQLServerBufferPoolRow struct {
	CaptureTimestampMs int64  `parquet:"name=capture_timestamp_ms, type=INT64"`
	ServerID           string `parquet:"name=server_id,            type=BYTE_ARRAY, converted=STRING"`
	DatabaseName       string `parquet:"name=database_name,       type=BYTE_ARRAY, converted=STRING"`
	BufferMB           int64  `parquet:"name=buffer_mb,           type=INT64"`
}

// SQLServerSchedulerRow is the Parquet schema for sqlserver_cpu_scheduler_stats.
type SQLServerSchedulerRow struct {
	CaptureTimestampMs              int64   `parquet:"name=capture_timestamp_ms,                type=INT64"`
	ServerID                        string  `parquet:"name=server_id,                           type=BYTE_ARRAY, converted=STRING"`
	MaxWorkersCount                 int32   `parquet:"name=max_workers_count,                   type=INT32"`
	SchedulerCount                  int32   `parquet:"name=scheduler_count,                     type=INT32"`
	CPUCount                        int32   `parquet:"name=cpu_count,                           type=INT32"`
	TotalRunnableTasksCount         int32   `parquet:"name=total_runnable_tasks_count,          type=INT32"`
	TotalWorkQueueCount             int64   `parquet:"name=total_work_queue_count,              type=INT64"`
	TotalCurrentWorkersCount        int32   `parquet:"name=total_current_workers_count,         type=INT32"`
	ActiveWorkersCount              int32   `parquet:"name=active_workers_count,                type=INT32"`
	PendingDiskIOCount              int32   `parquet:"name=pending_disk_io_count,               type=INT32"`
	AvgRunnableTasksCount           float64 `parquet:"name=avg_runnable_tasks_count,            type=DOUBLE"`
	TotalActiveRequestCount         int32   `parquet:"name=total_active_request_count,          type=INT32"`
	TotalQueuedRequestCount         int32   `parquet:"name=total_queued_request_count,          type=INT32"`
	TotalBlockedTaskCount           int32   `parquet:"name=total_blocked_task_count,           type=INT32"`
	TotalActiveParallelThreadCount  int64   `parquet:"name=total_active_parallel_thread_count,  type=INT64"`
	RunnableRequestCount            int32   `parquet:"name=runnable_request_count,              type=INT32"`
	TotalRequestCount               int32   `parquet:"name=total_request_count,                 type=INT32"`
	RunnablePercent                 float64 `parquet:"name=runnable_percent,                    type=DOUBLE"`
	WorkerThreadExhaustionWarning   bool    `parquet:"name=worker_thread_exhaustion_warning,    type=BOOLEAN"`
	RunnableTasksWarning            bool    `parquet:"name=runnable_tasks_warning,              type=BOOLEAN"`
	BlockedTasksWarning             bool    `parquet:"name=blocked_tasks_warning,               type=BOOLEAN"`
	QueuedRequestsWarning           bool    `parquet:"name=queued_requests_warning,             type=BOOLEAN"`
	TotalPhysicalMemoryKB           int64   `parquet:"name=total_physical_memory_kb,            type=INT64"`
	AvailablePhysicalMemoryKB       int64   `parquet:"name=available_physical_memory_kb,        type=INT64"`
	SystemMemoryStateDesc           string  `parquet:"name=system_memory_state_desc,            type=BYTE_ARRAY, converted=STRING"`
	PhysicalMemoryPressureWarning   bool    `parquet:"name=physical_memory_pressure_warning,    type=BOOLEAN"`
	TotalNodeCount                  int32   `parquet:"name=total_node_count,                    type=INT32"`
	NodesOnlineCount                int32   `parquet:"name=nodes_online_count,                  type=INT32"`
	OfflineCPUCount                 int32   `parquet:"name=offline_cpu_count,                   type=INT32"`
	OfflineCPUWarning               bool    `parquet:"name=offline_cpu_warning,                 type=BOOLEAN"`
}

// SQLServerLatchRow is the Parquet schema for sqlserver_latch_waits.
type SQLServerLatchRow struct {
	CaptureTimestampMs int64  `parquet:"name=capture_timestamp_ms, type=INT64"`
	ServerID           string `parquet:"name=server_id,            type=BYTE_ARRAY, converted=STRING"`
	WaitType           string `parquet:"name=wait_type,            type=BYTE_ARRAY, converted=STRING"`
	WaitingTasksCount  int64  `parquet:"name=waiting_tasks_count,  type=INT64"`
	WaitTimeMs         int64  `parquet:"name=wait_time_ms,         type=INT64"`
	SignalWaitTimeMs   int64  `parquet:"name=signal_wait_time_ms,  type=INT64"`
}

// SQLServerWaitingTaskRow is the Parquet schema for sqlserver_waiting_tasks.
type SQLServerWaitingTaskRow struct {
	CaptureTimestampMs  int64  `parquet:"name=capture_timestamp_ms, type=INT64"`
	ServerID            string `parquet:"name=server_id,            type=BYTE_ARRAY, converted=STRING"`
	WaitType            string `parquet:"name=wait_type,            type=BYTE_ARRAY, converted=STRING"`
	ResourceDescription string `parquet:"name=resource_description, type=BYTE_ARRAY, converted=STRING"`
	WaitingTasksCount   int64  `parquet:"name=waiting_tasks_count,  type=INT64"`
	WaitDurationMs      int64  `parquet:"name=wait_duration_ms,     type=INT64"`
}

// SQLServerSpinlockRow is the Parquet schema for sqlserver_spinlock_stats.
type SQLServerSpinlockRow struct {
	CaptureTimestampMs int64  `parquet:"name=capture_timestamp_ms, type=INT64"`
	ServerID           string `parquet:"name=server_id,            type=BYTE_ARRAY, converted=STRING"`
	SpinlockType       string `parquet:"name=spinlock_type,        type=BYTE_ARRAY, converted=STRING"`
	Collisions         int64  `parquet:"name=collisions,           type=INT64"`
	Spins              int64  `parquet:"name=spins,                type=INT64"`
	SleepTimeMs        int64  `parquet:"name=sleep_time_ms,        type=INT64"`
}

// SQLServerMemoryGrantWaiterRow is the Parquet schema for sqlserver_memory_grant_waiters.
type SQLServerMemoryGrantWaiterRow struct {
	CaptureTimestampMs int64  `parquet:"name=capture_timestamp_ms, type=INT64"`
	ServerID           string `parquet:"name=server_id,            type=BYTE_ARRAY, converted=STRING"`
	SessionID          int32  `parquet:"name=session_id,           type=INT32"`
	RequestID          int32  `parquet:"name=request_id,           type=INT32"`
	DatabaseName       string `parquet:"name=database_name,        type=BYTE_ARRAY, converted=STRING"`
	LoginName          string `parquet:"name=login_name,           type=BYTE_ARRAY, converted=STRING"`
	RequestedMemoryKB  int64  `parquet:"name=requested_memory_kb,  type=INT64"`
	GrantedMemoryKB    int64  `parquet:"name=granted_memory_kb,    type=INT64"`
	RequiredMemoryKB   int64  `parquet:"name=required_memory_kb,   type=INT64"`
	WaitTimeMs         int64  `parquet:"name=wait_time_ms,         type=INT64"`
	DOP                int32  `parquet:"name=dop,                  type=INT32"`
	QueryText          string `parquet:"name=query_text,           type=BYTE_ARRAY, converted=STRING"`
}
