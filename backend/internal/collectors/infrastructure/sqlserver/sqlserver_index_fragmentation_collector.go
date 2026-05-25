// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: SQL Server index fragmentation collector — queries
// sys.dm_db_index_physical_stats (SAMPLED mode) for a given database and
// returns rows where avg_fragmentation >= 5% AND page_count >= 100.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package sqlserver

import (
	"context"
	"database/sql"
	"fmt"
)

// IndexFragmentationRow holds one row from the fragmentation DMV.
type IndexFragmentationRow struct {
	DatabaseName           string
	SchemaName             string
	TableName              string
	IndexName              string
	IndexID                int
	IndexTypeDesc          string
	AvgFragmentationPct    float64
	PageCount              int64
	AvgPageSpaceUsedPct    float64
	RecordCount            int64
	FragmentCount          int64
	AvgFragmentSizePages   float64
}

// IndexFragmentationCollector fetches fragmentation stats for a single database.
type IndexFragmentationCollector struct{}

// Fetch returns index fragmentation rows for the given database.
// Uses SAMPLED mode to limit impact on the monitored server.
func (c *IndexFragmentationCollector) Fetch(ctx context.Context, db *sql.DB, databaseName string) ([]IndexFragmentationRow, error) {
	if databaseName == "" {
		return nil, fmt.Errorf("databaseName is required")
	}
	// Use dynamic SQL to run in the context of the target database.
	query := fmt.Sprintf(`
		/* SQL_OPTIMA */
		SELECT
			DB_NAME(ips.database_id)                           AS database_name,
			ISNULL(s.name, '')                                 AS schema_name,
			ISNULL(o.name, '')                                 AS table_name,
			ISNULL(i.name, '')                                 AS index_name,
			ips.index_id,
			ISNULL(ips.index_type_desc, '')                    AS index_type_desc,
			CAST(ips.avg_fragmentation_in_percent AS DECIMAL(8,2))  AS avg_fragmentation_pct,
			ips.page_count,
			CAST(ISNULL(ips.avg_page_space_used_in_percent, 0) AS DECIMAL(8,2)) AS avg_page_space_used_pct,
			ISNULL(ips.record_count, 0)                        AS record_count,
			ISNULL(ips.fragment_count, 0)                      AS fragment_count,
			CAST(ISNULL(ips.avg_fragment_size_in_pages, 0) AS DECIMAL(8,2)) AS avg_fragment_size_pages
		FROM sys.dm_db_index_physical_stats(
			DB_ID(N'%s'), NULL, NULL, NULL, 'SAMPLED'
		) ips
		INNER JOIN sys.objects o WITH (NOLOCK)
			ON ips.object_id = o.object_id
		INNER JOIN sys.schemas s WITH (NOLOCK)
			ON o.schema_id = s.schema_id
		LEFT JOIN sys.indexes i WITH (NOLOCK)
			ON ips.object_id = i.object_id
			AND ips.index_id  = i.index_id
		WHERE ips.avg_fragmentation_in_percent >= 5.0
		  AND ips.page_count >= 100
		  AND ips.index_id > 0
		ORDER BY ips.avg_fragmentation_in_percent DESC
		OPTION (RECOMPILE);`, databaseName)

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []IndexFragmentationRow
	for rows.Next() {
		var r IndexFragmentationRow
		if err := rows.Scan(
			&r.DatabaseName, &r.SchemaName, &r.TableName, &r.IndexName,
			&r.IndexID, &r.IndexTypeDesc,
			&r.AvgFragmentationPct, &r.PageCount, &r.AvgPageSpaceUsedPct,
			&r.RecordCount, &r.FragmentCount, &r.AvgFragmentSizePages,
		); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
