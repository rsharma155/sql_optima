// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL index catalog snapshots for Storage & Index Health duplicate/overlap detection (Timescale monitor.index_definitions).
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package collectors

import (
	"context"
	"database/sql"
	"time"

	"github.com/rsharma155/sql_optima/internal/storage/hot"
)

// CollectPostgresIndexDefinitions snapshots index key/include columns for duplicate/overlap analysis (daily cadence).
// Requires PostgreSQL 11+ (uses pg_index.indnkeyattrs for INCLUDE indexes).
func CollectPostgresIndexDefinitions(ctx context.Context, db *sql.DB) ([]IndexDefinitionCatalogRow, error) {
	q := `
		WITH base AS (
			SELECT
				current_database()::text AS db_name,
				n.nspname::text AS schema_name,
				t.relname::text AS table_name,
				t.oid AS tbl_oid,
				ic.relname::text AS index_name,
				i.indkey,
				COALESCE(NULLIF(i.indnkeyattrs::int, 0), cardinality(i.indkey::smallint[])) AS nkey,
				NULLIF(btrim(COALESCE(pg_get_expr(i.indpred, i.indrelid, true), '')), '') AS filter_definition,
				i.indisunique AS is_unique,
				i.indisprimary AS is_pk,
				am.amname::text AS index_type
			FROM pg_index i
			JOIN pg_class t ON t.oid = i.indrelid AND t.relkind = 'r'
			JOIN pg_namespace n ON n.oid = t.relnamespace
			JOIN pg_class ic ON ic.oid = i.indexrelid
			JOIN pg_am am ON am.oid = ic.relam
			WHERE n.nspname NOT IN ('pg_catalog', 'information_schema')
			  AND i.indisvalid
			  AND ic.relname IS NOT NULL
			  AND NOT EXISTS (
				SELECT 1 FROM generate_subscripts(i.indkey, 1) AS g
				WHERE (i.indkey::smallint[])[g] = 0
			  )
		)
		SELECT
			b.db_name,
			b.schema_name,
			b.table_name,
			b.index_name,
			COALESCE(string_agg(a.attname::text, ', ' ORDER BY u.pos) FILTER (WHERE u.pos <= b.nkey), '') AS key_columns,
			COALESCE(string_agg(a.attname::text, ', ' ORDER BY u.pos) FILTER (WHERE u.pos > b.nkey), '') AS include_columns,
			b.filter_definition,
			b.is_unique,
			b.is_pk,
			b.index_type
		FROM base b
		CROSS JOIN LATERAL unnest(b.indkey::smallint[]) WITH ORDINALITY AS u(attnum, pos)
		JOIN pg_attribute a ON a.attrelid = b.tbl_oid AND a.attnum = u.attnum AND NOT a.attisdropped
		WHERE u.attnum > 0
		GROUP BY b.db_name, b.schema_name, b.table_name, b.index_name, b.filter_definition, b.is_unique, b.is_pk, b.index_type, b.nkey
		ORDER BY b.db_name, b.schema_name, b.table_name, b.index_name
	`
	rows, err := db.QueryContext(ctx, q)
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

// PersistPostgresIndexDefinitions writes catalog rows for engine=postgres.
func PersistPostgresIndexDefinitions(ctx context.Context, tl *hot.TimescaleLogger, serverID string, rows []IndexDefinitionCatalogRow, capture time.Time) (inserted int, err error) {
	return persistIndexDefinitions(ctx, tl, "postgres", serverID, rows, capture)
}
