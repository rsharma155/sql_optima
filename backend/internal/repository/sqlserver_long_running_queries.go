// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: SQL Server long running query statistics.
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
	"time"
)

type LongRunningQueryStats struct {
	SessionID            int
	RequestID            int
	DatabaseName         string
	LoginName            string
	HostName             string
	ProgramName          string
	QueryHash            string
	QueryText            string
	WaitType             string
	BlockingSessionID    int
	Status               string
	CPUTimeMs            int64
	TotalElapsedTimeMs   int64
	Reads                int64
	Writes               int64
	GrantedQueryMemoryMB int
	RowCount             int64
}

func (c *SqlServerRepository) FetchLongRunningQueries(ctx context.Context, instanceName string, minDurationSeconds int) ([]LongRunningQueryStats, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("no connection for instance: %s", instanceName)
	}

	query := `
		/* SQL_OPTIMA */ SELECT   TOP 50
			r.session_id,
			r.request_id,
			DB_NAME(r.database_id) AS database_name,
			s.login_name,
			s.host_name,
			s.program_name,
			CONVERT(VARCHAR(64), r.sql_handle, 1) AS query_hash,
			CASE 
				WHEN qp.objectid IS NOT NULL 
				THEN QUOTENAME(OBJECT_SCHEMA_NAME(qp.objectid, r.database_id)) 
					 + '.' + QUOTENAME(OBJECT_NAME(qp.objectid, r.database_id))
				ELSE
					SUBSTRING(
						qt.text,
						(r.statement_start_offset/2) + 1,
						(
							CASE r.statement_end_offset
								WHEN -1 THEN DATALENGTH(qt.text)
								ELSE r.statement_end_offset
							END - r.statement_start_offset
						) / 2 + 1
					)
			END AS query_text,
			r.wait_type,
			r.blocking_session_id,
			r.status,
			r.cpu_time AS cpu_time_ms,
			r.total_elapsed_time AS total_elapsed_time_ms,
			r.reads,
			r.writes,
			(r.granted_query_memory * 8) / 1024 AS granted_query_memory_mb,
			r.row_count
		FROM sys.dm_exec_requests r
		JOIN sys.dm_exec_sessions s 
			ON r.session_id = s.session_id
		CROSS APPLY sys.dm_exec_sql_text(r.sql_handle) qt
		OUTER APPLY sys.dm_exec_query_plan(r.plan_handle) qp
		WHERE r.session_id <> @@SPID AND r.session_id > 50
		AND r.total_elapsed_time >= 5000
		AND s.is_user_process = 1
		AND LOWER(ISNULL(s.login_name, '')) NOT IN ('dbmonitor_user', 'sql-optima')
		AND LOWER(ISNULL(s.program_name, '')) NOT IN ('dbmonitor_user', 'sql-optima')
		ORDER BY r.total_elapsed_time DESC`

	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		slog.Error("[SQLSERVER] FetchLongRunningQueries Error", "target", instanceName, "err", err)
		return nil, err
	}
	defer rows.Close()

	var results []LongRunningQueryStats
	for rows.Next() {
		var s LongRunningQueryStats
		var qhash sql.NullString
		var waitType sql.NullString
		var cpuTime, totalElapsed, reads, writes, grantedMemory, rowCount sql.NullInt64
		var blockingSessionID sql.NullInt64

		if err := rows.Scan(
			&s.SessionID, &s.RequestID, &s.DatabaseName, &s.LoginName,
			&s.HostName, &s.ProgramName, &qhash, &s.QueryText, &waitType,
			&blockingSessionID, &s.Status, &cpuTime, &totalElapsed,
			&reads, &writes, &grantedMemory, &rowCount,
		); err != nil {
			slog.Error("[SQLSERVER] FetchLongRunningQueries Scan Error", "err", err)
			continue
		}

		if waitType.Valid {
			s.WaitType = waitType.String
		}
		if qhash.Valid {
			s.QueryHash = qhash.String
		}
		if cpuTime.Valid {
			s.CPUTimeMs = cpuTime.Int64
		}
		if totalElapsed.Valid {
			s.TotalElapsedTimeMs = totalElapsed.Int64
		}
		if reads.Valid {
			s.Reads = reads.Int64
		}
		if writes.Valid {
			s.Writes = writes.Int64
		}
		if blockingSessionID.Valid {
			s.BlockingSessionID = int(blockingSessionID.Int64)
		}
		if grantedMemory.Valid {
			s.GrantedQueryMemoryMB = int(grantedMemory.Int64)
		}
		if rowCount.Valid {
			s.RowCount = rowCount.Int64
		}

		results = append(results, s)
	}

	return results, rows.Err()
}

var _ time.Time
