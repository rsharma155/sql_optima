-- Metric: mssql_ag_health_no_lag
-- Source: backend/internal/repository/mssql_stats.go:315
-- Target Table: sqlserver_ag_health (TimescaleDB)
-- Description: Fetches AG health metrics without secondary_lag_seconds (pre-2016 SP1)

SELECT 
    ag.name AS ag_name,
    ar.replica_server_name,
    COALESCE(DB_NAME(dbs.database_id), 'N/A') AS database_name,
    rs.role_desc AS replica_role,
    COALESCE(drs.synchronization_state, 0) AS synchronization_state,
    COALESCE(drs.synchronization_state_desc, 'UNKNOWN') AS synchronization_state_desc,
    CASE WHEN rs.role_desc = 'PRIMARY' THEN 1 ELSE 0 END AS is_primary_replica,
    ISNULL(drs.log_send_queue_size, 0) / 1024 AS log_send_queue_kb,
    ISNULL(drs.undo_queue_size, 0) / 1024 AS redo_queue_kb,
    ISNULL(drs.log_send_rate, 0) / 1024 AS log_send_rate_kb,
    ISNULL(drs.undo_rate, 0) / 1024 AS redo_rate_kb,
    drs.last_sent_time,
    drs.last_received_time,
    drs.last_hardened_time,
    drs.last_redone_time,
    CAST(0 AS BIGINT) AS secondary_lag_seconds
FROM sys.availability_groups ag
INNER JOIN sys.availability_replicas ar ON ag.group_id = ar.group_id
INNER JOIN sys.dm_hadr_availability_group_states rs ON ag.group_id = rs.group_id
INNER JOIN sys.dm_hadr_availability_replica_states drs ON ar.replica_id = drs.replica_id
LEFT JOIN sys.dm_hadr_availability_database_states dbs ON ar.replica_id = dbs.replica_id AND dbs.database_id IS NOT NULL
ORDER BY ag.name, ar.replica_server_name;
