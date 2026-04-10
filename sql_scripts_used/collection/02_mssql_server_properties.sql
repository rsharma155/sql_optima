-- Metric: mssql_server_properties
-- Source: Collector Service
-- Target Table: sqlserver_server_properties (TimescaleDB)
-- Description: Collects CPU hardware properties - runs daily due to infrequent changes

SELECT 
    GETDATE() AS capture_timestamp,
    @server_instance_name AS server_instance_name,
    osi.cpu_count,
    osi.hyperthread_ratio,
    osi.socket_count,
    osi.cores_per_socket,
    osi.physical_memory_kb / 1024.0 / 1024.0 AS physical_memory_gb,
    osi.virtual_memory_kb / 1024.0 / 1024.0 AS virtual_memory_gb,
    (SELECT cpu_type FROM sys.dm_os_sys_info) AS cpu_type,
    CASE WHEN osi.hyperthread_ratio < osi.cpu_count THEN 1 ELSE 0 END AS hyperthread_enabled,
    (SELECT COUNT(*) FROM sys.dm_os_nodes WHERE node_id < 64) AS numa_nodes,
    osi.max_workers_count,
    CONVERT(VARCHAR(64), HASHBYTES('SHA2_256', 
        CONCAT(osi.cpu_count, osi.hyperthread_ratio, osi.socket_count, osi.cores_per_socket, 
               osi.physical_memory_kb, (SELECT COUNT(*) FROM sys.dm_os_nodes WHERE node_id < 64))), 2) AS properties_hash
FROM sys.dm_os_sys_info osi
OPTION (RECOMPILE);
