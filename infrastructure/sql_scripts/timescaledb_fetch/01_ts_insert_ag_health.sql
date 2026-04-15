-- Metric: ts_insert_ag_health
-- Source: backend/internal/storage/hot/ts_logger.go:854
-- Target Table: sqlserver_ag_health
-- Description: Batch inserts AlwaysOn Availability Group health metrics

INSERT INTO sqlserver_ag_health (
    capture_timestamp, server_instance_name, ag_name, replica_server_name, database_name,
    replica_role, synchronization_state, synchronization_state_desc, is_primary_replica,
    log_send_queue_kb, redo_queue_kb, log_send_rate_kb, redo_rate_kb,
    last_sent_time, last_received_time, last_hardened_time, last_redone_time, secondary_lag_seconds
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18);
