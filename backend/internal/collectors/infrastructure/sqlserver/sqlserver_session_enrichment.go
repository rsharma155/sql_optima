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
        -- monitoring tools
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
WHERE r.plan_handle IS NOT NULL;
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
