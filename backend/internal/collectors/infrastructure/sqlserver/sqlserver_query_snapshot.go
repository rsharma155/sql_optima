// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: SQL Server query snapshot repository.
//          Fetches cached execution statistics from DMVs.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package sqlserver

import (
	"context"
	"database/sql"
	"time"

	"github.com/rsharma155/sql_optima/internal/collectors/domain"
	ms "github.com/rsharma155/sql_optima/internal/sqlserver"
)

type SQLServerSnapshotRepository struct {
	db *sql.DB
}

func NewSQLServerSnapshotRepository(db *sql.DB) *SQLServerSnapshotRepository {
	return &SQLServerSnapshotRepository{db: db}
}

const querySnapshotSQL = `
/* SQL_OPTIMA */
SELECT TOP 500
 COALESCE(pa.dbid, st.dbid) AS db_id,
 ISNULL(DB_NAME(COALESCE(pa.dbid, st.dbid)), 'unknown') AS database_name,
 qs.execution_count AS total_executions,
 DATEADD(MINUTE, DATEDIFF(MINUTE, GETDATE(), GETUTCDATE()), qs.last_execution_time) AS last_execution_time,
 qs.total_worker_time/1000 AS total_cpu_ms,
 qs.total_elapsed_time/1000 AS total_elapsed_ms,
 qs.total_logical_reads,
 qs.total_logical_writes,
 qs.total_physical_reads,
 qs.total_rows,
 qs.total_grant_kb,
 qs.max_worker_time/1000 AS max_worker_time,
 qs.max_logical_reads,
 qs.max_dop,
 qs.max_grant_kb,
 qs.max_rows,
 ISNULL(CASE 
  WHEN st.objectid IS NOT NULL
  THEN 'EXEC ' + QUOTENAME(OBJECT_SCHEMA_NAME(st.objectid, COALESCE(pa.dbid, st.dbid)))
       + '.' + QUOTENAME(OBJECT_NAME(st.objectid, COALESCE(pa.dbid, st.dbid)))
  ELSE SUBSTRING(st.text,
        (qs.statement_start_offset/2)+1,
        ((CASE qs.statement_end_offset
          WHEN -1 THEN DATALENGTH(st.text)
          ELSE qs.statement_end_offset END
         - qs.statement_start_offset)/2) + 1)
 END, '') AS statement_text,
 st.text AS query_text_raw,
 qs.query_plan_hash,
 qs.query_hash,
 qs.plan_handle
FROM sys.dm_exec_query_stats qs WITH (NOLOCK)
CROSS APPLY sys.dm_exec_sql_text(qs.sql_handle) st
OUTER APPLY (
 SELECT CONVERT(INT,value) dbid
 FROM sys.dm_exec_plan_attributes(qs.plan_handle)
 WHERE attribute=N'dbid'
) pa
WHERE ISNULL(pa.dbid, st.dbid) > 4
  AND qs.statement_sql_handle IS NOT NULL
  AND DATEADD(MINUTE, DATEDIFF(MINUTE, GETDATE(), GETUTCDATE()), qs.last_execution_time) >= @last_watermark
ORDER BY DATEADD(MINUTE, DATEDIFF(MINUTE, GETDATE(), GETUTCDATE()), qs.last_execution_time) DESC,
         qs.total_worker_time DESC;
`

func (r *SQLServerSnapshotRepository) GetSqlServerStartTime(ctx context.Context) (time.Time, error) {
	var startTime time.Time
	err := r.db.QueryRowContext(ctx, "SELECT sqlserver_start_time FROM sys.dm_os_sys_info").Scan(&startTime)
	if err != nil && ms.IsMSSQLConnError(err) {
		err = r.db.QueryRowContext(ctx, "SELECT sqlserver_start_time FROM sys.dm_os_sys_info").Scan(&startTime)
	}
	return startTime, err
}

func (r *SQLServerSnapshotRepository) FetchSnapshot(ctx context.Context, lastWatermark time.Time) ([]domain.MSSQLQuerySnapshot, error) {
	rows, err := r.db.QueryContext(ctx, querySnapshotSQL, sql.Named("last_watermark", lastWatermark))
	if err != nil && ms.IsMSSQLConnError(err) {
		rows, err = r.db.QueryContext(ctx, querySnapshotSQL, sql.Named("last_watermark", lastWatermark))
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var snapshots []domain.MSSQLQuerySnapshot
	for rows.Next() {
		var s domain.MSSQLQuerySnapshot
		err := rows.Scan(
			&s.DBID,
			&s.DatabaseName,
			&s.TotalExecutions,
			&s.LastExecutionTime,
			&s.TotalCPUMs,
			&s.TotalElapsedMs,
			&s.TotalLogicalReads,
			&s.TotalLogicalWrites,
			&s.TotalPhysicalReads,
			&s.TotalRows,
			&s.TotalGrantKB,
			&s.MaxWorkerTime,
			&s.MaxLogicalReads,
			&s.MaxDOP,
			&s.MaxGrantKB,
			&s.MaxRows,
			&s.StatementText,
			&s.QueryTextRaw,
			&s.QueryPlanHash,
			&s.QueryHash,
			&s.PlanHandle,
		)
		if err != nil {
			return nil, err
		}
		snapshots = append(snapshots, s)
	}
	return snapshots, nil
}
