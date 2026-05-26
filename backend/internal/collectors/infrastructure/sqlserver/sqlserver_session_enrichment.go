// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: SQL Server session enrichment repository.
//          Fetches active session metadata for query attribution.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package sqlserver

import (
	"context"
	"database/sql"

	"github.com/rsharma155/sql_optima/internal/collectors/domain"
)

const sessionEnrichmentSQL = `
SELECT
    r.plan_handle,
    qs.query_hash,
    s.login_name,
    s.program_name AS application_name,
    ISNULL(DB_NAME(r.database_id), 'unknown') AS database_name,
    CASE 
        WHEN s.is_user_process = 0 THEN 0
        WHEN s.session_id <= 50 THEN 0
        WHEN DB_NAME(r.database_id) IN ('master','model','msdb','tempdb') THEN 0
        WHEN s.program_name LIKE '%SQLAgent%' THEN 0
        WHEN s.program_name LIKE '%Monitoring%' THEN 0
        WHEN s.program_name LIKE '%sql-optima%' THEN 0
        WHEN LOWER(s.program_name) IN ('go-mssqldb', 'sqlserverms') THEN 0
        WHEN s.program_name LIKE '%Telegraf%' THEN 0
        WHEN s.program_name LIKE '%Grafana%' THEN 0
        ELSE 1
    END AS is_user_workload
FROM sys.dm_exec_requests r
JOIN sys.dm_exec_sessions s
    ON r.session_id = s.session_id
LEFT JOIN sys.dm_exec_query_stats qs WITH (NOLOCK)
    ON qs.plan_handle = r.plan_handle
WHERE r.plan_handle IS NOT NULL
UNION ALL
SELECT
    qs.plan_handle,
    qs.query_hash,
    COALESCE(active.login_name, 'sql-optima') AS login_name,
    COALESCE(active.application_name, 'sql-optima') AS application_name,
    ISNULL(DB_NAME(COALESCE(active.database_id, pa.dbid)), 'unknown') AS database_name,
    0 AS is_user_workload
FROM sys.dm_exec_query_stats qs WITH (NOLOCK)
CROSS APPLY sys.dm_exec_sql_text(qs.sql_handle) st
OUTER APPLY (
    SELECT CONVERT(INT, value) AS dbid
    FROM sys.dm_exec_plan_attributes(qs.plan_handle)
    WHERE attribute = N'dbid'
) pa
OUTER APPLY (
    SELECT TOP 1
        s.login_name,
        s.program_name AS application_name,
        r.database_id
    FROM sys.dm_exec_requests r WITH (NOLOCK)
    JOIN sys.dm_exec_sessions s WITH (NOLOCK) ON r.session_id = s.session_id
    WHERE r.plan_handle = qs.plan_handle
) active
WHERE st.text LIKE '%/* SQL_OPTIMA%'
  AND NOT EXISTS (
    SELECT 1 FROM sys.dm_exec_requests r2 WITH (NOLOCK)
    WHERE r2.plan_handle = qs.plan_handle
  )
UNION ALL
SELECT
    qs.plan_handle,
    qs.query_hash,
    active.login_name,
    active.application_name,
    ISNULL(DB_NAME(COALESCE(active.database_id, pa.dbid)), 'unknown') AS database_name,
    CASE
        WHEN active.is_user_process = 0 THEN 0
        WHEN active.session_id <= 50 THEN 0
        WHEN DB_NAME(COALESCE(active.database_id, pa.dbid)) IN ('master','model','msdb','tempdb') THEN 0
        WHEN active.program_name LIKE '%SQLAgent%' THEN 0
        WHEN active.program_name LIKE '%Monitoring%' THEN 0
        WHEN active.program_name LIKE '%sql-optima%' THEN 0
        WHEN LOWER(active.program_name) IN ('go-mssqldb', 'sqlserverms') THEN 0
        WHEN active.program_name LIKE '%Telegraf%' THEN 0
        WHEN active.program_name LIKE '%Grafana%' THEN 0
        ELSE 1
    END AS is_user_workload
FROM sys.dm_exec_query_stats qs WITH (NOLOCK)
CROSS APPLY sys.dm_exec_sql_text(qs.sql_handle) st
OUTER APPLY (
    SELECT CONVERT(INT, value) AS dbid
    FROM sys.dm_exec_plan_attributes(qs.plan_handle)
    WHERE attribute = N'dbid'
) pa
OUTER APPLY (
    SELECT TOP 1
        s.login_name,
        s.program_name AS application_name,
        r.database_id,
        s.is_user_process,
        s.session_id,
        s.program_name
    FROM sys.dm_exec_requests r WITH (NOLOCK)
    JOIN sys.dm_exec_sessions s WITH (NOLOCK) ON r.session_id = s.session_id
    WHERE (r.plan_handle = qs.plan_handle OR r.sql_handle = qs.sql_handle)
      AND r.session_id > 50
    ORDER BY CASE WHEN r.plan_handle = qs.plan_handle THEN 0 ELSE 1 END,
             r.start_time DESC
) active
WHERE qs.plan_handle IS NOT NULL
  AND active.login_name IS NOT NULL
  AND st.text NOT LIKE '%/* SQL_OPTIMA%'
  AND NOT EXISTS (
    SELECT 1 FROM sys.dm_exec_requests r2 WITH (NOLOCK)
    WHERE r2.plan_handle = qs.plan_handle
  );
`

func (r *SQLServerSnapshotRepository) FetchSessionEnrichment(ctx context.Context) ([]domain.MSSQLSessionEnrichment, error) {
	return withConnRetry(r, ctx, func(db *sql.DB) ([]domain.MSSQLSessionEnrichment, error) {
		return fetchSessionEnrichmentOnce(ctx, db)
	})
}

func fetchSessionEnrichmentOnce(ctx context.Context, db *sql.DB) ([]domain.MSSQLSessionEnrichment, error) {
	rows, err := db.QueryContext(ctx, sessionEnrichmentSQL)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var enrichments []domain.MSSQLSessionEnrichment
	for rows.Next() {
		var e domain.MSSQLSessionEnrichment
		err := rows.Scan(
			&e.PlanHandle,
			&e.QueryHash,
			&e.LoginName,
			&e.ApplicationName,
			&e.DatabaseName,
			&e.IsUserWorkload,
		)
		if err != nil {
			return nil, err
		}
		enrichments = append(enrichments, e)
	}
	return enrichments, rows.Err()
}
