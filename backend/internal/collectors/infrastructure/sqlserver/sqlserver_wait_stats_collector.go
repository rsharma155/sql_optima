// Package sqlserver contains SQL Server DMV collection logic.
// sqlserver_wait_stats_collector.go collects wait stats snapshots
// for the new Wait Stats Dashboard V2 pipeline.
// Metadata:
//   - Feature: SQL Server Wait Stats V2
//   - Layer: Infrastructure (DMV Collection)
package sqlserver

import (
	"log/slog"
	"context"
	"database/sql"
	"fmt"

	"github.com/rsharma155/sql_optima/internal/collectors/domain"
)

// WaitStatsDMVCollector executes DMV queries on a monitored SQL Server instance.
// It is stateless — state is held by the calling orchestrator.
type WaitStatsDMVCollector struct{}

// FetchCumulativeSnapshot executes the Tier-1 query (sys.dm_os_wait_stats)
// and returns all actionable rows. The caller computes deltas.
func (c *WaitStatsDMVCollector) FetchCumulativeSnapshot(
	ctx context.Context,
	db *sql.DB,
) ([]domain.WaitStatsCumulativeSnapshot, error) {

	const query = `/* SQL_OPTIMA */
    SELECT
        wait_type,
        waiting_tasks_count,
        wait_time_ms,
        signal_wait_time_ms,
        (wait_time_ms - signal_wait_time_ms) AS resource_wait_time_ms
    FROM sys.dm_os_wait_stats WITH (NOLOCK)
    WHERE wait_type NOT IN (
        -- Benign background idle waits
        N'BROKER_EVENTHANDLER', N'BROKER_RECEIVE_WAITFOR', N'BROKER_TASK_STOP',
        N'BROKER_TO_FLUSH', N'BROKER_TRANSMITTER', N'CHECKPOINT_QUEUE',
        N'CHKPT', N'CLR_AUTO_EVENT', N'CLR_MANUAL_EVENT', N'CLR_SEMAPHORE',
        N'CXCOMPILER', N'DBMIRROR_DBM_EVENT', N'DBMIRROR_EVENTS_QUEUE',
        N'DBMIRROR_WORKER_QUEUE', N'DBMIRRORING_CMD', N'DIRTY_PAGE_POLL',
        N'DISPATCHER_QUEUE_SEMAPHORE', N'FSAGENT', N'FT_IFTS_SCHEDULER_IDLE_WAIT',
        N'FT_IFTSHC_MUTEX', N'HADR_CLUSAPI_CALL', N'HADR_FILESTREAM_IOMGR_IOCOMPLETION',
        N'HADR_LOGCAPTURE_WAIT', N'HADR_NOTIFICATION_DEQUEUE',
        N'HADR_TIMER_TASK', N'HADR_WORK_QUEUE',
        N'KSOURCE_WAKEUP', N'LAZYWRITER_SLEEP', N'LOGMGR_QUEUE',
        N'MEMORY_ALLOCATION_EXT', N'ONDEMAND_TASK_QUEUE',
        N'PARALLEL_REDO_DRAIN_WORKER', N'PARALLEL_REDO_LOG_CACHE',
        N'PARALLEL_REDO_TRAN_LIST', N'PARALLEL_REDO_WORKER_SYNC',
        N'PARALLEL_REDO_WORKER_WAIT_WORK', N'PREEMPTIVE_XE_GETTARGETSTATE',
        N'PWAIT_ALL_COMPONENTS_INITIALIZED', N'PWAIT_DIRECTLOGCONSUMER_GETNEXT',
        N'QDS_PERSIST_TASK_MAIN_LOOP_SLEEP', N'QDS_ASYNC_QUEUE',
        N'QDS_CLEANUP_STALE_QUERIES_TASK_MAIN_LOOP_SLEEP', N'QDS_SHUTDOWN_QUEUE',
        N'REDO_THREAD_PENDING_WORK', N'REQUEST_FOR_DEADLOCK_SEARCH',
        N'RESOURCE_QUEUE', N'SERVER_IDLE_CHECK', N'SLEEP_BPOOL_FLUSH',
        N'SLEEP_DBSTARTUP', N'SLEEP_DCOMSTARTUP', N'SLEEP_MASTERDBREADY',
        N'SLEEP_MASTERMDREADY', N'SLEEP_MASTERUPGRADED', N'SLEEP_MSDBSTARTUP',
        N'SLEEP_SYSTEMTASK', N'SLEEP_TASK', N'SLEEP_TEMPDBSTARTUP',
        N'SNI_HTTP_ACCEPT', N'SOS_WORK_DISPATCHER', N'SP_SERVER_DIAGNOSTICS_SLEEP',
        N'SQLTRACE_BUFFER_FLUSH', N'SQLTRACE_INCREMENTAL_FLUSH_SLEEP',
        N'SQLTRACE_WAIT_ENTRIES', N'VDI_CLIENT_OTHER', N'WAIT_FOR_RESULTS',
        N'WAITFOR', N'WAITFOR_TASKSHUTDOWN', N'WAIT_XTP_RECOVERY',
        N'WAIT_XTP_HOST_WAIT', N'WAIT_XTP_OFFLINE_CKPT_NEW_LOG',
        N'WAIT_XTP_CKPT_CLOSE', N'XE_DISPATCHER_JOIN', N'XE_DISPATCHER_WAIT',
        N'XE_TIMER_EVENT'
    )
    AND waiting_tasks_count > 0
    ORDER BY wait_time_ms DESC`

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("FetchCumulativeSnapshot: %w", err)
	}
	defer rows.Close()

	var out []domain.WaitStatsCumulativeSnapshot
	for rows.Next() {
		var s domain.WaitStatsCumulativeSnapshot
		if err := rows.Scan(
			&s.WaitType,
			&s.WaitingTasksCount,
			&s.WaitTimeMs,
			&s.SignalWaitTimeMs,
			&s.ResourceWaitTimeMs,
		); err != nil {
			slog.Error("[WaitStatsDMVCollector] scan error", "err", err)
			continue
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// FetchActiveWaitSessions executes the Tier-2 query (enriched waiting tasks).
// Uses READ UNCOMMITTED to avoid becoming part of the blocking chain.
func (c *WaitStatsDMVCollector) FetchActiveWaitSessions(
	ctx context.Context,
	db *sql.DB,
) ([]domain.ActiveWaitSession, error) {

	const query = `/* SQL_OPTIMA */
    SET TRANSACTION ISOLATION LEVEL READ UNCOMMITTED;
    SELECT TOP 100
        wt.session_id,
        wt.wait_type,
        wt.wait_duration_ms,
        wt.blocking_session_id,
        DB_NAME(r.database_id)        AS database_name,
        s.host_name,
        s.program_name,
        s.login_name,
        SUBSTRING(st.text,
            (r.statement_start_offset / 2) + 1,
            ((CASE r.statement_end_offset
                WHEN -1 THEN DATALENGTH(st.text)
                ELSE r.statement_end_offset
              END - r.statement_start_offset) / 2) + 1
        )                             AS query_text
    FROM sys.dm_os_waiting_tasks wt WITH (NOLOCK)
    INNER JOIN sys.dm_exec_sessions s WITH (NOLOCK) ON wt.session_id = s.session_id
    INNER JOIN sys.dm_exec_requests r WITH (NOLOCK) ON s.session_id = r.session_id
    OUTER APPLY sys.dm_exec_sql_text(r.sql_handle) st
    WHERE s.is_user_process = 1
      AND s.database_id > 4
      AND LOWER(ISNULL(DB_NAME(s.database_id),'')) <> 'distribution'
      AND LOWER(ISNULL(s.login_name,''))   NOT IN ('dbmonitor_user','sql-optima')
      AND LOWER(ISNULL(s.program_name,'')) NOT IN ('dbmonitor_user','sql-optima')
      AND wt.wait_type NOT IN (N'SLEEP_TASK', N'WAITFOR', N'BROKER_TO_FLUSH')
      AND wt.session_id <> @@SPID
    ORDER BY wt.wait_duration_ms DESC`

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("FetchActiveWaitSessions: %w", err)
	}
	defer rows.Close()

	var out []domain.ActiveWaitSession
	for rows.Next() {
		var s domain.ActiveWaitSession
		var blockingID sql.NullInt32
		var dbName, host, program, login, qtext sql.NullString
		if err := rows.Scan(
			&s.SessionID, &s.WaitType, &s.WaitDurationMs, &blockingID,
			&dbName, &host, &program, &login, &qtext,
		); err != nil {
			continue
		}
		if blockingID.Valid {
			id := int(blockingID.Int32)
			s.BlockingSessionID = &id
		}
		s.DatabaseName = dbName.String
		s.HostName = host.String
		s.ProgramName = program.String
		s.LoginName = login.String
		s.QueryText = qtext.String
		out = append(out, s)
	}
	return out, rows.Err()
}
