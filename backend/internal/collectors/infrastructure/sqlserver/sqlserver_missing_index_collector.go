// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: SQL Server missing index collector — queries dm_db_missing_index_*
// DMVs and returns top indexes by improvement_score for user databases.
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

// MissingIndexRow holds one row from the missing index advisor DMVs.
type MissingIndexRow struct {
	DatabaseName      string
	SchemaName        string
	TableName         string
	EqualityColumns   string
	InequalityColumns string
	IncludedColumns   string
	UserSeeks         int64
	UserScans         int64
	AvgTotalUserCost  float64
	AvgUserImpact     float64
	ImprovementScore  float64
	LastUserSeek      *string
	LastUserScan      *string
}

// MissingIndexCollector fetches missing index recommendations.
type MissingIndexCollector struct{}

// Fetch returns top missing indexes ordered by improvement_score DESC.
// limit controls maximum rows returned (default 100 if <= 0).
func (c *MissingIndexCollector) Fetch(ctx context.Context, db *sql.DB, limit int) ([]MissingIndexRow, error) {
	if limit <= 0 {
		limit = 100
	}
	query := fmt.Sprintf(`
		/* SQL_OPTIMA */
		SELECT TOP(%d)
			DB_NAME(mid.database_id)                                 AS database_name,
			ISNULL(mid.statement, '')                                AS full_table,
			ISNULL(mid.equality_columns, '')                         AS equality_columns,
			ISNULL(mid.inequality_columns, '')                       AS inequality_columns,
			ISNULL(mid.included_columns, '')                         AS included_columns,
			ISNULL(migs.user_seeks, 0)                               AS user_seeks,
			ISNULL(migs.user_scans, 0)                               AS user_scans,
			ISNULL(migs.avg_total_user_cost, 0)                      AS avg_total_user_cost,
			ISNULL(migs.avg_user_impact, 0)                          AS avg_user_impact,
			CAST(ISNULL(migs.avg_total_user_cost, 0) *
				ISNULL(migs.avg_user_impact, 0) *
				(ISNULL(migs.user_seeks, 0) + ISNULL(migs.user_scans, 0)) AS DECIMAL(19,4)) AS improvement_score,
			CONVERT(VARCHAR(30), migs.last_user_seek, 126)           AS last_user_seek,
			CONVERT(VARCHAR(30), migs.last_user_scan, 126)           AS last_user_scan
		FROM sys.dm_db_missing_index_groups mig WITH (NOLOCK)
		INNER JOIN sys.dm_db_missing_index_group_stats migs WITH (NOLOCK)
			ON mig.index_group_handle = migs.group_handle
		INNER JOIN sys.dm_db_missing_index_details mid WITH (NOLOCK)
			ON mig.index_handle = mid.index_handle
		WHERE mid.database_id > 4
		  AND migs.user_seeks >= 500
		  AND (migs.avg_total_user_cost * migs.avg_user_impact * (migs.user_seeks + migs.user_scans)) >= 5000
		ORDER BY improvement_score DESC
		OPTION (RECOMPILE);`, limit)

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []MissingIndexRow
	for rows.Next() {
		var r MissingIndexRow
		var fullTable string
		var lastSeek, lastScan sql.NullString
		if err := rows.Scan(
			&r.DatabaseName, &fullTable,
			&r.EqualityColumns, &r.InequalityColumns, &r.IncludedColumns,
			&r.UserSeeks, &r.UserScans,
			&r.AvgTotalUserCost, &r.AvgUserImpact, &r.ImprovementScore,
			&lastSeek, &lastScan,
		); err != nil {
			continue
		}
		r.SchemaName, r.TableName = parseMissingIndexSchemaTable(fullTable)
		if lastSeek.Valid {
			r.LastUserSeek = &lastSeek.String
		}
		if lastScan.Valid {
			r.LastUserScan = &lastScan.String
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// parseMissingIndexSchemaTable splits "[db].[schema].[table]" into schema and table parts.
func parseMissingIndexSchemaTable(full string) (schema, table string) {
	parts := splitBracketedDotParts(full)
	switch len(parts) {
	case 0:
		return "", ""
	case 1:
		return "", parts[0]
	default:
		return parts[len(parts)-2], parts[len(parts)-1]
	}
}

func splitBracketedDotParts(s string) []string {
	var parts []string
	var cur []byte
	inBracket := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '[':
			inBracket = true
		case c == ']':
			inBracket = false
		case c == '.' && !inBracket:
			parts = append(parts, string(cur))
			cur = cur[:0]
		default:
			cur = append(cur, c)
		}
	}
	if len(cur) > 0 {
		parts = append(parts, string(cur))
	}
	return parts
}
