-- Metric: ts_create_hypertables
-- Source: backend/internal/storage/hot/storage.go:288
-- Target Table: All hypertables
-- Description: Creates TimescaleDB hypertables on all time-series tables

SELECT create_hypertable('sqlserver_ag_health', 'capture_timestamp', chunk_time_interval => INTERVAL '1 day', if_not_exists => TRUE);
SELECT create_hypertable('sqlserver_database_throughput', 'capture_timestamp', chunk_time_interval => INTERVAL '1 day', if_not_exists => TRUE);
SELECT create_hypertable('sqlserver_query_store_stats', 'capture_timestamp', chunk_time_interval => INTERVAL '1 day', if_not_exists => TRUE);
SELECT create_hypertable('sqlserver_top_queries', 'capture_timestamp', chunk_time_interval => INTERVAL '1 day', if_not_exists => TRUE);
SELECT create_hypertable('postgres_bgwriter_stats', 'capture_timestamp', chunk_time_interval => INTERVAL '1 day', if_not_exists => TRUE);
SELECT create_hypertable('postgres_archiver_stats', 'capture_timestamp', chunk_time_interval => INTERVAL '1 day', if_not_exists => TRUE);
SELECT create_hypertable('postgres_throughput_metrics', 'capture_timestamp', chunk_time_interval => INTERVAL '1 day', if_not_exists => TRUE);
SELECT create_hypertable('postgres_connection_stats', 'capture_timestamp', chunk_time_interval => INTERVAL '1 day', if_not_exists => TRUE);
SELECT create_hypertable('postgres_replication_stats', 'capture_timestamp', chunk_time_interval => INTERVAL '1 day', if_not_exists => TRUE);
