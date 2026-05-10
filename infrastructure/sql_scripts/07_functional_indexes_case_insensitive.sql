-- SQL Optima — https://github.com/rsharma155/sql_optima
--
-- Purpose: Optimized functional indexes for case-insensitive instance lookups.
--          Replaces standard B-tree indexes with UPPER() expression indexes 
--          to match the application's query patterns.
--
-- Author: Ravi Sharma
-- Copyright (c) 2026 Ravi Sharma
-- SPDX-License-Identifier: MIT

-- 1. PostgreSQL Memory Intelligence (monitor schema)
DROP INDEX IF EXISTS monitor.idx_host_mem_instance_ts;
CREATE INDEX idx_host_mem_instance_ts ON monitor.host_memory_samples (UPPER(server_instance_name), ts DESC);

DROP INDEX IF EXISTS monitor.idx_pg_mem_instance_ts;
CREATE INDEX idx_pg_mem_instance_ts ON monitor.pg_memory_samples (UPPER(server_instance_name), ts DESC);

DROP INDEX IF EXISTS monitor.idx_pg_comp_instance_ts;
CREATE INDEX idx_pg_comp_instance_ts ON monitor.pg_memory_components (UPPER(server_instance_name), ts DESC);

DROP INDEX IF EXISTS monitor.idx_pg_der_instance_ts;
CREATE UNIQUE INDEX idx_pg_der_instance_ts ON monitor.pg_memory_derived (UPPER(server_instance_name), ts DESC);

-- 2. PostgreSQL Core Stats
DROP INDEX IF EXISTS idx_postgres_tp_server_db;
CREATE INDEX idx_postgres_tp_server_db ON postgres_throughput_metrics (UPPER(server_instance_name), database_name, capture_timestamp DESC);

DROP INDEX IF EXISTS idx_postgres_conn_server;
CREATE INDEX idx_postgres_conn_server ON postgres_connection_stats (UPPER(server_instance_name), capture_timestamp DESC);

DROP INDEX IF EXISTS idx_postgres_repl_server;
CREATE INDEX idx_postgres_repl_server ON postgres_replication_stats (UPPER(server_instance_name), capture_timestamp DESC);

DROP INDEX IF EXISTS idx_postgres_sys_server;
CREATE INDEX idx_postgres_sys_server ON postgres_system_stats (UPPER(server_instance_name), capture_timestamp DESC);

DROP INDEX IF EXISTS idx_postgres_qrystat_server;
CREATE INDEX idx_postgres_qrystat_server ON postgres_query_stats (UPPER(server_instance_name), capture_timestamp DESC);

DROP INDEX IF EXISTS idx_pgss_qdim_db_user;
CREATE INDEX idx_pgss_qdim_db_user ON pgss_query_dim (UPPER(server_instance_name), db_name, username);

DROP INDEX IF EXISTS idx_pgss_delta_1m_server;
CREATE INDEX idx_pgss_delta_1m_server ON pgss_delta_1m (UPPER(server_instance_name), capture_timestamp DESC);

DROP INDEX IF EXISTS idx_postgres_bgw_server_time;
CREATE INDEX idx_postgres_bgw_server_time ON postgres_bgwriter_stats (UPPER(server_instance_name), capture_timestamp DESC);

DROP INDEX IF EXISTS idx_postgres_arch_server_time;
CREATE INDEX idx_postgres_arch_server_time ON postgres_archiver_stats (UPPER(server_instance_name), capture_timestamp DESC);

DROP INDEX IF EXISTS idx_pg_cc_server_time;
CREATE INDEX idx_pg_cc_server_time ON postgres_control_center_stats (UPPER(server_instance_name), capture_timestamp DESC);

DROP INDEX IF EXISTS idx_pg_repl_lag_detail;
CREATE INDEX idx_pg_repl_lag_detail ON postgres_replication_lag_detail (UPPER(server_instance_name), replica_name, capture_timestamp DESC);

DROP INDEX IF EXISTS idx_pg_repl_slot_server_time;
CREATE INDEX idx_pg_repl_slot_server_time ON postgres_replication_slot_stats (UPPER(server_instance_name), capture_timestamp DESC);

DROP INDEX IF EXISTS idx_pg_disk_server_time;
CREATE INDEX idx_pg_disk_server_time ON postgres_disk_stats (UPPER(server_instance_name), capture_timestamp DESC);

DROP INDEX IF EXISTS idx_pg_backup_runs_server_time;
CREATE INDEX idx_pg_backup_runs_server_time ON postgres_backup_runs (UPPER(server_instance_name), capture_timestamp DESC);

DROP INDEX IF EXISTS idx_pg_log_events_server_time;
CREATE INDEX idx_pg_log_events_server_time ON postgres_log_events (UPPER(server_instance_name), capture_timestamp DESC);

DROP INDEX IF EXISTS idx_pg_vac_prog_server_time;
CREATE INDEX idx_pg_vac_prog_server_time ON postgres_vacuum_progress (UPPER(server_instance_name), capture_timestamp DESC);

DROP INDEX IF EXISTS idx_pg_tblmaint_server_time;
CREATE INDEX idx_pg_tblmaint_server_time ON postgres_table_maintenance_stats (UPPER(server_instance_name), capture_timestamp DESC);

DROP INDEX IF EXISTS idx_pg_sess_state_server_time;
CREATE INDEX idx_pg_sess_state_server_time ON postgres_session_state_counts (UPPER(server_instance_name), capture_timestamp DESC);

DROP INDEX IF EXISTS idx_pg_pooler_server_time;
CREATE INDEX idx_pg_pooler_server_time ON postgres_pooler_stats (UPPER(server_instance_name), capture_timestamp DESC);

DROP INDEX IF EXISTS idx_pg_deadlocks_server_time;
CREATE INDEX idx_pg_deadlocks_server_time ON postgres_deadlock_stats (UPPER(server_instance_name), capture_timestamp DESC);

-- 3. SQL Server Stats
DROP INDEX IF EXISTS idx_sqlserver_metrics_server;
CREATE INDEX idx_sqlserver_metrics_server ON sqlserver_metrics (UPPER(server_instance_name), capture_timestamp DESC);

DROP INDEX IF EXISTS idx_sqlserver_cpu_server;
CREATE INDEX idx_sqlserver_cpu_server ON sqlserver_cpu_history (UPPER(server_instance_name), capture_timestamp DESC);

DROP INDEX IF EXISTS idx_sqlserver_memory_server;
CREATE INDEX idx_sqlserver_memory_server ON sqlserver_memory_history (UPPER(server_instance_name), capture_timestamp DESC);

DROP INDEX IF EXISTS idx_sqlserver_wait_server;
CREATE INDEX idx_sqlserver_wait_server ON sqlserver_wait_history (UPPER(server_instance_name), capture_timestamp DESC);

DROP INDEX IF EXISTS idx_sqlserver_conn_server;
CREATE INDEX idx_sqlserver_conn_server ON sqlserver_connection_history (UPPER(server_instance_name), capture_timestamp DESC);

DROP INDEX IF EXISTS idx_sqlserver_lock_server;
CREATE INDEX idx_sqlserver_lock_server ON sqlserver_lock_history (UPPER(server_instance_name), capture_timestamp DESC);

DROP INDEX IF EXISTS idx_sqlserver_disk_server;
CREATE INDEX idx_sqlserver_disk_server ON sqlserver_disk_history (UPPER(server_instance_name), capture_timestamp DESC);

DROP INDEX IF EXISTS idx_db_throughput_server_time;
CREATE INDEX idx_db_throughput_server_time ON sqlserver_database_throughput (UPPER(server_instance_name), capture_timestamp DESC);

DROP INDEX IF EXISTS idx_ag_health_server_time;
CREATE INDEX idx_ag_health_server_time ON sqlserver_ag_health (UPPER(server_instance_name), capture_timestamp DESC);

DROP INDEX IF EXISTS idx_sqlserver_job_server;
CREATE INDEX idx_sqlserver_job_server ON sqlserver_job_metrics (UPPER(server_instance_name), capture_timestamp DESC);

DROP INDEX IF EXISTS idx_server_props_server_time;
CREATE INDEX idx_server_props_server_time ON sqlserver_server_properties (UPPER(server_instance_name), capture_timestamp DESC);

DROP INDEX IF EXISTS idx_cpu_scheduler_server_time;
CREATE INDEX idx_cpu_scheduler_server_time ON sqlserver_cpu_scheduler_stats (UPPER(server_instance_name), capture_timestamp DESC);

DROP INDEX IF EXISTS idx_sqlserver_lrq_server_time;
CREATE INDEX idx_sqlserver_lrq_server_time ON sqlserver_long_running_queries (UPPER(server_instance_name), capture_timestamp DESC);

DROP INDEX IF EXISTS idx_perfdebt_server_time;
CREATE INDEX idx_perfdebt_server_time ON sqlserver_performance_debt_findings (UPPER(server_instance_name), capture_timestamp DESC);

DROP INDEX IF EXISTS idx_latch_waits_server_time;
CREATE INDEX idx_latch_waits_server_time ON sqlserver_latch_waits (UPPER(server_instance_name), capture_timestamp DESC);

DROP INDEX IF EXISTS idx_memory_metrics_server_time;
CREATE INDEX idx_memory_metrics_server_time ON sqlserver_memory_metrics (UPPER(server_instance_name), capture_timestamp DESC);

DROP INDEX IF EXISTS idx_waiting_tasks_server_time;
CREATE INDEX idx_waiting_tasks_server_time ON sqlserver_waiting_tasks (UPPER(server_instance_name), capture_timestamp DESC);

DROP INDEX IF EXISTS idx_proc_stats_server_time;
CREATE INDEX idx_proc_stats_server_time ON sqlserver_procedure_stats (UPPER(server_instance_name), capture_timestamp DESC);

DROP INDEX IF EXISTS idx_spinlock_stats_server_time;
CREATE INDEX idx_spinlock_stats_server_time ON sqlserver_spinlock_stats (UPPER(server_instance_name), capture_timestamp DESC);

DROP INDEX IF EXISTS idx_tempdb_files_server_time;
CREATE INDEX idx_tempdb_files_server_time ON sqlserver_tempdb_files (UPPER(server_instance_name), capture_timestamp DESC);

-- 4. Tables with instance_id
DROP INDEX IF EXISTS idx_sqlserver_query_stats_history_instance_ts;
CREATE INDEX idx_sqlserver_query_stats_history_instance_ts ON sqlserver_query_stats_history (UPPER(instance_id), ts DESC);

DROP INDEX IF EXISTS idx_sqlserver_session_snapshot_instance_time;
CREATE INDEX idx_sqlserver_session_snapshot_instance_time ON sqlserver_session_snapshot (UPPER(instance_id), sample_time DESC);

DROP INDEX IF EXISTS idx_sqlserver_query_metrics_v2_instance_ts;
CREATE INDEX idx_sqlserver_query_metrics_v2_instance_ts ON sqlserver_query_metrics_v2 (UPPER(instance_id), ts DESC);

DROP INDEX IF EXISTS idx_pg_query_metrics_v2_instance_ts;
CREATE INDEX idx_pg_query_metrics_v2_instance_ts ON pg_query_metrics_v2 (UPPER(instance_id), ts DESC);

DROP INDEX IF EXISTS idx_pg_ts_metrics_lookup;
CREATE INDEX idx_pg_ts_metrics_lookup ON pg_ts_metrics (UPPER(instance_id), metric, time DESC);

DROP INDEX IF EXISTS idx_pg_query_bucket_instance_time;
CREATE INDEX idx_pg_query_bucket_instance_time ON pg_query_bucket_metrics (UPPER(instance_id), bucket_start DESC);

DROP INDEX IF EXISTS pg_ts_locks_server_instance_name_idx; -- some might have auto-gen names
DROP INDEX IF EXISTS idx_pg_ts_locks_server_time;
CREATE INDEX idx_pg_ts_locks_server_time ON pg_ts_locks (UPPER(server_instance_name), capture_timestamp DESC);

DROP INDEX IF EXISTS idx_pg_ts_stat_server_query;
CREATE INDEX idx_pg_ts_stat_server_query ON pg_ts_stat_statements_delta (UPPER(server_instance_name), query_id, capture_timestamp DESC);

DROP INDEX IF EXISTS idx_pg_ts_snap_server_time;
CREATE INDEX idx_pg_ts_snap_server_time ON pg_ts_instance_snapshot (UPPER(server_instance_name), capture_timestamp DESC);

DROP INDEX IF EXISTS idx_pg_query_metrics_instance_time;
CREATE INDEX idx_pg_query_metrics_instance_time ON pg_query_metrics (UPPER(instance_name), sample_time DESC);

DROP INDEX IF EXISTS idx_sqlserver_blocking_snaps_sqlhash;
CREATE INDEX idx_sqlserver_blocking_snaps_sqlhash ON sqlserver_blocking_snapshots (UPPER(instance_id), sql_hash, ts DESC);

DROP INDEX IF EXISTS idx_sqlserver_blocking_snaps_sid;
CREATE INDEX idx_sqlserver_blocking_snaps_sid ON sqlserver_blocking_snapshots (UPPER(instance_id), session_id, ts DESC);

DROP INDEX IF EXISTS idx_sqlserver_blocking_snaps_login;
CREATE INDEX idx_sqlserver_blocking_snaps_login ON sqlserver_blocking_snapshots (UPPER(instance_id), login_name, ts DESC) WHERE blocking_session_id = 0;

DROP INDEX IF EXISTS idx_sqlserver_blocking_locks_instance_ts;
CREATE INDEX idx_sqlserver_blocking_locks_instance_ts ON sqlserver_blocking_locks (UPPER(instance_id), ts DESC);

DROP INDEX IF EXISTS idx_sqlserver_qr_instance;
CREATE INDEX idx_sqlserver_qr_instance ON sqlserver_query_regressions (UPPER(server_instance_name), capture_time DESC);

DROP INDEX IF EXISTS idx_sqlserver_pi_instance;
CREATE INDEX idx_sqlserver_pi_instance ON sqlserver_plan_instability (UPPER(server_instance_name), capture_time DESC);
