// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: SQL Server index definition collector; shared IndexDefinitionCatalogRow type is also used by pg_index_definition_collector.go for Timescale monitor.index_definitions.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package collectors

import (
	"context"
	"database/sql"
	"time"

	"github.com/rsharma155/sql_optima/internal/models"
	"github.com/rsharma155/sql_optima/internal/storage/hot"
)

// IndexDefinitionCatalogRow is a normalized index definition snapshot (SQL Server or PostgreSQL catalog).
type IndexDefinitionCatalogRow struct {
	DBName           string
	SchemaName       string
	TableName        string
	IndexName        string
	KeyColumns       string
	IncludeColumns   string
	FilterDefinition sql.NullString
	IsUnique         bool
	IsPK             bool
	IndexType        string
}

// CollectSQLServerIndexDefinitions snapshots index definitions (for duplicate/overlap analysis).
// Should run daily (or on-demand) rather than every 60s.
func CollectSQLServerIndexDefinitions(ctx context.Context, dbq Queryer) ([]IndexDefinitionCatalogRow, error) {
	q := `
		WITH keycols AS (
			SELECT
				i.object_id, i.index_id,
				STUFF((
					SELECT ',' + c.name
					FROM sys.index_columns ic2
					JOIN sys.columns c ON c.object_id = ic2.object_id AND c.column_id = ic2.column_id
					WHERE ic2.object_id = i.object_id AND ic2.index_id = i.index_id
					  AND ic2.is_included_column = 0
					ORDER BY ic2.key_ordinal
					FOR XML PATH(''), TYPE).value('.', 'nvarchar(max)'), 1, 1, '') AS key_columns
			FROM sys.indexes i
		),
		inccols AS (
			SELECT
				i.object_id, i.index_id,
				STUFF((
					SELECT ',' + c.name
					FROM sys.index_columns ic2
					JOIN sys.columns c ON c.object_id = ic2.object_id AND c.column_id = ic2.column_id
					WHERE ic2.object_id = i.object_id AND ic2.index_id = i.index_id
					  AND ic2.is_included_column = 1
					ORDER BY ic2.index_column_id
					FOR XML PATH(''), TYPE).value('.', 'nvarchar(max)'), 1, 1, '') AS include_columns
			FROM sys.indexes i
		)
		SELECT
			DB_NAME() AS db_name,
			OBJECT_SCHEMA_NAME(i.object_id) AS schema_name,
			OBJECT_NAME(i.object_id) AS table_name,
			i.name AS index_name,
			COALESCE(k.key_columns, '') AS key_columns,
			COALESCE(inc.include_columns, '') AS include_columns,
			i.filter_definition,
			CAST(i.is_unique AS bit) AS is_unique,
			CASE WHEN i.is_primary_key = 1 THEN CAST(1 AS bit) ELSE CAST(0 AS bit) END AS is_pk,
			i.type_desc AS index_type
		FROM sys.indexes i
		JOIN sys.tables t ON t.object_id = i.object_id AND t.is_ms_shipped = 0
		LEFT JOIN keycols k ON k.object_id=i.object_id AND k.index_id=i.index_id
		LEFT JOIN inccols inc ON inc.object_id=i.object_id AND inc.index_id=i.index_id
		WHERE i.name IS NOT NULL AND i.index_id > 0
	`

	rows, err := dbq.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []IndexDefinitionCatalogRow
	for rows.Next() {
		var r IndexDefinitionCatalogRow
		if err := rows.Scan(&r.DBName, &r.SchemaName, &r.TableName, &r.IndexName, &r.KeyColumns, &r.IncludeColumns, &r.FilterDefinition, &r.IsUnique, &r.IsPK, &r.IndexType); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func persistIndexDefinitions(ctx context.Context, tl *hot.TimescaleLogger, engine, serverID string, rows []IndexDefinitionCatalogRow, capture time.Time) (inserted int, err error) {
	for _, r := range rows {
		def := models.IndexDefinition{
			Time:             capture.UTC(),
			Engine:           engine,
			ServerID:         serverID,
			DBName:           r.DBName,
			SchemaName:       r.SchemaName,
			TableName:        r.TableName,
			IndexName:        r.IndexName,
			KeyColumns:       r.KeyColumns,
			IncludeColumns:   r.IncludeColumns,
			FilterDefinition: "",
			IsUnique:         r.IsUnique,
			IsPK:             r.IsPK,
			IndexType:        r.IndexType,
		}
		if r.FilterDefinition.Valid {
			def.FilterDefinition = r.FilterDefinition.String
		}
		if err := tl.InsertIndexDefinition(ctx, def); err != nil {
			continue
		}
		inserted++
	}
	return inserted, nil
}

func PersistSQLServerIndexDefinitions(ctx context.Context, tl *hot.TimescaleLogger, serverID string, rows []IndexDefinitionCatalogRow, capture time.Time) (inserted int, err error) {
	return persistIndexDefinitions(ctx, tl, "sqlserver", serverID, rows, capture)
}
