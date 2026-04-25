package sqlserver

import (
	"context"
	"database/sql"
	"time"

	"github.com/rsharma155/sql_optima/internal/collectors/domain"
)

type SQLServerSnapshotRepository struct {
	db *sql.DB
}

func NewSQLServerSnapshotRepository(db *sql.DB) *SQLServerSnapshotRepository {
	return &SQLServerSnapshotRepository{db: db}
}

const querySnapshotSQL = `
SELECT /* SQL_OPTIMA */
 COALESCE(pa.dbid, st.dbid) AS db_id,
 DB_NAME(COALESCE(pa.dbid, st.dbid)) AS database_name,
 qs.execution_count AS total_executions,
 qs.last_execution_time,
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
 st.text AS query_text,
 CASE 
  WHEN st.objectid IS NOT NULL
  THEN 'EXEC ' + QUOTENAME(OBJECT_SCHEMA_NAME(st.objectid, COALESCE(pa.dbid, st.dbid)))
       + '.' + QUOTENAME(OBJECT_NAME(st.objectid, COALESCE(pa.dbid, st.dbid)))
  ELSE SUBSTRING(st.text,
        (qs.statement_start_offset/2)+1,
        ((CASE qs.statement_end_offset
          WHEN -1 THEN DATALENGTH(st.text)
          ELSE qs.statement_end_offset END
         - qs.statement_start_offset)/2) + 1)
 END AS statement_text,
 st.text AS query_text_raw,
 qs.query_plan_hash,
 qs.query_hash,
 qs.plan_handle
FROM sys.dm_exec_query_stats qs
CROSS APPLY sys.dm_exec_sql_text(qs.sql_handle) st
OUTER APPLY (
 SELECT CONVERT(INT,value) dbid
 FROM sys.dm_exec_plan_attributes(qs.plan_handle)
 WHERE attribute=N'dbid'
) pa
WHERE pa.dbid > 4
AND pa.dbid < 32761
AND qs.last_execution_time > @last_watermark;
`

const sessionEnrichmentSQL = `
SELECT
 r.plan_handle,
 s.login_name,
 s.program_name AS application_name,
 DB_NAME(r.database_id) AS database_name
FROM sys.dm_exec_requests r
JOIN sys.dm_exec_sessions s
 ON r.session_id = s.session_id
WHERE r.plan_handle IS NOT NULL;
`

func (r *SQLServerSnapshotRepository) GetSqlServerStartTime(ctx context.Context) (time.Time, error) {
	var startTime time.Time
	err := r.db.QueryRowContext(ctx, "SELECT sqlserver_start_time FROM sys.dm_os_sys_info").Scan(&startTime)
	return startTime, err
}

func (r *SQLServerSnapshotRepository) FetchSnapshot(ctx context.Context, lastWatermark time.Time) ([]domain.MSSQLQuerySnapshot, error) {
	rows, err := r.db.QueryContext(ctx, querySnapshotSQL, sql.Named("last_watermark", lastWatermark))
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
			&s.QueryText,
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

func (r *SQLServerSnapshotRepository) FetchSessionEnrichment(ctx context.Context) ([]domain.MSSQLSessionEnrichment, error) {
	rows, err := r.db.QueryContext(ctx, sessionEnrichmentSQL)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var enrichments []domain.MSSQLSessionEnrichment
	for rows.Next() {
		var e domain.MSSQLSessionEnrichment
		err := rows.Scan(
			&e.PlanHandle,
			&e.LoginName,
			&e.ApplicationName,
			&e.DatabaseName,
		)
		if err != nil {
			return nil, err
		}
		enrichments = append(enrichments, e)
	}
	return enrichments, nil
}
