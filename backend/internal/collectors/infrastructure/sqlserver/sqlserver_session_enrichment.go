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
	"github.com/rsharma155/sql_optima/internal/collectors/domain"
	ms "github.com/rsharma155/sql_optima/internal/sqlserver"
)

const sessionEnrichmentSQL = `
SELECT
    r.plan_handle,
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
        WHEN s.program_name LIKE '%Telegraf%' THEN 0
        WHEN s.program_name LIKE '%Grafana%' THEN 0
        ELSE 1
    END AS is_user_workload
FROM sys.dm_exec_requests r
JOIN sys.dm_exec_sessions s
    ON r.session_id = s.session_id
WHERE r.plan_handle IS NOT NULL
UNION ALL
SELECT
    qs.plan_handle,
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
  );
`

func (r *SQLServerSnapshotRepository) FetchSessionEnrichment(ctx context.Context) ([]domain.MSSQLSessionEnrichment, error) {
	rows, err := r.db.QueryContext(ctx, sessionEnrichmentSQL)
	if err != nil && ms.IsMSSQLConnError(err) {
		rows, err = r.db.QueryContext(ctx, sessionEnrichmentSQL)
	}
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
			&e.IsUserWorkload,
		)
		if err != nil {
			return nil, err
		}
		enrichments = append(enrichments, e)
	}
	return enrichments, nil
}
