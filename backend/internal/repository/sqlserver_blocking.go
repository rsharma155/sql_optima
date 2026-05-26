// SQL Optima — https://github.com/rsharma155/sql_optima
//
// File: sqlserver_blocking.go
// Purpose: SQL Server blocking chain detection and lock analysis (Redesigned).
// Metadata:
//   - Version: 1.1
//   - Package: repository
//   - Logic: DMVs (sys.dm_os_waiting_tasks, sys.dm_exec_requests)
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"log/slog"
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/rsharma155/sql_optima/internal/models"
)

// FetchBlockingSnapshots returns a detailed list of blocked and blocking sessions.
func (c *SqlServerRepository) FetchBlockingSnapshots(ctx context.Context, db *sql.DB) ([]models.SQLServerBlockingSnapshot, error) {
	query := `
		/* SQL_OPTIMA */
		;WITH waiting_agg_raw AS (
			-- Combine sys.dm_os_waiting_tasks and sys.dm_exec_requests for robust blocking detection
			SELECT 
				session_id, 
				blocking_session_id, 
				wait_type, 
				wait_duration_ms,
				resource_description
			FROM (
				SELECT 
					wt.session_id, 
					wt.blocking_session_id, 
					wt.wait_type, 
					wt.wait_duration_ms,
					wt.resource_description,
					ROW_NUMBER() OVER(PARTITION BY wt.session_id ORDER BY wt.wait_duration_ms DESC) as rn
				FROM sys.dm_os_waiting_tasks wt
				WHERE wt.blocking_session_id IS NOT NULL AND wt.blocking_session_id > 0
			) t WHERE rn = 1
			UNION ALL
			SELECT 
				r.session_id, 
				r.blocking_session_id, 
				r.wait_type, 
				r.wait_time as wait_duration_ms,
				'' as resource_description
			FROM sys.dm_exec_requests r
			WHERE r.blocking_session_id IS NOT NULL AND r.blocking_session_id > 0
			AND NOT EXISTS (SELECT 1 FROM sys.dm_os_waiting_tasks wt WHERE wt.session_id = r.session_id)
		),
		waiting_agg AS (
			SELECT session_id, blocking_session_id, wait_type, wait_duration_ms, resource_description
			FROM (
				SELECT *, ROW_NUMBER() OVER(PARTITION BY session_id ORDER BY wait_duration_ms DESC) as rn
				FROM waiting_agg_raw
			) t WHERE rn = 1
		),
		requests AS (
			SELECT
				r.session_id,
				r.status,
				r.command,
				r.database_id,
				r.cpu_time,
				r.total_elapsed_time,
				r.reads,
				r.writes,
				r.logical_reads,
				r.sql_handle,
				r.plan_handle,
				r.wait_resource,
				r.percent_complete,
				r.statement_start_offset,
				r.statement_end_offset,
				CONVERT(VARCHAR(64), r.sql_handle, 1) AS sql_hash,
				CONVERT(VARCHAR(64), r.plan_handle, 1) AS plan_hash
			FROM sys.dm_exec_requests r
		),
		sessions AS (
			SELECT
				s.session_id,
				s.login_name,
				s.host_name,
				s.program_name,
				s.open_transaction_count,
				s.status,
				s.memory_usage,
				CASE s.transaction_isolation_level 
					WHEN 0 THEN 'Unspecified' 
					WHEN 1 THEN 'ReadUncommitted' 
					WHEN 2 THEN 'ReadCommitted' 
					WHEN 3 THEN 'Repeatable' 
					WHEN 4 THEN 'Serializable' 
					WHEN 5 THEN 'Snapshot' 
					ELSE 'Unknown'
				END AS isolation_level
			FROM sys.dm_exec_sessions s
			WHERE s.is_user_process = 1
			   OR s.session_id IN (SELECT blocking_session_id FROM waiting_agg)
		),
		transactions AS (
			SELECT 
				st.session_id, 
				MIN(at.transaction_begin_time) as begin_time,
				SUM(dt.database_transaction_log_bytes_used) as log_bytes_used,
				SUM(dt.database_transaction_log_bytes_reserved) as log_bytes_reserved
			FROM sys.dm_tran_session_transactions st
			JOIN sys.dm_tran_active_transactions at ON st.transaction_id = at.transaction_id
			LEFT JOIN sys.dm_tran_database_transactions dt ON at.transaction_id = dt.transaction_id
			GROUP BY st.session_id
		),
		connections AS (
			SELECT session_id, most_recent_sql_handle
			FROM sys.dm_exec_connections WITH (NOLOCK)
		)
		SELECT DISTINCT
			SYSUTCDATETIME() AS ts,
			s.session_id,
			ISNULL(w.blocking_session_id, 0) AS blocking_session_id,
			ISNULL(w.wait_type, '') AS wait_type,
			ISNULL(w.wait_duration_ms, 0) AS wait_duration_ms,
			ISNULL(DB_NAME(r.database_id), '') AS database_name,
			ISNULL(s.login_name, 'system') AS login_name,
			ISNULL(s.host_name, '') AS host_name,
			ISNULL(s.program_name, '') AS program_name,
			ISNULL(s.open_transaction_count, 0) AS open_transaction_count,
			t.begin_time AS transaction_start_time,
			ISNULL(s.isolation_level, 'Unknown') AS isolation_level,
			ISNULL(r.cpu_time, 0) AS cpu_time,
			ISNULL(r.reads, 0) AS reads,
			ISNULL(r.writes, 0) AS writes,
			ISNULL(r.logical_reads, 0) AS logical_reads,
			ISNULL(s.memory_usage, 0) AS memory_usage,
			ISNULL(t.log_bytes_used, 0) AS log_bytes_used,
			ISNULL(t.log_bytes_reserved, 0) AS log_bytes_reserved,
			ISNULL(w.resource_description, ISNULL(r.wait_resource, '')) AS wait_resource,
			ISNULL(r.percent_complete, 0.0) AS percent_complete,
			ISNULL(r.sql_hash, CONVERT(VARCHAR(64), c.most_recent_sql_handle, 1)) AS sql_hash,
			ISNULL(r.plan_hash, '') AS plan_hash,
			-- Prefer the actual call (input buffer = "EXEC sp_name @p=v") over the SP definition body.
			-- For ad-hoc batches, extract just the currently-executing statement via offsets.
			CASE
				WHEN st.objectid IS NOT NULL THEN
					-- Stored procedure: show the EXEC call from input buffer, fall back to SP name
					ISNULL(NULLIF(LTRIM(RTRIM(ib.event_info)), ''),
						'EXEC ' + ISNULL(OBJECT_SCHEMA_NAME(st.objectid, r.database_id) + '.', '') +
						ISNULL(OBJECT_NAME(st.objectid, r.database_id), ''))
				WHEN r.statement_start_offset IS NOT NULL AND st.text IS NOT NULL THEN
					-- Ad-hoc batch: extract only the statement currently running
					LTRIM(RTRIM(SUBSTRING(st.text, r.statement_start_offset / 2 + 1,
						(CASE WHEN r.statement_end_offset = -1 THEN DATALENGTH(st.text)
							  ELSE r.statement_end_offset END - r.statement_start_offset) / 2 + 1)))
				ELSE
					ISNULL(NULLIF(LTRIM(RTRIM(ib.event_info)), ''), ISNULL(st.text, cst.text))
			END AS query_text,
			qp.query_plan
		FROM sessions s
		LEFT JOIN requests r ON s.session_id = r.session_id
		LEFT JOIN waiting_agg w ON s.session_id = w.session_id
		LEFT JOIN transactions t ON s.session_id = t.session_id
		LEFT JOIN connections c ON s.session_id = c.session_id
		OUTER APPLY sys.dm_exec_sql_text(r.sql_handle) st
		OUTER APPLY sys.dm_exec_sql_text(c.most_recent_sql_handle) cst
		OUTER APPLY sys.dm_exec_input_buffer(s.session_id, 0) ib
		OUTER APPLY (
			SELECT CAST(query_plan AS NVARCHAR(MAX)) AS query_plan
			FROM sys.dm_exec_query_plan(r.plan_handle)
			WHERE ISNULL(w.wait_duration_ms, 0) > 5000
		) qp
		WHERE w.blocking_session_id IS NOT NULL 
		   OR s.session_id IN (SELECT blocking_session_id FROM waiting_agg);
	`

	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		slog.Error("[SQLSERVER] FetchBlockingSnapshots Error", "err", err)
		return nil, err
	}
	defer rows.Close()

	var results []models.SQLServerBlockingSnapshot
	for rows.Next() {
		var r models.SQLServerBlockingSnapshot
		var sqlText, queryPlan, waitType, dbName, loginName, hostName, progName, isoLevel, sqlHash, planHash, waitRes sql.NullString
		var xactStart sql.NullTime

		if err := rows.Scan(
			&r.Timestamp, &r.SessionID, &r.BlockingSessionID, &waitType, &r.WaitDurationMs,
			&dbName, &loginName, &hostName, &progName,
			&r.OpenTransactionCount, &xactStart, &isoLevel,
			&r.CpuTime, &r.Reads, &r.Writes, &r.LogicalReads,
			&r.MemoryUsage, &r.TransactionLogBytesUsed, &r.TransactionLogBytesReserved,
			&waitRes, &r.PercentComplete,
			&sqlHash, &planHash, &sqlText, &queryPlan,
		); err != nil {
			slog.Error("[SQLSERVER] FetchBlockingSnapshots Scan Error", "err", err)
			continue
		}

		r.WaitType = waitType.String
		r.DatabaseName = dbName.String
		r.LoginName = loginName.String
		r.HostName = hostName.String
		r.ProgramName = progName.String
		r.TransactionIsolationLevel = isoLevel.String
		r.WaitResource = waitRes.String
		r.SqlHash = sqlHash.String
		r.PlanHash = planHash.String
		r.QueryText = sqlText.String
		r.QueryPlan = queryPlan.String

		if xactStart.Valid {
			t := xactStart.Time
			r.TransactionStartTime = &t
		}

		results = append(results, r)
	}
	return results, nil
}

// FetchBlockingLocks returns detailed lock information for sessions involved in blocking.
func (c *SqlServerRepository) FetchBlockingLocks(ctx context.Context, db *sql.DB) ([]models.SQLServerBlockingLock, error) {
	query := `
		/* SQL_OPTIMA */
		SELECT 
			SYSUTCDATETIME() AS ts,
			tl.request_session_id AS session_id,
			tl.resource_type,
			tl.request_mode,
			tl.request_status,
			COALESCE(
				CASE 
					WHEN tl.resource_type = 'OBJECT' THEN OBJECT_NAME(tl.resource_associated_entity_id, tl.resource_database_id)
					WHEN tl.resource_type IN ('PAGE', 'KEY', 'RID', 'HOBT') THEN 
						(SELECT OBJECT_NAME(object_id, tl.resource_database_id) FROM sys.partitions WHERE hobt_id = tl.resource_associated_entity_id)
					ELSE tl.resource_description
				END, 
				tl.resource_description,
				'Unknown Object'
			) AS resource_description,
			MAX(ISNULL(wt.wait_duration_ms, 0)) AS wait_duration_ms
		FROM sys.dm_tran_locks tl
		LEFT JOIN sys.dm_os_waiting_tasks wt ON tl.request_session_id = wt.session_id
		WHERE tl.request_session_id IN (
			SELECT session_id FROM sys.dm_os_waiting_tasks WHERE blocking_session_id IS NOT NULL
			UNION
			SELECT blocking_session_id FROM sys.dm_os_waiting_tasks WHERE blocking_session_id IS NOT NULL
		)
		GROUP BY tl.request_session_id, tl.resource_type, tl.request_mode, tl.request_status, 
		         tl.resource_associated_entity_id, tl.resource_database_id, tl.resource_description;
	`
	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []models.SQLServerBlockingLock
	for rows.Next() {
		var l models.SQLServerBlockingLock
		if err := rows.Scan(&l.Timestamp, &l.SessionID, &l.ResourceType, &l.RequestMode, &l.RequestStatus, &l.ResourceDescription, &l.WaitDurationMs); err != nil {
			continue
		}
		results = append(results, l)
	}
	return results, nil
}

// FetchDeadlockEvents reads deadlock events from Extended Events
func (c *SqlServerRepository) FetchDeadlockEvents(ctx context.Context, db *sql.DB) ([]models.SQLServerDeadlockEvent, error) {
	sessionName, targetPath, err := c.discoverDeadlockXESession(ctx, db)
	if err != nil || sessionName == "" {
		return nil, nil
	}

	query := fmt.Sprintf(`
		/* SQL_OPTIMA */
		SELECT 
			CAST(event_data AS XML).value('(/event/@timestamp)[1]', 'datetime2') AS ts,
			ISNULL(CAST(event_data AS XML).value('(/event/data[@name="database_name"]/value)[1]', 'nvarchar(max)'), 'Unknown') AS database_name,
			ISNULL(CAST(event_data AS XML).value('(/event/data[@name="victim_process_id"]/value)[1]', 'int'), 0) AS victim_session_id,
			event_data AS deadlock_graph
		FROM sys.fn_xe_file_target_read_file('%s', NULL, NULL, NULL)
		WHERE CAST(event_data AS XML).value('(/event/@timestamp)[1]', 'datetime2') > DATEADD(hour, -24, GETUTCDATE());
	`, targetPath)

	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, nil
	}
	defer rows.Close()

	var results []models.SQLServerDeadlockEvent
	for rows.Next() {
		var e models.SQLServerDeadlockEvent
		if err := rows.Scan(&e.Timestamp, &e.DatabaseName, &e.VictimSessionID, &e.DeadlockGraph); err != nil {
			continue
		}
		results = append(results, e)
	}
	return results, nil
}

// IsDeadlockXESessionEnabled checks if a deadlock XE session is defined
func (c *SqlServerRepository) IsDeadlockXESessionEnabled(ctx context.Context, db *sql.DB) (bool, error) {
	checkQuery := `
		SELECT COUNT(*) 
		FROM sys.server_event_session_events e
		JOIN sys.server_event_sessions s ON e.event_session_id = s.event_session_id
		WHERE e.name = 'xml_deadlock_report'
		  AND s.name <> 'system_health'
	`
	var count int
	ctx, cancel := WithQueryTimeout(ctx, 0)
	err := db.QueryRowContext(ctx, checkQuery).Scan(&count)
	cancel()
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// EnsureDeadlockXESession creates the XE session if missing
func (c *SqlServerRepository) EnsureDeadlockXESession(ctx context.Context, db *sql.DB) error {
	sessionName, _, _ := c.discoverDeadlockXESession(ctx, db)
	
	if sessionName == "" || sessionName == "system_health" {
		// Create preferred session if no custom session exists
		createSQL := `
			IF NOT EXISTS (SELECT 1 FROM sys.server_event_sessions WHERE name = 'sqloptima_deadlocks')
			BEGIN
				CREATE EVENT SESSION [sqloptima_deadlocks]
				ON SERVER
				ADD EVENT sqlserver.xml_deadlock_report
				ADD TARGET package0.event_file
				(
					SET filename = 'sqloptima_deadlocks.xel',
						max_file_size = 50,
						max_rollover_files = 10
				)
				WITH (STARTUP_STATE = ON);
			END
		`
		ctx, cancel := WithQueryTimeout(ctx, 0)
		defer cancel()
		_, err := db.ExecContext(ctx, createSQL)
		if err != nil {
			slog.Error("[SQLSERVER] Failed to create Deadlock XE session", "err", err)
			return err
		}
		sessionName = "sqloptima_deadlocks"
	}

	// Ensure the session is running
	startSQL := fmt.Sprintf(`
		IF NOT EXISTS (SELECT 1 FROM sys.dm_xe_sessions WHERE name = '%s')
		BEGIN
			DECLARE @sql NVARCHAR(MAX) = 'ALTER EVENT SESSION [' + @p1 + '] ON SERVER STATE = START';
			EXEC sp_executesql @sql;
		END
	`, sessionName)
	
	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	_, err := db.ExecContext(ctx, startSQL, sessionName)
	return err
}

// discoverDeadlockXESession finds any session capturing deadlock reports and its file path
func (c *SqlServerRepository) discoverDeadlockXESession(ctx context.Context, db *sql.DB) (string, string, error) {
	// Find the session name first
	sessionQuery := `
		SELECT TOP 1 s.name
		FROM sys.server_event_session_events e
		JOIN sys.server_event_sessions s ON e.event_session_id = s.event_session_id
		WHERE e.name = 'xml_deadlock_report'
		ORDER BY CASE 
			WHEN s.name = 'sqloptima_deadlocks' THEN 0 
			WHEN s.name = 'dbamon_deadlocks' THEN 1 
			WHEN s.name = 'system_health' THEN 3
			ELSE 2 END
	`
	var sessionName string
	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	err := db.QueryRowContext(ctx, sessionQuery).Scan(&sessionName)
	if err != nil {
		return "", "", err
	}

	// Find the target path
	pathQuery := `
		SELECT TOP 1 
			ISNULL(
				CAST(t.target_data AS XML).value('(/EventFileTarget/File/@name)[1]', 'nvarchar(max)'),
				(SELECT CAST(f.value AS NVARCHAR(MAX)) 
				 FROM sys.server_event_session_fields f 
				 WHERE f.event_session_id = s.event_session_id AND f.name = 'filename')
			) AS target_path
		FROM sys.server_event_sessions s
		LEFT JOIN sys.dm_xe_sessions ds ON ds.name = s.name
		LEFT JOIN sys.dm_xe_session_targets t ON t.event_session_address = ds.address AND t.target_name = 'event_file'
		WHERE s.name = @p1
	`

	var targetPath string
	ctx, cancel = WithQueryTimeout(ctx, 0)
	defer cancel()
	if err := db.QueryRowContext(ctx, pathQuery, sessionName).Scan(&targetPath); err != nil {
		targetPath = ""
	}

	if targetPath == "" {
		targetPath = sessionName + "*.xel"
	}

	// Ensure wildcard
	if !strings.Contains(targetPath, "*") {
		if strings.HasSuffix(targetPath, ".xel") {
			targetPath = strings.Replace(targetPath, ".xel", "*.xel", 1)
		} else if !strings.Contains(targetPath, ".xel") {
			targetPath += "*.xel"
		}
	}

	return sessionName, targetPath, nil
}

// --------------------------------------------------------------------------
// DEPRECATED / LEGACY METHODS (Kept for backward compatibility)
// --------------------------------------------------------------------------

// CollectBlockingChains returns aggregated blockers for Real-Time Diagnostics (user databases only; matches UI shape).
func (c *SqlServerRepository) CollectBlockingChains(ctx context.Context, db *sql.DB, database string) ([]map[string]interface{}, error) {
	query := `
		/* SQL_OPTIMA */ 
		;WITH BlockingTree AS (
			SELECT  r.session_id AS Blocked_SPID,
				r.blocking_session_id AS Blocking_SPID,
				r.wait_time
			FROM sys.dm_exec_requests r
			INNER JOIN sys.dm_exec_sessions s ON r.session_id = s.session_id
			WHERE r.blocking_session_id > 0
			  AND r.session_id > 50
			  AND s.is_user_process = 1
			  AND s.database_id > 4
			  AND LOWER(ISNULL(DB_NAME(s.database_id), '')) <> 'distribution'
			  AND (@p1 = '' OR DB_NAME(s.database_id) = @p1)
		  AND LOWER(ISNULL(s.login_name, '')) NOT IN ('dbmonitor_user', 'sql-optima')
		  AND LOWER(ISNULL(s.program_name, '')) NOT IN ('dbmonitor_user', 'sql-optima')
		)
		SELECT   TOP 50
			b.Blocking_SPID AS Lead_Blocker,
			ISNULL(s.login_name, 'Unknown') AS Blocker_Login,
			ISNULL(s.program_name, 'Unknown') AS Blocker_App,
			COUNT(b.Blocked_SPID) AS Total_Victims,
			MAX(b.wait_time) AS Max_Wait_Time_ms
		FROM BlockingTree b
		INNER JOIN sys.dm_exec_sessions s ON b.Blocking_SPID = s.session_id
		GROUP BY b.Blocking_SPID, s.login_name, s.program_name
		ORDER BY Max_Wait_Time_ms DESC
	`

	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	rows, err := db.QueryContext(ctx, query, strings.TrimSpace(database))
	if err != nil {
		slog.Error("[SQLSERVER] Blocking Query Error", "err", err)
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var resultsMap = make(map[string]interface{})
		columns, _ := rows.Columns()
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}
		if err := rows.Scan(valuePtrs...); err == nil {
			for i, col := range columns {
				resultsMap[col] = values[i]
			}
			results = append(results, resultsMap)
		}
	}
	return results, nil
}

// CollectLocks fetches lock statistics from sys.dm_tran_locks
func (c *SqlServerRepository) CollectLocks(ctx context.Context, db *sql.DB) (int, int, map[string]int, error) {
	totalLocksQuery := `SELECT /* SQL_OPTIMA */   COUNT(*) FROM sys.dm_tran_locks WHERE request_session_id > 50`
	var totalLocks int
	ctx, cancel := WithQueryTimeout(ctx, 0)
	if err := db.QueryRowContext(ctx, totalLocksQuery).Scan(&totalLocks); err != nil {
	cancel()
		slog.Error("[SQLSERVER] Total Locks Query Error", "err", err)
	}

	deadlockQuery := `
		/* SQL_OPTIMA */ 
		;WITH CTE AS (
			SELECT  ROW_NUMBER() OVER(PARTITION BY blocked ORDER BY blocked DESC) AS rn, blocked, blocking
			FROM (
				SELECT  blocked, 0 AS blocking FROM sys.dm_exec_requests WHERE blocked > 0
				UNION ALL
				SELECT  0, blocking_session_id FROM sys.dm_exec_requests WHERE blocking_session_id > 0
			) a(bblocked, bblocking)
		)
		SELECT COUNT(*) FROM CTE WHERE rn > 1
	`
	var deadlocks int
	ctx, cancel = WithQueryTimeout(ctx, 0)
	defer cancel()
	if err := db.QueryRowContext(ctx, deadlockQuery).Scan(&deadlocks); err != nil {
		slog.Error("[SQLSERVER] Deadlock Query Error", "err", err)
	}

	locksByDB := make(map[string]int)
	dbLockQuery := `
		/* SQL_OPTIMA */ SELECT  ISNULL(DB_NAME(resource_database_id), 'Unknown'), COUNT(*)
		FROM sys.dm_tran_locks
		WHERE request_session_id > 50
		GROUP BY resource_database_id
	`
	ctx, cancel = WithQueryTimeout(ctx, 0)
	defer cancel()
	rows, err := db.QueryContext(ctx, dbLockQuery)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var dbName string
			var count int
			if err := rows.Scan(&dbName, &count); err == nil {
				locksByDB[dbName] = count
			}
		}
	}

	return totalLocks, deadlocks, locksByDB, nil
}

// CollectSpinlockStats fetches spinlock statistics
func (c *SqlServerRepository) CollectSpinlockStats(ctx context.Context, db *sql.DB) ([]map[string]interface{}, error) {
	query := `
		/* SQL_OPTIMA */ 
		SELECT   TOP 20 
			name AS spinlock_type, 
			collisions, 
			spins, 
			sleep_time AS sleep_time_ms,
			backoffs,
			spins_per_collision
		FROM sys.dm_os_spinlock_stats 
		ORDER BY spins DESC
	`

	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var name, collisions, spins, sleep_time, backoffs, spins_per_collision interface{}
		if err := rows.Scan(&name, &collisions, &spins, &sleep_time, &backoffs, &spins_per_collision); err == nil {
			results = append(results, map[string]interface{}{
				"spinlock_type":       name,
				"collisions":          collisions,
				"spins":               spins,
				"sleep_time":          sleep_time,
				"backoffs":            backoffs,
				"spins_per_collision": spins_per_collision,
			})
		}
	}
	return results, nil
}
