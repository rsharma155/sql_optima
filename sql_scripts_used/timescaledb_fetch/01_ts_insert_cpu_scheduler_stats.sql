-- Metric: ts_insert_cpu_scheduler_stats
-- Source: Collector Service
-- Target Table: sqlserver_cpu_scheduler_stats (TimescaleDB)
-- Description: Batch inserts CPU scheduler statistics with pressure warnings

INSERT INTO sqlserver_cpu_scheduler_stats (
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
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28);
