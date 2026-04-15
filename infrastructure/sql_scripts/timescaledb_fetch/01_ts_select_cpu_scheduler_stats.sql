-- Metric: ts_select_cpu_scheduler_stats
-- Source: Backend API
-- Target: Dashboard retrieval
-- Description: Fetches CPU scheduler stats from TimescaleDB for a time range

SELECT 
    capture_timestamp,
    server_instance_name,
    max_workers_count,
    scheduler_count,
    cpu_count,
    total_runnable_tasks_count,
    total_work_queue_count,
    total_current_workers_count,
    avg_runnable_tasks_count,
    total_active_request_count,
    total_queued_request_count,
    total_blocked_task_count,
    total_active_parallel_thread_count,
    runnable_request_count,
    total_request_count,
    runnable_percent,
    worker_thread_exhaustion_warning,
    runnable_tasks_warning,
    blocked_tasks_warning,
    queued_requests_warning,
    total_physical_memory_kb,
    available_physical_memory_kb,
    system_memory_state_desc,
    physical_memory_pressure_warning,
    total_node_count,
    nodes_online_count,
    offline_cpu_count,
    offline_cpu_warning
FROM sqlserver_cpu_scheduler_stats
WHERE server_instance_name = $1
  AND capture_timestamp >= $2
  AND capture_timestamp <= $3
ORDER BY capture_timestamp DESC
LIMIT $4;
