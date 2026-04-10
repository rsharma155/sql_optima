package hot

import (
	"context"
	"fmt"
	"hash/fnv"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
)

func (tl *TimescaleLogger) GetSQLServerMetrics(ctx context.Context, instanceName string, limit int) ([]SQLServerMetricRow, error) {
	if limit <= 0 {
		limit = 100
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	query := `
		SELECT capture_timestamp, server_instance_name, avg_cpu_load, memory_usage,
		       active_users, total_locks, deadlocks, data_disk_mb, log_disk_mb, free_disk_mb,
		       COALESCE(cpu_wait, 0), COALESCE(disk_wait, 0), COALESCE(lock_wait, 0), COALESCE(network_wait, 0)
		FROM sqlserver_metrics
		WHERE server_instance_name = $1
		ORDER BY capture_timestamp DESC
		LIMIT $2
	`

	rows, err := tl.pool.Query(ctx, query, instanceName, limit)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			log.Printf("[TSLogger] GetSQLServerMetrics timed out after 5s")
			return []SQLServerMetricRow{}, nil
		}
		log.Printf("[TSLogger] Failed to query SQLServer metrics: %v", err)
		return nil, err
	}
	defer rows.Close()

	var results []SQLServerMetricRow
	for rows.Next() {
		var r SQLServerMetricRow
		if err := rows.Scan(&r.CaptureTimestamp, &r.ServerName, &r.AvgCpuLoad, &r.MemoryUsage,
			&r.ActiveUsers, &r.TotalLocks, &r.Deadlocks, &r.DataDiskMB, &r.LogDiskMB, &r.FreeDiskMB,
			&r.CpuWait, &r.DiskWait, &r.LockWait, &r.NetworkWait); err != nil {
			log.Printf("[TSLogger] Failed to scan row: %v", err)
			continue
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

func (tl *TimescaleLogger) GetSQLServerMetricsTimeRange(ctx context.Context, instanceName string, start, end time.Time) ([]SQLServerMetricRow, error) {
	query := `
		SELECT capture_timestamp, server_instance_name, avg_cpu_load, memory_usage,
		       active_users, total_locks, deadlocks, data_disk_mb, log_disk_mb, free_disk_mb
		FROM sqlserver_metrics
		WHERE server_instance_name = $1
		  AND capture_timestamp >= $2
		  AND capture_timestamp <= $3
		ORDER BY capture_timestamp ASC
	`

	rows, err := tl.pool.Query(ctx, query, instanceName, start, end)
	if err != nil {
		log.Printf("[TSLogger] Failed to query SQLServer metrics: %v", err)
		return nil, err
	}
	defer rows.Close()

	var results []SQLServerMetricRow
	for rows.Next() {
		var r SQLServerMetricRow
		if err := rows.Scan(&r.CaptureTimestamp, &r.ServerName, &r.AvgCpuLoad, &r.MemoryUsage,
			&r.ActiveUsers, &r.TotalLocks, &r.Deadlocks, &r.DataDiskMB, &r.LogDiskMB, &r.FreeDiskMB); err != nil {
			log.Printf("[TSLogger] Failed to scan row: %v", err)
			continue
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

func (tl *TimescaleLogger) LogSQLServerMetrics(ctx context.Context, instanceName string, data map[string]interface{}) error {
	query := `
		INSERT INTO sqlserver_metrics (
			capture_timestamp, server_instance_name, avg_cpu_load, memory_usage,
			active_users, total_locks, deadlocks, data_disk_mb, log_disk_mb, free_disk_mb
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (server_instance_name, capture_timestamp) DO UPDATE SET
			avg_cpu_load = EXCLUDED.avg_cpu_load,
			memory_usage = EXCLUDED.memory_usage,
			active_users = EXCLUDED.active_users,
			total_locks = EXCLUDED.total_locks,
			deadlocks = EXCLUDED.deadlocks,
			data_disk_mb = EXCLUDED.data_disk_mb,
			log_disk_mb = EXCLUDED.log_disk_mb,
			free_disk_mb = EXCLUDED.free_disk_mb
	`
	now := time.Now().UTC()

	_, err := tl.pool.Exec(ctx, query,
		now, instanceName,
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

func (tl *TimescaleLogger) LogSQLServerCPUHistory(ctx context.Context, instanceName string, ticks []map[string]interface{}) error {
	if len(ticks) == 0 {
		return nil
	}
	tick := ticks[len(ticks)-1]
	query := `INSERT INTO sqlserver_cpu_history (capture_timestamp, server_instance_name, sql_process, system_idle, other_process) VALUES ($1, $2, $3, $4, $5)`
	now := time.Now().UTC()
	_, err := tl.pool.Exec(ctx, query, now, instanceName, tick["sql_process"], tick["system_idle"], tick["other_process"])
	return err
}

func (tl *TimescaleLogger) LogSQLServerMemoryHistory(ctx context.Context, instanceName string, ple float64) error {
	tl.mu.Lock()
	defer tl.mu.Unlock()

	if ple <= 0 || tl.prevMemoryPLE <= 0 {
		tl.prevMemoryPLE = ple
		return nil
	}

	query := `INSERT INTO sqlserver_memory_history (capture_timestamp, server_instance_name, page_life_expectancy_seconds) VALUES ($1, $2, $3)`
	now := time.Now().UTC()
	_, err := tl.pool.Exec(ctx, query, now, instanceName, ple)
	tl.prevMemoryPLE = ple
	return err
}

func (tl *TimescaleLogger) LogSQLServerWaitHistory(ctx context.Context, instanceName string, waits []map[string]interface{}) error {
	if len(waits) == 0 {
		return nil
	}

	wait := waits[len(waits)-1]
	query := `INSERT INTO sqlserver_wait_history (capture_timestamp, server_instance_name, disk_read, blocking, parallelism, other) VALUES ($1, $2, $3, $4, $5, $6)`
	now := time.Now().UTC()
	_, err := tl.pool.Exec(ctx, query, now, instanceName,
		getFloat64(wait, "disk_read"),
		getFloat64(wait, "blocking"),
		getFloat64(wait, "parallelism"),
		getFloat64(wait, "other"),
	)
	return err
}

func (tl *TimescaleLogger) LogSQLServerConnectionHistory(ctx context.Context, instanceName string, conns map[string]map[string]interface{}) error {
	for dbName, conn := range conns {
		query := `INSERT INTO sqlserver_connection_history (capture_timestamp, server_instance_name, database_name, login_name, active_connections, active_requests)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (server_instance_name, database_name, capture_timestamp) DO UPDATE SET
				login_name = EXCLUDED.login_name,
				active_connections = EXCLUDED.active_connections,
				active_requests = EXCLUDED.active_requests`
		now := time.Now().UTC()
		_, err := tl.pool.Exec(ctx, query, now, instanceName, dbName,
			getStr(conn, "login_name"),
			getInt(conn, "active_connections"),
			getInt(conn, "active_requests"),
		)
		if err != nil {
			log.Printf("[TSLogger] Failed to log connection history: %v", err)
		}
	}
	return nil
}

func (tl *TimescaleLogger) LogSQLServerLockHistory(ctx context.Context, instanceName string, locks map[string]map[string]interface{}) error {
	for dbName, lock := range locks {
		query := `INSERT INTO sqlserver_lock_history (capture_timestamp, server_instance_name, database_name, total_locks, deadlocks)
			VALUES ($1, $2, $3, $4, $5)`
		now := time.Now().UTC()
		_, err := tl.pool.Exec(ctx, query, now, instanceName, dbName, getInt(lock, "total_locks"), getInt(lock, "deadlocks"))
		if err != nil {
			log.Printf("[TSLogger] Failed to log lock history: %v", err)
		}
	}
	return nil
}

func (tl *TimescaleLogger) LogSQLServerDiskHistory(ctx context.Context, instanceName string, disk map[string]map[string]interface{}) error {
	for dbName, d := range disk {
		query := `INSERT INTO sqlserver_disk_history (capture_timestamp, server_instance_name, database_name, data_mb, log_mb, free_mb)
			VALUES ($1, $2, $3, $4, $5, $6)`
		now := time.Now().UTC()
		_, err := tl.pool.Exec(ctx, query, now, instanceName, dbName, getFloat64(d, "data_mb"), getFloat64(d, "log_mb"), getFloat64(d, "free_mb"))
		if err != nil {
			log.Printf("[TSLogger] Failed to log disk history: %v", err)
		}
	}
	return nil
}

// cpuSchedulerSnapshotHash fingerprints the collected DMV snapshot (excluding capture time).
// Used to skip inserts when the engine reports identical scheduler state between polls.
func cpuSchedulerSnapshotHash(m map[string]interface{}) uint64 {
	h := fnv.New64a()
	_, _ = fmt.Fprintf(h, "%d|%d|%d|%d|%d|%d|%g|%d|%d|%d|%d|%d|%d|%g|%v|%v|%v|%v|%d|%d|%s|%v|%d|%d|%d|%v",
		getInt(m, "max_workers_count"),
		getInt(m, "scheduler_count"),
		getInt(m, "cpu_count"),
		getInt(m, "total_runnable_tasks_count"),
		getInt(m, "total_work_queue_count"),
		getInt(m, "total_current_workers_count"),
		getFloat64(m, "avg_runnable_tasks_count"),
		getInt(m, "total_active_request_count"),
		getInt(m, "total_queued_request_count"),
		getInt(m, "total_blocked_task_count"),
		getInt(m, "total_active_parallel_thread_count"),
		getInt(m, "runnable_request_count"),
		getInt(m, "total_request_count"),
		getFloat64(m, "runnable_percent"),
		getBool(m, "worker_thread_exhaustion_warning"),
		getBool(m, "runnable_tasks_warning"),
		getBool(m, "blocked_tasks_warning"),
		getBool(m, "queued_requests_warning"),
		getInt(m, "total_physical_memory_kb"),
		getInt(m, "available_physical_memory_kb"),
		getStr(m, "system_memory_state_desc"),
		getBool(m, "physical_memory_pressure_warning"),
		getInt(m, "total_node_count"),
		getInt(m, "nodes_online_count"),
		getInt(m, "offline_cpu_count"),
		getBool(m, "offline_cpu_warning"),
	)
	return h.Sum64()
}

func (tl *TimescaleLogger) LogCPUSchedulerStats(ctx context.Context, instanceName string, statsMap map[string]interface{}) error {
	sig := cpuSchedulerSnapshotHash(statsMap)
	tl.mu.Lock()
	if prev, ok := tl.prevSchedulerStats[instanceName]; ok && prev == sig {
		tl.mu.Unlock()
		return nil
	}
	tl.prevSchedulerStats[instanceName] = sig
	tl.mu.Unlock()

	query := `INSERT INTO sqlserver_cpu_scheduler_stats (
		capture_timestamp, server_instance_name,
		max_workers_count, scheduler_count, cpu_count,
		total_runnable_tasks_count, total_work_queue_count, total_current_workers_count,
		avg_runnable_tasks_count, total_active_request_count, total_queued_request_count,
		total_blocked_task_count, total_active_parallel_thread_count,
		runnable_request_count, total_request_count, runnable_percent,
		worker_thread_exhaustion_warning, runnable_tasks_warning,
		blocked_tasks_warning, queued_requests_warning,
		total_physical_memory_kb, available_physical_memory_kb,
		system_memory_state_desc, physical_memory_pressure_warning,
		total_node_count, nodes_online_count, offline_cpu_count, offline_cpu_warning
	) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28)`

	now := time.Now().UTC()
	_, err := tl.pool.Exec(ctx, query, now, instanceName,
		getInt(statsMap, "max_workers_count"),
		getInt(statsMap, "scheduler_count"),
		getInt(statsMap, "cpu_count"),
		getInt(statsMap, "total_runnable_tasks_count"),
		getInt(statsMap, "total_work_queue_count"),
		getInt(statsMap, "total_current_workers_count"),
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

func (tl *TimescaleLogger) GetCPUSchedulerStats(ctx context.Context, instanceName string, limit int) ([]CPUSchedulerStatsRow, error) {
	if limit <= 0 {
		limit = 100
	}

	query := `
		SELECT capture_timestamp, server_instance_name,
		       max_workers_count, scheduler_count, cpu_count,
		       total_runnable_tasks_count, total_work_queue_count, total_current_workers_count,
		       avg_runnable_tasks_count, total_active_request_count, total_queued_request_count,
		       total_blocked_task_count,
		       runnable_percent,
		       worker_thread_exhaustion_warning, runnable_tasks_warning, blocked_tasks_warning, queued_requests_warning,
		       total_physical_memory_kb, available_physical_memory_kb, system_memory_state_desc, physical_memory_pressure_warning,
		       total_node_count, nodes_online_count, offline_cpu_count, offline_cpu_warning
		FROM sqlserver_cpu_scheduler_stats
		WHERE server_instance_name = $1
		ORDER BY capture_timestamp DESC
		LIMIT $2
	`

	rows, err := tl.pool.Query(ctx, query, instanceName, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []CPUSchedulerStatsRow
	for rows.Next() {
		var r CPUSchedulerStatsRow
		if err := rows.Scan(&r.CaptureTimestamp, &r.ServerInstanceName,
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

func (tl *TimescaleLogger) LogServerProperties(ctx context.Context, instanceName string, propsMap map[string]interface{}) error {
	query := `INSERT INTO sqlserver_server_properties (
		capture_timestamp, server_instance_name,
		cpu_count, hyperthread_ratio, socket_count, cores_per_socket,
		physical_memory_gb, virtual_memory_gb, cpu_type,
		hyperthread_enabled, numa_nodes, max_workers_count, properties_hash
	) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`

	now := time.Now().UTC()
	_, err := tl.pool.Exec(ctx, query, now, instanceName,
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

func (tl *TimescaleLogger) GetServerProperties(ctx context.Context, instanceName string) (*ServerPropertiesRow, error) {
	query := `
		SELECT capture_timestamp, server_instance_name,
		       cpu_count, hyperthread_ratio, socket_count, cores_per_socket,
		       physical_memory_gb, virtual_memory_gb, cpu_type,
		       hyperthread_enabled, numa_nodes, max_workers_count, properties_hash
		FROM sqlserver_server_properties
		WHERE server_instance_name = $1
		ORDER BY capture_timestamp DESC
		LIMIT 1
	`

	var r ServerPropertiesRow
	err := tl.pool.QueryRow(ctx, query, instanceName).Scan(
		&r.CaptureTimestamp, &r.ServerInstanceName,
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

func getFloat64(m map[string]interface{}, key string) float64 {
	if v, ok := m[key]; ok {
		switch val := v.(type) {
		case float64:
			return val
		case int:
			return float64(val)
		case int64:
			return float64(val)
		}
	}
	return 0
}

func getInt(m map[string]interface{}, key string) int {
	if v, ok := m[key]; ok {
		switch val := v.(type) {
		case int:
			return val
		case int64:
			return int(val)
		case float64:
			return int(val)
		}
	}
	return 0
}

func getStr(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func getBool(m map[string]interface{}, key string) bool {
	if v, ok := m[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}
