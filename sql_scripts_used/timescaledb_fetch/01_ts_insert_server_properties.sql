-- Metric: ts_insert_server_properties
-- Source: Collector Service
-- Target Table: sqlserver_server_properties (TimescaleDB)
-- Description: Batch inserts server hardware properties with hash for deduplication

INSERT INTO sqlserver_server_properties (
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
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13);
