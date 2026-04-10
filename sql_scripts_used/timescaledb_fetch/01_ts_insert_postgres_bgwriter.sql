-- Metric: ts_insert_postgres_bgwriter
-- Source: backend/internal/storage/hot/ts_logger.go:1060
-- Target Table: postgres_bgwriter_stats
-- Description: Inserts PostgreSQL BGWriter and checkpointer statistics

INSERT INTO postgres_bgwriter_stats (
    capture_timestamp, server_instance_name,
    checkpoints_timed, checkpoints_req, checkpoint_write_time, checkpoint_sync_time,
    buffers_checkpoint, buffers_clean, maxwritten_clean, buffers_backend, buffers_alloc
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11);
