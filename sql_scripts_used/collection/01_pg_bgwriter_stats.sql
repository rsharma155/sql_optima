-- Metric: pg_bgwriter_stats
-- Source: backend/internal/repository/pg_stats.go:1377
-- Target Table: postgres_bgwriter_stats (TimescaleDB)
-- Description: Retrieves full background writer and checkpointer statistics from pg_stat_bgwriter

SELECT 
    checkpoints_timed,
    checkpoints_req,
    checkpoint_write_time,
    checkpoint_sync_time,
    buffers_checkpoint,
    buffers_clean,
    maxwritten_clean,
    buffers_backend,
    buffers_alloc
FROM pg_stat_bgwriter;
