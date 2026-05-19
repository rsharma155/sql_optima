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
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rsharma155/sql_optima/internal/models"
	"github.com/rsharma155/sql_optima/internal/repository"
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
// On the first initial run (since.IsZero()), it collects definitions for all user tables without filtering.
// On subsequent runs, it only collects definitions for tables modified after the specified 'since' time.
func CollectSQLServerIndexDefinitions(ctx context.Context, dbq repository.Queryer, since time.Time) ([]IndexDefinitionCatalogRow, error) {
	var tableFilter string
	var args []interface{}

	// If since is not zero, this is a subsequent run. We check for modified tables
	// to avoid a full scan of the index catalog if nothing has changed.
	if !since.IsZero() {
		modifiedTables, err := GetModifiedTables(ctx, dbq, since)
		if err != nil {
			return nil, fmt.Errorf("failed to check for modified tables: %w", err)
		}

		// If no tables were modified since the last run, we can skip the main collection.
		if len(modifiedTables) == 0 {
			return nil, nil
		}

		// Build a filter for the main query to only fetch definitions for the modified tables.
		placeholders := make([]string, len(modifiedTables))
		for i, t := range modifiedTables {
			placeholders[i] = fmt.Sprintf("@p%d", i+1)
			args = append(args, t)
		}
		tableFilter = fmt.Sprintf(" AND t.name IN (%s)", strings.Join(placeholders, ","))
	}

	// The main query collects detailed index metadata including key and included columns.
	// If tableFilter is empty (e.g. on the first run), it collects for all non-system tables.
	q := ` /* SQL_OPTIMA */ 
		SELECT
			DB_NAME() AS db_name,
			OBJECT_SCHEMA_NAME(i.object_id) AS schema_name,
			OBJECT_NAME(i.object_id) AS table_name,
			i.name AS index_name,
			ISNULL(k.key_columns, '') AS key_columns,
			ISNULL(inc.include_columns, '') AS include_columns,
			i.filter_definition,
			CAST(i.is_unique AS bit) AS is_unique,
			i.is_primary_key AS is_pk, 
			i.type_desc AS index_type
		FROM sys.indexes i
		INNER JOIN sys.tables t 
			ON t.object_id = i.object_id 
			AND t.is_ms_shipped = 0
		OUTER APPLY (
			SELECT STRING_AGG(c.name, ',') WITHIN GROUP (ORDER BY ic.key_ordinal) AS key_columns
			FROM sys.index_columns ic
			INNER JOIN sys.columns c 
				ON c.object_id = ic.object_id 
				AND c.column_id = ic.column_id
			WHERE ic.object_id = i.object_id 
			AND ic.index_id = i.index_id
			AND ic.is_included_column = 0
		) k
		OUTER APPLY (
			SELECT STRING_AGG(c.name, ',') WITHIN GROUP (ORDER BY ic.index_column_id) AS include_columns
			FROM sys.index_columns ic
			INNER JOIN sys.columns c 
				ON c.object_id = ic.object_id 
				AND c.column_id = ic.column_id
			WHERE ic.object_id = i.object_id 
			AND ic.index_id = i.index_id
			AND ic.is_included_column = 1
		) inc
		WHERE i.name IS NOT NULL 
		AND i.index_id > 0 ` + tableFilter + `;`

	rows, err := dbq.QueryContext(ctx, q, args...)
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

// GetModifiedTables returns a list of user tables modified after the specified time.
func GetModifiedTables(ctx context.Context, dbq repository.Queryer, since time.Time) ([]string, error) {
	q := ` /* SQL_OPTIMA */
		SELECT name AS table_name
		FROM sys.objects 
		WHERE type = 'U' 
		  AND is_ms_shipped = 0
		  AND modify_date > @p1;`

	rows, err := dbq.QueryContext(ctx, q, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			continue
		}
		tables = append(tables, name)
	}
	return tables, rows.Err()
}

func persistIndexDefinitions(ctx context.Context, tl *hot.TimescaleLogger, engine string, serverID uuid.UUID, rows []IndexDefinitionCatalogRow, capture time.Time) (inserted int, err error) {
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

func PersistSQLServerIndexDefinitions(ctx context.Context, tl *hot.TimescaleLogger, serverID uuid.UUID, rows []IndexDefinitionCatalogRow, capture time.Time) (inserted int, err error) {
	return persistIndexDefinitions(ctx, tl, "sqlserver", serverID, rows, capture)
}
