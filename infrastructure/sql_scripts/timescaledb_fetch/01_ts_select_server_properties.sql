-- Metric: ts_select_server_properties
-- Source: Backend API
-- Target: Dashboard retrieval
-- Description: Fetches server properties from TimescaleDB

SELECT 
    capture_timestamp,
    server_instance_name,
    cpu_count,
    hyperthread_ratio,
    socket_count,
    cores_per_socket,
    physical_memory_gb,
    virtual_memory_gb,
    cpu_type,
    hyperthread_enabled,
    numa_nodes,
    max_workers_count,
    properties_hash
FROM sqlserver_server_properties
WHERE server_instance_name = $1
ORDER BY capture_timestamp DESC
LIMIT 1;
