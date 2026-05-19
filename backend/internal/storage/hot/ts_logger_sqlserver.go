// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: SQL Server-specific TimescaleDB logger for enterprise metrics.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package hot

import (
	"fmt"
	"log/slog"
	"context"
	"database/sql"
	"hash/fnv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type WaitSnapshotRow struct {
	CaptureTimestamp time.Time `json:"capture_timestamp"`
	ServerID         uuid.UUID `json:"server_id"`
	DiskRead         float64   `json:"disk_read"`
	Blocking         float64   `json:"blocking"`
	Parallelism      float64   `json:"parallelism"`
	Other            float64   `json:"other"`
}

type SQLServerMetricRow struct {
	CaptureTimestamp time.Time `json:"capture_timestamp" db:"capture_timestamp"`
	ServerID         uuid.UUID `json:"server_id" db:"server_id"`
	ServerName       string    `json:"server_name" db:"-"`
	AvgCpuLoad       float64   `json:"avg_cpu_load" db:"avg_cpu_load"`
	MemoryUsage      float64   `json:"memory_usage" db:"memory_usage"`
	ActiveUsers      int       `json:"active_users" db:"active_users"`
	TotalLocks       int64     `json:"total_locks" db:"total_locks"`
	Deadlocks        int64     `json:"deadlocks" db:"deadlocks"`
	DataDiskMB       float64   `json:"data_disk_mb" db:"data_disk_mb"`
	LogDiskMB        float64   `json:"log_disk_mb" db:"log_disk_mb"`
	FreeDiskMB       float64   `json:"free_disk_mb" db:"free_disk_mb"`
	CpuWait          float64   `json:"cpu_wait" db:"-"`
	DiskWait         float64   `json:"disk_wait" db:"-"`
	LockWait         float64   `json:"lock_wait" db:"-"`
	NetworkWait      float64   `json:"network_wait" db:"-"`
}

type CPUSchedulerStatsRow struct {
	CaptureTimestamp              time.Time `json:"capture_timestamp"`
	ServerID                      uuid.UUID `json:"server_id"`
	MaxWorkersCount               int       `json:"max_workers_count"`
	SchedulerCount                int       `json:"scheduler_count"`
	CPUCount                      int       `json:"cpu_count"`
	TotalRunnableTasksCount       int       `json:"total_runnable_tasks_count"`
	TotalWorkQueueCount           int64     `json:"total_work_queue_count"`
	TotalCurrentWorkersCount      int       `json:"total_current_workers_count"`
	AvgRunnableTasksCount         float64   `json:"avg_runnable_tasks_count"`
	TotalActiveRequestCount       int       `json:"total_active_request_count"`
	TotalQueuedRequestCount       int       `json:"total_queued_request_count"`
	TotalBlockedTaskCount         int       `json:"total_blocked_task_count"`
	RunnablePercent               float64   `json:"runnable_percent"`
	WorkerThreadExhaustionWarning bool      `json:"worker_thread_exhaustion_warning"`
	RunnableTasksWarning          bool      `json:"runnable_tasks_warning"`
	BlockedTasksWarning           bool      `json:"blocked_tasks_warning"`
	QueuedRequestsWarning         bool      `json:"queued_requests_warning"`
	TotalPhysicalMemoryKB         int64     `json:"total_physical_memory_kb"`
	AvailablePhysicalMemoryKB     int64     `json:"available_physical_memory_kb"`
	SystemMemoryStateDesc         string    `json:"system_memory_state_desc"`
	PhysicalMemoryPressureWarning bool      `json:"physical_memory_pressure_warning"`
	TotalNodeCount                int       `json:"total_node_count"`
	NodesOnlineCount              int       `json:"nodes_online_count"`
	OfflineCPUCount               int       `json:"offline_cpu_count"`
	OfflineCPUWarning             bool      `json:"offline_cpu_warning"`
}

type ServerPropertiesRow struct {
	CaptureTimestamp   time.Time `json:"capture_timestamp"`
	ServerID           uuid.UUID `json:"server_id"`
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

func (tl *TimescaleLogger) GetLatestSQLServerWaitHistory(ctx context.Context, serverID uuid.UUID, limit int) ([]WaitSnapshotRow, error) {
	if limit <= 0 {
		limit = 1
	}
	q := `
		SELECT capture_timestamp, server_id, disk_read_ms_per_sec, blocking_ms_per_sec, parallelism_ms_per_sec, other_ms_per_sec
		FROM sqlserver_wait_history
		WHERE server_id = $1
		ORDER BY capture_timestamp DESC
		LIMIT $2
	`
	rows, err := tl.pool.Query(ctx, q, serverID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []WaitSnapshotRow
	for rows.Next() {
		var r WaitSnapshotRow
		if err := rows.Scan(&r.CaptureTimestamp, &r.ServerID, &r.DiskRead, &r.Blocking, &r.Parallelism, &r.Other); err != nil {
			continue
		}
		results = append(results, r)
	}
	return results, nil
}

func (tl *TimescaleLogger) GetSQLServerMetrics(ctx context.Context, serverID uuid.UUID, limit int) ([]SQLServerMetricRow, error) {
	if limit <= 0 {
		limit = 100
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	query := `
		SELECT capture_timestamp, server_id, avg_cpu_load, memory_usage,
		       active_users, total_locks, deadlocks, data_disk_mb, log_disk_mb, free_disk_mb
		FROM sqlserver_metrics
		WHERE server_id = $1
		ORDER BY capture_timestamp DESC
		LIMIT $2
	`

	rows, err := tl.pool.Query(ctx, query, serverID, limit)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			slog.Info("[TSLogger] GetSQLServerMetrics timed out after 5s")
			return []SQLServerMetricRow{}, nil
		}
		slog.Error("[TSLogger] Failed to query SQLServer metrics", "err", err)
		return nil, err
	}
	defer rows.Close()

	var results []SQLServerMetricRow
	for rows.Next() {
		var r SQLServerMetricRow
		if err := rows.Scan(&r.CaptureTimestamp, &r.ServerID, &r.AvgCpuLoad, &r.MemoryUsage,
			&r.ActiveUsers, &r.TotalLocks, &r.Deadlocks, &r.DataDiskMB, &r.LogDiskMB, &r.FreeDiskMB); err != nil {
			slog.Error("[TSLogger] Failed to scan row", "err", err)
			continue
		}
		r.CpuWait, r.DiskWait, r.LockWait, r.NetworkWait = 0, 0, 0, 0
		results = append(results, r)
	}
	return results, rows.Err()
}

func (tl *TimescaleLogger) GetSQLServerMetricsTimeRange(ctx context.Context, serverID uuid.UUID, start, end time.Time) ([]SQLServerMetricRow, error) {
	query := `
		SELECT capture_timestamp, server_id, avg_cpu_load, memory_usage,
		       active_users, total_locks, deadlocks, data_disk_mb, log_disk_mb, free_disk_mb
		FROM sqlserver_metrics
		WHERE server_id = $1
		  AND capture_timestamp >= $2
		  AND capture_timestamp <= $3
		ORDER BY capture_timestamp ASC
	`

	rows, err := tl.pool.Query(ctx, query, serverID, start, end)
	if err != nil {
		slog.Error("[TSLogger] Failed to query SQLServer metrics", "err", err)
		return nil, err
	}
	defer rows.Close()

	var results []SQLServerMetricRow
	for rows.Next() {
		var r SQLServerMetricRow
		if err := rows.Scan(&r.CaptureTimestamp, &r.ServerID, &r.AvgCpuLoad, &r.MemoryUsage,
			&r.ActiveUsers, &r.TotalLocks, &r.Deadlocks, &r.DataDiskMB, &r.LogDiskMB, &r.FreeDiskMB); err != nil {
			slog.Error("[TSLogger] Failed to scan row", "err", err)
			continue
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

func (tl *TimescaleLogger) LogSQLServerMetrics(ctx context.Context, serverID uuid.UUID, data map[string]interface{}) error {
	query := `
		INSERT INTO sqlserver_metrics (
			capture_timestamp, server_id, avg_cpu_load, memory_usage,
			active_users, total_locks, deadlocks, data_disk_mb, log_disk_mb, free_disk_mb
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`
	now := time.Now().UTC()

	_, err := tl.pool.Exec(ctx, query,
		now, serverID,
		getFloat64(data, "avg_cpu_load"),
		getFloat64(data, "memory_usage"),
		getInt(data, "active_users"),
		getInt(data, "total_locks"),
		getInt(data, "deadlocks"),
		getFloat64(data, "data_disk_mb"),
		getFloat64(data, "log_disk_mb"),
		getFloat64(data, "free_disk_mb"),
	)
	return err
}

func (tl *TimescaleLogger) LogSQLServerCPUHistory(ctx context.Context, serverID uuid.UUID, ticks []map[string]interface{}) error {
	if len(ticks) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	query := `
		INSERT INTO sqlserver_cpu_history (capture_timestamp, server_id, sql_process, system_idle, other_process)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (capture_timestamp, server_id) DO NOTHING`

	now := time.Now().UTC()

	for _, tick := range ticks {
		ts := now
		if et, ok := tick["event_time"].(string); ok {
			if t, err := time.Parse("2006-01-02 15:04:05", et); err == nil {
				ts = t.UTC()
			} else if t, err := time.Parse(time.RFC3339, et); err == nil {
				ts = t.UTC()
			}
		} else if et, ok := tick["event_time"].(time.Time); ok {
			ts = et.UTC()
		}

		batch.Queue(query, ts, serverID, tick["sql_process"], tick["system_idle"], tick["other_process"])
	}

	res := tl.pool.SendBatch(ctx, batch)
	return res.Close()
}
func (tl *TimescaleLogger) LogSQLServerMemoryHistory(ctx context.Context, serverID uuid.UUID, ple float64) error {
	if ple < 0 {
		return nil
	}

	query := `INSERT INTO sqlserver_memory_history (capture_timestamp, server_id, page_life_expectancy) VALUES ($1, $2, $3)`
	now := time.Now().UTC()
	_, err := tl.pool.Exec(ctx, query, now, serverID, ple)
	return err
}

func (tl *TimescaleLogger) LogCollectorError(ctx context.Context, serverID uuid.UUID, errMsg string) error {
	// Ensure column exists (one-time check/fix)
	setupSQL := `
        DO $$ 
        BEGIN 
            IF NOT EXISTS (SELECT 1 FROM information_schema.columns 
                           WHERE table_name='sqlserver_collector_instance_state' AND column_name='last_error') THEN 
                ALTER TABLE sqlserver_collector_instance_state ADD COLUMN last_error TEXT; 
            END IF; 
        END $$;
    `
	_, _ = tl.pool.Exec(ctx, setupSQL)

	q := `
        INSERT INTO sqlserver_collector_instance_state (server_id, last_error)
        VALUES ($1, $2)
        ON CONFLICT (server_id) DO UPDATE SET
            last_error = EXCLUDED.last_error;
    `
	_, err := tl.pool.Exec(ctx, q, serverID, errMsg)
	return err
}

func (tl *TimescaleLogger) LogSQLServerWaitHistory(ctx context.Context, serverID uuid.UUID, waits []map[string]interface{}) error {
	if len(waits) == 0 {
		return nil
	}

	now := time.Now().UTC()
	batch := &pgx.Batch{}
	for _, wait := range waits {
		query := `INSERT INTO sqlserver_wait_history (
			capture_timestamp, server_id, wait_type, wait_time_ms_total,
			disk_read_ms_per_sec, blocking_ms_per_sec, parallelism_ms_per_sec, other_ms_per_sec
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`
		batch.Queue(query, now, serverID,
			getStr(wait, "wait_type"),
			getInt64FromMap(wait, "wait_time_ms_total"),
			getFloat64(wait, "disk_read"),
			getFloat64(wait, "blocking"),
			getFloat64(wait, "parallelism"),
			getFloat64(wait, "other"),
		)
	}
	br := tl.pool.SendBatch(ctx, batch)
	return br.Close()
}

func (tl *TimescaleLogger) GetLatestSQLServerConnectionSnapshots(ctx context.Context, serverID uuid.UUID, limit int) ([]map[string]interface{}, error) {
	if limit <= 0 {
		limit = 100
	}
	q := `
		SELECT database_name, login_name, active_connections, active_requests, capture_timestamp
		FROM (
			SELECT database_name, login_name, active_connections, active_requests, capture_timestamp,
				ROW_NUMBER() OVER (PARTITION BY COALESCE(database_name, '') ORDER BY capture_timestamp DESC) AS rn
			FROM sqlserver_connection_history
			WHERE server_id = $1
			  AND (login_name IS NULL OR login_name <> 'dbmonitor_user')
		) t
		WHERE rn = 1
		ORDER BY database_name
		LIMIT $2
	`
	rows, err := tl.pool.Query(ctx, q, serverID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []map[string]interface{}
	for rows.Next() {
		var dbName, login sql.NullString
		var ac, ar int
		var ts time.Time
		if err := rows.Scan(&dbName, &login, &ac, &ar, &ts); err != nil {
			continue
		}
		out = append(out, map[string]interface{}{
			"database_name":      dbName.String,
			"login_name":         login.String,
			"active_connections": ac,
			"active_requests":    ar,
			"capture_timestamp":  ts,
		})
	}
	return out, rows.Err()
}

func (tl *TimescaleLogger) LogSQLServerConnectionHistory(ctx context.Context, serverID uuid.UUID, conns map[string]map[string]interface{}) error {
	const q = `INSERT INTO sqlserver_connection_history (capture_timestamp, server_id, database_name, login_name, active_connections, active_requests)
		VALUES ($1, $2, $3, $4, $5, $6)`
	now := time.Now().UTC()

	tl.mu.Lock()
	defer tl.mu.Unlock()

	for dbName, conn := range conns {
		loginName := tl.ToSafeUTF8(getStr(conn, "login_name"))
		ac := getInt(conn, "active_connections")
		ar := getInt(conn, "active_requests")

		key := serverID.String() + ":" + dbName + ":" + loginName
		h := fnv.New64a()
		b := [8]byte{byte(ac), byte(ac >> 8), byte(ac >> 16), byte(ac >> 24), byte(ar), byte(ar >> 8), byte(ar >> 16), byte(ar >> 24)}
		_, _ = h.Write([]byte(key))
		_, _ = h.Write(b[:])
		newHash := h.Sum64()

		if prev, ok := tl.prevConnHashByKey[key]; ok && prev == newHash {
			continue // values unchanged since last write
		}
		tl.prevConnHashByKey[key] = newHash

		if _, err := tl.pool.Exec(ctx, q, now, serverID, tl.ToSafeUTF8(dbName), loginName, ac, ar); err != nil {
			slog.Error("[TSLogger] Connection history log failed", "err", err)
		}
	}
	return nil
}

func (tl *TimescaleLogger) LogSQLServerLockHistory(ctx context.Context, serverID uuid.UUID, locks map[string]map[string]interface{}) error {
	for dbName, lock := range locks {
		query := `INSERT INTO sqlserver_lock_history (capture_timestamp, server_id, database_name, total_locks, deadlocks)
			VALUES ($1, $2, $3, $4, $5)`
		now := time.Now().UTC()
		_, err := tl.pool.Exec(ctx, query, now, serverID, tl.ToSafeUTF8(dbName), getInt(lock, "total_locks"), getInt(lock, "deadlocks"))
		if err != nil {
			slog.Error("[TSLogger] Failed to log lock history", "err", err)
		}
	}
	return nil
}

func (tl *TimescaleLogger) LogSQLServerDiskHistory(ctx context.Context, serverID uuid.UUID, disk map[string]map[string]interface{}) error {
	now := time.Now().UTC()
	const q = `INSERT INTO sqlserver_disk_history
		(capture_timestamp, server_id, database_name, data_mb, log_mb, free_mb, delta_data_mb, delta_log_mb)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (server_id, database_name, capture_timestamp) DO NOTHING`

	tl.mu.Lock()
	defer tl.mu.Unlock()

	for dbName, d := range disk {
		dataMB := getFloat64(d, "data_mb")
		logMB := getFloat64(d, "log_mb")
		freeMB := getFloat64(d, "free_mb")

		key := serverID.String() + ":" + dbName
		newHash := diskRowHash(dataMB, logMB, freeMB)
		if prev, ok := tl.prevDiskHashByKey[key]; ok && prev == newHash {
			continue // identical snapshot — skip write
		}

		var deltaMB, deltaLog float64
		if prev, ok := tl.prevDiskHistory[serverID.String()]; ok {
			if prevDB, ok := prev[dbName]; ok {
				prevMap, _ := prevDB.(map[string]interface{})
				deltaMB = dataMB - getFloat64(prevMap, "data_mb")
				deltaLog = logMB - getFloat64(prevMap, "log_mb")
			}
		}
		if tl.prevDiskHistory[serverID.String()] == nil {
			tl.prevDiskHistory[serverID.String()] = make(map[string]interface{})
		}
		tl.prevDiskHistory[serverID.String()][dbName] = map[string]interface{}{
			"data_mb": dataMB, "log_mb": logMB, "free_mb": freeMB,
		}
		tl.prevDiskHashByKey[key] = newHash

		if _, err := tl.pool.Exec(ctx, q, now, serverID, dbName, dataMB, logMB, freeMB, deltaMB, deltaLog); err != nil {
			slog.Error(fmt.Sprintf("[TSLogger] Failed to log disk history for %s/%s: %v", serverID, dbName, err))
		}
	}
	return nil
}

func (tl *TimescaleLogger) LogCPUSchedulerStats(ctx context.Context, serverID uuid.UUID, statsMap map[string]interface{}) error {
	sig := cpuSchedulerSnapshotHash(statsMap)
	tl.mu.Lock()
	if prev, ok := tl.prevSchedulerStats[serverID]; ok && prev == sig {
		tl.mu.Unlock()
		return nil
	}
	tl.prevSchedulerStats[serverID] = sig
	tl.mu.Unlock()

	query := `INSERT INTO sqlserver_cpu_scheduler_stats (
		capture_timestamp, server_id,
		max_workers_count, scheduler_count, cpu_count,
		total_runnable_tasks_count, total_work_queue_count, total_current_workers_count,
		active_workers_count, pending_disk_io_count,
		avg_runnable_tasks_count, total_active_request_count, total_queued_request_count,
		total_blocked_task_count, total_active_parallel_thread_count,
		runnable_request_count, total_request_count, runnable_percent,
		worker_thread_exhaustion_warning, runnable_tasks_warning,
		blocked_tasks_warning, queued_requests_warning,
		total_physical_memory_kb, available_physical_memory_kb,
		system_memory_state_desc, physical_memory_pressure_warning,
		total_node_count, nodes_online_count, offline_cpu_count, offline_cpu_warning
	) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28, $29, $30)`

	now := time.Now().UTC()
	_, err := tl.pool.Exec(ctx, query, now, serverID,
		getInt(statsMap, "max_workers_count"),
		getInt(statsMap, "scheduler_count"),
		getInt(statsMap, "cpu_count"),
		getInt(statsMap, "total_runnable_tasks_count"),
		getInt(statsMap, "total_work_queue_count"),
		getInt(statsMap, "total_current_workers_count"),
		getInt(statsMap, "active_workers_count"),
		getInt(statsMap, "pending_disk_io_count"),
		getFloat64(statsMap, "avg_runnable_tasks_count"),
		getInt(statsMap, "total_active_request_count"),
		getInt(statsMap, "total_queued_request_count"),
		getInt(statsMap, "total_blocked_task_count"),
		getInt(statsMap, "total_active_parallel_thread_count"),
		getInt(statsMap, "runnable_request_count"),
		getInt(statsMap, "total_request_count"),
		getFloat64(statsMap, "runnable_percent"),
		getBool(statsMap, "worker_thread_exhaustion_warning"),
		getBool(statsMap, "runnable_tasks_warning"),
		getBool(statsMap, "blocked_tasks_warning"),
		getBool(statsMap, "queued_requests_warning"),
		getInt(statsMap, "total_physical_memory_kb"),
		getInt(statsMap, "available_physical_memory_kb"),
		getStr(statsMap, "system_memory_state_desc"),
		getBool(statsMap, "physical_memory_pressure_warning"),
		getInt(statsMap, "total_node_count"),
		getInt(statsMap, "nodes_online_count"),
		getInt(statsMap, "offline_cpu_count"),
		getBool(statsMap, "offline_cpu_warning"),
	)
	return err
}

func (tl *TimescaleLogger) GetCPUSchedulerStats(ctx context.Context, serverID uuid.UUID, limit int) ([]CPUSchedulerStatsRow, error) {
	if limit <= 0 {
		limit = 100
	}

	query := `
		SELECT capture_timestamp, server_id,
		       max_workers_count, scheduler_count, cpu_count,
		       total_runnable_tasks_count, total_work_queue_count, total_current_workers_count,
		       avg_runnable_tasks_count, total_active_request_count, total_queued_request_count,
		       total_blocked_task_count,
		       runnable_percent,
		       worker_thread_exhaustion_warning, runnable_tasks_warning, blocked_tasks_warning, queued_requests_warning,
		       total_physical_memory_kb, available_physical_memory_kb, system_memory_state_desc, physical_memory_pressure_warning,
		       total_node_count, nodes_online_count, offline_cpu_count, offline_cpu_warning
		FROM sqlserver_cpu_scheduler_stats
		WHERE server_id = $1
		ORDER BY capture_timestamp DESC
		LIMIT $2
	`

	rows, err := tl.pool.Query(ctx, query, serverID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []CPUSchedulerStatsRow
	for rows.Next() {
		var r CPUSchedulerStatsRow
		if err := rows.Scan(&r.CaptureTimestamp, &r.ServerID,
			&r.MaxWorkersCount, &r.SchedulerCount, &r.CPUCount,
			&r.TotalRunnableTasksCount, &r.TotalWorkQueueCount, &r.TotalCurrentWorkersCount,
			&r.AvgRunnableTasksCount, &r.TotalActiveRequestCount, &r.TotalQueuedRequestCount,
			&r.TotalBlockedTaskCount,
			&r.RunnablePercent,
			&r.WorkerThreadExhaustionWarning, &r.RunnableTasksWarning, &r.BlockedTasksWarning, &r.QueuedRequestsWarning,
			&r.TotalPhysicalMemoryKB, &r.AvailablePhysicalMemoryKB, &r.SystemMemoryStateDesc, &r.PhysicalMemoryPressureWarning,
			&r.TotalNodeCount, &r.NodesOnlineCount, &r.OfflineCPUCount, &r.OfflineCPUWarning,
		); err != nil {
			continue
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

func (tl *TimescaleLogger) LogServerProperties(ctx context.Context, serverID uuid.UUID, propsMap map[string]interface{}) error {
	query := `INSERT INTO sqlserver_server_properties (
		capture_timestamp, server_id,
		cpu_count, hyperthread_ratio, socket_count, cores_per_socket,
		physical_memory_gb, virtual_memory_gb, cpu_type,
		hyperthread_enabled, numa_nodes, max_workers_count, properties_hash
	) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`

	now := time.Now().UTC()
	_, err := tl.pool.Exec(ctx, query, now, serverID,
		getInt(propsMap, "cpu_count"),
		getFloat64(propsMap, "hyperthread_ratio"),
		getInt(propsMap, "socket_count"),
		getInt(propsMap, "cores_per_socket"),
		getFloat64(propsMap, "physical_memory_gb"),
		getFloat64(propsMap, "virtual_memory_gb"),
		getStr(propsMap, "cpu_type"),
		getBool(propsMap, "hyperthread_enabled"),
		getInt(propsMap, "numa_nodes"),
		getInt(propsMap, "max_workers_count"),
		getStr(propsMap, "properties_hash"),
	)
	return err
}

func (tl *TimescaleLogger) GetServerProperties(ctx context.Context, serverID uuid.UUID) (*ServerPropertiesRow, error) {
	query := `
		SELECT capture_timestamp, server_id,
		       cpu_count, hyperthread_ratio, socket_count, cores_per_socket,
		       physical_memory_gb, virtual_memory_gb, cpu_type,
		       hyperthread_enabled, numa_nodes, max_workers_count, properties_hash
		FROM sqlserver_server_properties
		WHERE server_id = $1
		ORDER BY capture_timestamp DESC
		LIMIT 1
	`

	var r ServerPropertiesRow
	err := tl.pool.QueryRow(ctx, query, serverID).Scan(
		&r.CaptureTimestamp, &r.ServerID,
		&r.CPUCount, &r.HyperthreadRatio, &r.SocketCount, &r.CoresPerSocket,
		&r.PhysicalMemoryGB, &r.VirtualMemoryGB, &r.CPUType,
		&r.HyperthreadEnabled, &r.NUMANodes, &r.MaxWorkersCount, &r.PropertiesHash,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &r, nil
}

// GetSQLServerCPUHistory returns sqlserver_cpu_history rows for [from, to] (RFC3339), ordered by time.
func (tl *TimescaleLogger) GetSQLServerCPUHistory(ctx context.Context, serverID uuid.UUID, from, to string, limit int) ([]map[string]interface{}, error) {
	start, end, err := parseTimeRangeRFC3339(from, to)
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 10000 {
		limit = 2000
	}

	q := `
		SELECT capture_timestamp, sql_process, system_idle, other_process
		FROM sqlserver_cpu_history
		WHERE server_id = $1
		  AND capture_timestamp >= $2
		  AND capture_timestamp <= $3
		ORDER BY capture_timestamp ASC
		LIMIT $4`

	rows, err := tl.pool.Query(ctx, q, serverID, start, end, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []map[string]interface{}
	for rows.Next() {
		var ts time.Time
		var sqlProc, sysIdle, other sql.NullFloat64
		if err := rows.Scan(&ts, &sqlProc, &sysIdle, &other); err != nil {
			continue
		}
		sp := 0.0
		if sqlProc.Valid {
			sp = sqlProc.Float64
		}
		si := 0.0
		if sysIdle.Valid {
			si = sysIdle.Float64
		}
		ot := 0.0
		if other.Valid {
			ot = other.Float64
		}
		out = append(out, map[string]interface{}{
			"capture_timestamp": ts,
			"event_time":        ts.Format(time.RFC3339),
			"sql_process":       sp,
			"system_idle":       si,
			"other_process":     ot,
		})
	}
	return out, rows.Err()
}

func (tl *TimescaleLogger) OpenMsBlockingIncident(ctx context.Context, serverID uuid.UUID, startedAt time.Time, rootPID *int, rootQuery string) (int64, error) {
	q := `
		INSERT INTO sqlserver_blocking_incidents (server_id, started_at, root_blocker_pid, root_blocker_query, status)
		VALUES ($1, $2, $3, $4, 'active')
		RETURNING incident_id
	`
	var id int64
	err := tl.pool.QueryRow(ctx, q, serverID, startedAt, rootPID, rootQuery).Scan(&id)
	return id, err
}

func (tl *TimescaleLogger) UpdateMsBlockingIncident(ctx context.Context, incidentID int64, peakVictims int, rootPID *int, rootQuery string) error {
	q := `
		UPDATE sqlserver_blocking_incidents
		SET peak_blocked_sessions = GREATEST(COALESCE(peak_blocked_sessions, 0), $2),
		    root_blocker_pid = COALESCE($3, root_blocker_pid),
		    root_blocker_query = CASE WHEN $4 <> '' THEN $4 ELSE root_blocker_query END
		WHERE incident_id = $1
	`
	_, err := tl.pool.Exec(ctx, q, incidentID, peakVictims, rootPID, rootQuery)
	return err
}

func (tl *TimescaleLogger) CloseMsBlockingIncident(ctx context.Context, incidentID int64, endedAt time.Time) error {
	q := `
		UPDATE sqlserver_blocking_incidents
		SET ended_at = $2, status = 'resolved'
		WHERE incident_id = $1
	`
	_, err := tl.pool.Exec(ctx, q, incidentID, endedAt)
	return err
}

func (tl *TimescaleLogger) LogMsBlockingPairs(ctx context.Context, rows []PgBlockingPairRow) error {
	if len(rows) == 0 {
		return nil
	}
	q := `INSERT INTO sqlserver_blocking_pairs (capture_timestamp, server_id, blocked_spid, blocking_spid) VALUES ($1,$2,$3,$4)`
	b := &pgx.Batch{}
	for _, r := range rows {
		b.Queue(q, r.CollectedAt, r.ServerID, r.BlockedPID, r.BlockingPID)
	}
	br := tl.pool.SendBatch(ctx, b)
	defer br.Close()
	for range rows {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return nil
}

func (tl *TimescaleLogger) GetSQLServerAGHealth(ctx context.Context, serverID uuid.UUID) ([]map[string]interface{}, error) {
	// Reads from the canonical monitor schema tables that LogAGHealthFromMap now writes to.
	// Joins per-database state with per-replica state for the same capture timestamp.
	q := `
		SELECT DISTINCT ON (d.replica_server_name, d.database_name)
			d.capture_timestamp,
			d.replica_server_name,
			d.database_name,
			d.ag_name,
			COALESCE(r.role_desc, 'UNKNOWN')                AS role_desc,
			COALESCE(d.synchronization_state_desc, 'UNKNOWN') AS sync_state_desc,
			COALESCE(r.connected_state_desc, 'UNKNOWN')      AS connected_state,
			'UNKNOWN'                                        AS operational_state,
			d.log_send_queue_kb,
			COALESCE(r.log_send_rate_kbps, 0)               AS log_send_rate_kbps,
			d.redo_queue_kb,
			COALESCE(r.redo_rate_kbps, 0)                   AS redo_rate_kbps,
			COALESCE(r.secondary_lag_seconds, 0)             AS secondary_lag_seconds
		FROM monitor.sqlserver_ha_database_state d
		LEFT JOIN monitor.sqlserver_ha_replica_state r
			ON  r.server_id           = d.server_id
			AND r.ag_name             = d.ag_name
			AND r.replica_server_name = d.replica_server_name
			AND r.capture_timestamp   = d.capture_timestamp
		WHERE d.server_id = $1
		ORDER BY d.replica_server_name, d.database_name, d.capture_timestamp DESC
	`
	rows, err := tl.pool.Query(ctx, q, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var ts time.Time
		var replica, db, ag, role, sync, conn, oper sql.NullString
		var sendQ, sendR, redoQ, redoR, lag sql.NullInt64
		if err := rows.Scan(&ts, &replica, &db, &ag, &role, &sync, &conn, &oper, &sendQ, &sendR, &redoQ, &redoR, &lag); err != nil {
			continue
		}
		results = append(results, map[string]interface{}{
			"capture_timestamp":          ts,
			"replica_server_name":        replica.String,
			"database_name":              db.String,
			"ag_name":                    ag.String,
			"role_desc":                  role.String,
			"synchronization_state_desc": sync.String,
			"connected_state":            conn.String,
			"operational_state":          oper.String,
			"log_send_queue_size":        sendQ.Int64,
			"log_send_rate":              sendR.Int64,
			"redo_queue_size":            redoQ.Int64,
			"redo_rate":                  redoR.Int64,
			"secondary_lag_seconds":      lag.Int64,
		})
	}
	return results, nil
}

func (tl *TimescaleLogger) GetSQLServerReplicationStatus(ctx context.Context, serverID uuid.UUID) ([]map[string]interface{}, error) {
	// For now, return empty as it might be in a different table or not yet implemented in collectors
	return []map[string]interface{}{}, nil
}

