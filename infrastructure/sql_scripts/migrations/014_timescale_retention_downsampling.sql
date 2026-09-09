-- SQL Optima — optional Timescale retention + downsampling guidance helpers
-- Apply on upgrade when you want explicit long-term retention floors and hourly CAGGs.
-- Idempotent. Does not delete data shorter than existing policies.

-- Core high-volume tables: keep 90 days raw (adjust after cold storage is validated).
SELECT add_retention_policy('sqlserver_cpu_history', INTERVAL '90 days', if_not_exists => TRUE);
SELECT add_retention_policy('sqlserver_wait_history', INTERVAL '90 days', if_not_exists => TRUE);
SELECT add_retention_policy('sqlserver_connection_history', INTERVAL '90 days', if_not_exists => TRUE);
SELECT add_retention_policy('sqlserver_memory_history', INTERVAL '90 days', if_not_exists => TRUE);

-- Example continuous aggregate for CPU (1-hour buckets) for long-range charts when cold is off.
-- Safe to re-run: CREATE MATERIALIZED VIEW IF NOT EXISTS is not supported for CAGGs in all versions;
-- use DO block to skip if already present.

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM timescaledb_information.continuous_aggregates
    WHERE view_name = 'sqlserver_cpu_history_1h'
  ) THEN
    EXECUTE $cagg$
      CREATE MATERIALIZED VIEW sqlserver_cpu_history_1h
      WITH (timescaledb.continuous) AS
      SELECT time_bucket(INTERVAL '1 hour', capture_timestamp) AS bucket,
             server_id,
             AVG(sql_process) AS sql_process,
             AVG(system_idle) AS system_idle,
             AVG(other_process) AS other_process
      FROM sqlserver_cpu_history
      GROUP BY bucket, server_id
      WITH NO DATA
    $cagg$;
    PERFORM add_continuous_aggregate_policy(
      'sqlserver_cpu_history_1h',
      start_offset => INTERVAL '3 days',
      end_offset   => INTERVAL '1 hour',
      schedule_interval => INTERVAL '1 hour',
      if_not_exists => TRUE
    );
    PERFORM add_retention_policy('sqlserver_cpu_history_1h', INTERVAL '400 days', if_not_exists => TRUE);
    EXECUTE 'COMMENT ON MATERIALIZED VIEW sqlserver_cpu_history_1h IS ''Optional hourly downsampling of sqlserver_cpu_history for longer lookbacks without cold storage.''';
  END IF;
END $$;
