-- Metric: mssql_wait_stats
-- Source: backend/internal/repository/mssql_dashboard.go:219
-- Target Table: sqlserver_wait_history (TimescaleDB)
-- Description: Gets cumulative wait stats excluding noise wait types

SELECT wait_type, CAST(wait_time_ms AS FLOAT) FROM sys.dm_os_wait_stats WITH (NOLOCK) WHERE wait_type NOT IN ('DIRTY_PAGE_POLL', 'HADR_FILESTREAM_IOMGR_IOCOMPLETION', 'LAZYWRITER_SLEEP', 'LOGMGR_QUEUE', 'REQUEST_FOR_DEADLOCK_SEARCH', 'XE_DISPATCHER_WAIT', 'XE_TIMER_EVENT', 'SQLTRACE_BUFFER_FLUSH', 'SLEEP_TASK', 'BROKER_TO_FLUSH', 'SP_SERVER_DIAGNOSTICS_SLEEP') AND wait_time_ms > 0;
