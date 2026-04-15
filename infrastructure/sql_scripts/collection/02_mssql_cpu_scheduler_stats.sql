-- Metric: mssql_cpu_scheduler_stats
-- Source: Collector Service
-- Target Table: sqlserver_cpu_scheduler_stats (TimescaleDB)
-- Description: Collects CPU scheduler and workload group metrics with pressure warnings

SELECT 
    GETDATE() AS capture_timestamp,
    @server_instance_name AS server_instance_name,
    osi.max_workers_count,
    osi.scheduler_count,
    osi.cpu_count,
    COALESCE(sched.runnable_tasks_count, 0) AS total_runnable_tasks_count,
    COALESCE(sched.work_queue_count, 0) AS total_work_queue_count,
    COALESCE(sched.current_workers_count, 0) AS total_current_workers_count,
    0.0 AS avg_runnable_tasks_count,
    wg.active_request_count AS total_active_request_count,
    wg.queued_request_count AS total_queued_request_count,
    wg.blocked_task_count AS total_blocked_task_count,
    wg.active_parallel_thread_count AS total_active_parallel_thread_count,
    wg.runnable_request_count AS runnable_request_count,
    wg.total_request_count AS total_request_count,
    CASE WHEN wg.total_request_count > 0 
         THEN 100.0 * wg.runnable_request_count / wg.total_request_count 
         ELSE 0 END AS runnable_percent,
    CASE WHEN COALESCE(sched.current_workers_count, 0) >= osi.max_workers_count * 0.90 THEN 1 ELSE 0 END AS worker_thread_exhaustion_warning,
    CASE WHEN COALESCE(sched.runnable_tasks_count, 0) >= osi.cpu_count THEN 1 ELSE 0 END AS runnable_tasks_warning,
    CASE WHEN COALESCE(wg.blocked_task_count, 0) >= 10 THEN 1 ELSE 0 END AS blocked_tasks_warning,
    CASE WHEN COALESCE(wg.queued_request_count, 0) >= osi.cpu_count THEN 1 ELSE 0 END AS queued_requests_warning,
    osm.total_physical_memory_kb AS total_physical_memory_kb,
    osm.available_physical_memory_kb AS available_physical_memory_kb,
    osm.system_memory_state_desc AS system_memory_state_desc,
    CASE WHEN osm.available_physical_memory_kb < osm.total_physical_memory_kb * 0.10 THEN 1 ELSE 0 END AS physical_memory_pressure_warning,
    (SELECT COUNT(*) FROM sys.dm_os_nodes WHERE node_id < 64) AS total_node_count,
    (SELECT COUNT(*) FROM sys.dm_os_nodes WHERE node_id < 64 AND node_state_desc LIKE '%ONLINE%') AS nodes_online_count,
    (SELECT COUNT(*) FROM sys.dm_os_schedulers WHERE is_online = 0) AS offline_cpu_count,
    CASE WHEN EXISTS (SELECT 1 FROM sys.dm_os_schedulers WHERE is_online = 0) THEN 1 ELSE 0 END AS offline_cpu_warning
FROM sys.dm_os_sys_info osi
CROSS JOIN (
    SELECT 
        SUM(runnable_tasks_count) AS runnable_tasks_count,
        SUM(work_queue_count) AS work_queue_count,
        SUM(current_workers_count) AS current_workers_count
    FROM sys.dm_os_schedulers
    WHERE is_online = 1
) sched
CROSS JOIN (
    SELECT 
        SUM(active_request_count) AS active_request_count,
        SUM(queued_request_count) AS queued_request_count,
        SUM(blocked_task_count) AS blocked_task_count,
        SUM(active_parallel_thread_count) AS active_parallel_thread_count,
        SUM(runnable_request_count) AS runnable_request_count,
        SUM(total_request_count) AS total_request_count
    FROM sys.dm_resource_governor_workload_groups
) wg
CROSS JOIN sys.dm_os_sys_memory osm
OPTION (RECOMPILE);
