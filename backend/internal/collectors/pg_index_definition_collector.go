// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Collector for PostgreSQL index definitions to support index health analysis.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package collectors

import (
	"fmt"
	"log/slog"
	"context"
	"database/sql"
	"strings"

	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/repository"
	"github.com/rsharma155/sql_optima/internal/storage/hot"
)

type PgIndexDefinitionCollector struct {
	pgRepo   *repository.PgRepository
	tsLogger *hot.TimescaleLogger
}

func NewPgIndexDefinitionCollector(pgRepo *repository.PgRepository, tsLogger *hot.TimescaleLogger) *PgIndexDefinitionCollector {
	return &PgIndexDefinitionCollector{
		pgRepo:   pgRepo,
		tsLogger: tsLogger,
	}
}

func (c *PgIndexDefinitionCollector) Collect(ctx context.Context, inst config.Instance) error {
	instanceName := inst.Name
	serverID := inst.ServerID

	// 1. Fetch all databases
	dbs, err := c.pgRepo.FetchDatabases(ctx, instanceName)
	if err != nil {
		slog.Error("[PgIndexDefinitionCollector] Failed to fetch databases", "target", instanceName, "err", err)
		return err
	}

	for _, dbName := range dbs {
		// 2. Connect to database
		db, err := c.pgRepo.GetConnForDB(instanceName, dbName)
		if err != nil {
			slog.Error(fmt.Sprintf("[PgIndexDefinitionCollector] Failed to connect to %s/%s: %v", instanceName, dbName, err))
			continue
		}

		// 3. Fetch index definitions
		rows, err := c.fetchIndexDefinitions(ctx, db, dbName)
		if err != nil {
			slog.Error(fmt.Sprintf("[PgIndexDefinitionCollector] Failed to fetch index definitions for %s/%s: %v", instanceName, dbName, err))
			continue
		}

		if len(rows) == 0 {
			continue
		}

		// 4. Map to hot.IndexDefinitionCatalogRow
		var hotRows []hot.IndexDefinitionCatalogRow
		for _, r := range rows {
			hotRows = append(hotRows, hot.IndexDefinitionCatalogRow{
				DBName:         r.DBName,
				SchemaName:     r.SchemaName,
				TableName:      r.TableName,
				IndexName:      r.IndexName,
				KeyColumns:     r.KeyColumns,
				IncludeColumns: r.IncludeColumns,
				FilterDefinition: struct{ String string }{
					String: r.FilterDefinition.String,
				},
				IsUnique:  r.IsUnique,
				IsPK:      r.IsPK,
				IndexType: r.IndexType,
			})
		}

		// 5. Dedup and Store
		sig := c.tsLogger.FingerprintIndexDefinitionRows(serverID, hotRows)
		if c.tsLogger.EnterpriseSnapshotUnchanged(serverID, "pg_index_definitions", sig, dbName) {
			continue
		}

		if err := c.tsLogger.LogPgIndexDefinitions(ctx, serverID, hotRows); err != nil {
			slog.Error(fmt.Sprintf("[PgIndexDefinitionCollector] Failed to store index definitions for %s/%s: %v", instanceName, dbName, err))
			continue
		}

		c.tsLogger.RememberEnterpriseSnapshot(serverID, "pg_index_definitions", sig, dbName)
	}

	return nil
}

type pgIndexRow struct {
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

func (c *PgIndexDefinitionCollector) fetchIndexDefinitions(ctx context.Context, db *sql.DB, dbName string) ([]pgIndexRow, error) {
	query := `
		SELECT 
			$1 as db_name,
			schemaname,
			tablename,
			indexname,
			indexdef,
			CASE WHEN indexdef LIKE '%UNIQUE%' THEN true ELSE false END as is_unique,
			CASE WHEN indexdef LIKE '%PRIMARY KEY%' THEN true ELSE false END as is_pk
		FROM pg_indexes
		WHERE schemaname NOT IN ('pg_catalog', 'information_schema')`

	rows, err := db.QueryContext(ctx, query, dbName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []pgIndexRow
	for rows.Next() {
		var r pgIndexRow
		var def string
		if err := rows.Scan(&r.DBName, &r.SchemaName, &r.TableName, &r.IndexName, &def, &r.IsUnique, &r.IsPK); err != nil {
			continue
		}
		// Basic parsing of indexdef
		r.IndexType = "btree" // default
		if strings.Contains(strings.ToUpper(def), " USING GIN ") {
			r.IndexType = "gin"
		} else if strings.Contains(strings.ToUpper(def), " USING GIST ") {
			r.IndexType = "gist"
		} else if strings.Contains(strings.ToUpper(def), " USING BRIN ") {
			r.IndexType = "brin"
		} else if strings.Contains(strings.ToUpper(def), " USING HASH ") {
			r.IndexType = "hash"
		}

		// Very basic column extraction from indexdef
		// indexdef: CREATE [UNIQUE] INDEX name ON schema.table USING type (col1, col2) [INCLUDE (col3)] [WHERE filter]
		parts := strings.Split(def, "(")
		if len(parts) >= 2 {
			colPart := strings.Split(parts[1], ")")[0]
			r.KeyColumns = colPart
		}

		if strings.Contains(strings.ToUpper(def), " INCLUDE ") {
			incParts := strings.Split(def, "INCLUDE (")
			if len(incParts) >= 2 {
				incColPart := strings.Split(incParts[1], ")")[0]
				r.IncludeColumns = incColPart
			}
		}

		if strings.Contains(strings.ToUpper(def), " WHERE ") {
			whereParts := strings.Split(def, " WHERE ")
			if len(whereParts) >= 2 {
				r.FilterDefinition.String = whereParts[1]
				r.FilterDefinition.Valid = true
			}
		}

		results = append(results, r)
	}
	return results, rows.Err()
}
