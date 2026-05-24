// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: SQL Server active session collector — joins dm_exec_sessions +
// dm_exec_requests into a single snapshot written to sqlserver_active_sessions.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package sqlserver

import (
	"context"
	"database/sql"
)

// ActiveSessionRow holds one row from the session/request join.
type ActiveSessionRow struct {
	SessionID            int
	LoginName            string
	HostName             string
	ProgramName          string
	DatabaseName         string
	RequestStatus        string
	WaitType             string
	WaitTimeMs           int64
	BlockingSessionID    int
	CPUTimeMs            int64
	TotalElapsedMs       int64
	LogicalReads         int64
	Reads                int64
	Writes               int64
	GrantedQueryMemoryKB int64
	DOP                  int
	QueryHash            string
	QueryText            string
}

// SessionCollector fetches active sessions from SQL Server in one query.
type SessionCollector struct{}

// Fetch returns all active user sessions (excludes system and monitoring accounts).
func (c *SessionCollector) Fetch(ctx context.Context, db *sql.DB) ([]ActiveSessionRow, error) {
	const query = `
		/* SQL_OPTIMA */
		SELECT
			s.session_id,
			ISNULL(s.login_name, '')                                      AS login_name,
			ISNULL(s.host_name, '')                                       AS host_name,
			ISNULL(s.program_name, '')                                    AS program_name,
			ISNULL(DB_NAME(ISNULL(r.database_id, s.database_id)), '')     AS database_name,
			ISNULL(r.status, s.status)                                    AS request_status,
			ISNULL(r.wait_type, '')                                       AS wait_type,
			ISNULL(r.wait_time, 0)                                        AS wait_time_ms,
			ISNULL(r.blocking_session_id, 0)                              AS blocking_session_id,
			ISNULL(r.cpu_time, s.cpu_time)                                AS cpu_time_ms,
			ISNULL(r.total_elapsed_time, 0)                               AS total_elapsed_ms,
			ISNULL(r.logical_reads, 0)                                    AS logical_reads,
			ISNULL(r.reads, 0)                                            AS reads,
			ISNULL(r.writes, 0)                                           AS writes,
			ISNULL(r.granted_query_memory * 8, 0)                         AS granted_query_memory_kb,
			ISNULL(r.dop, 0)                                              AS dop,
			ISNULL(CONVERT(VARCHAR(32), r.query_hash, 1), '')             AS query_hash,
			ISNULL(SUBSTRING(qt.text, (r.statement_start_offset/2)+1,
				CASE r.statement_end_offset
					WHEN -1 THEN DATALENGTH(qt.text)
					ELSE r.statement_end_offset
				END - r.statement_start_offset/2 + 1), 
				ISNULL(ct.text, ''))                                     AS query_text
		FROM sys.dm_exec_sessions s WITH (NOLOCK)
		LEFT JOIN sys.dm_exec_requests r WITH (NOLOCK)
			ON s.session_id = r.session_id
		LEFT JOIN sys.dm_exec_connections c WITH (NOLOCK)
			ON s.session_id = c.session_id
		OUTER APPLY sys.dm_exec_sql_text(r.sql_handle) qt
		OUTER APPLY sys.dm_exec_sql_text(c.most_recent_sql_handle) ct
		WHERE s.is_user_process = 1
		  AND s.database_id > 4
		  AND LOWER(ISNULL(s.login_name, ''))    NOT IN ('dbmonitor_user', 'sql-optima')
		  AND LOWER(ISNULL(s.program_name, ''))  NOT IN ('dbmonitor_user', 'sql-optima')
		OPTION (RECOMPILE);`

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ActiveSessionRow
	for rows.Next() {
		var row ActiveSessionRow
		if err := rows.Scan(
			&row.SessionID, &row.LoginName, &row.HostName, &row.ProgramName,
			&row.DatabaseName, &row.RequestStatus, &row.WaitType, &row.WaitTimeMs,
			&row.BlockingSessionID, &row.CPUTimeMs, &row.TotalElapsedMs,
			&row.LogicalReads, &row.Reads, &row.Writes,
			&row.GrantedQueryMemoryKB, &row.DOP, &row.QueryHash, &row.QueryText,
		); err != nil {
			continue
		}
		out = append(out, row)
	}
	return out, rows.Err()
}
