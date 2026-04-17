// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Engine-agnostic Timescale persistence and raw queries for storage index health collectors and APIs.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package hot

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rsharma155/sql_optima/internal/models"
)

type indexUsageStateRow struct {
	SeeksTotal   int64
	ScansTotal   int64
	LookupsTotal int64
	UpdatesTotal int64
}

type tableUsageStateRow struct {
	SeqScansTotal     int64
	IdxScansTotal     int64
	RowsReadTotal     int64
	RowsModifiedTotal int64
}

// GetLastIndexUsageSnapshot fetches the most recent cumulative counters (not delta) for a single index identity.
// It is used by collectors to compute deltas and to survive process restarts.
func (tl *TimescaleLogger) GetLastIndexUsageSnapshot(ctx context.Context, engine, serverID, dbName, schemaName, tableName, indexName string) (*models.IndexUsageStat, error) {
	query := `
		SELECT time, engine, server_id, db_name, schema_name, table_name, index_name,
		       seeks, scans, lookups, updates,
		       COALESCE(index_size_mb, 0)::float8,
		       COALESCE(is_unique, false),
		       COALESCE(is_pk, false),
		       COALESCE(fillfactor, 0)
		FROM monitor.index_usage_stats
		WHERE engine = $1 AND server_id = $2 AND db_name = $3 AND schema_name = $4 AND table_name = $5 AND index_name = $6
		ORDER BY time DESC
		LIMIT 1
	`

	row := tl.pool.QueryRow(ctx, query, engine, serverID, dbName, schemaName, tableName, indexName)
	var out models.IndexUsageStat
	var seeks, scans, lookups, updates sql.NullInt64
	if err := row.Scan(
		&out.Time, &out.Engine, &out.ServerID, &out.DBName, &out.SchemaName, &out.TableName, &out.IndexName,
		&seeks, &scans, &lookups, &updates,
		&out.IndexSizeMB,
		&out.IsUnique, &out.IsPK, &out.FillFactor,
	); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	out.Seeks = seeks.Int64
	out.Scans = scans.Int64
	out.Lookups = lookups.Int64
	out.Updates = updates.Int64
	return &out, nil
}

func (tl *TimescaleLogger) GetIndexUsageState(ctx context.Context, engine, serverID, dbName, schemaName, tableName, indexName string) (*indexUsageStateRow, error) {
	q := `
		SELECT seeks_total, scans_total, lookups_total, updates_total
		FROM monitor.index_usage_state
		WHERE engine = $1 AND server_id = $2 AND db_name = $3 AND schema_name = $4 AND table_name = $5 AND index_name = $6
	`
	row := tl.pool.QueryRow(ctx, q, engine, serverID, dbName, schemaName, tableName, indexName)
	var out indexUsageStateRow
	if err := row.Scan(&out.SeeksTotal, &out.ScansTotal, &out.LookupsTotal, &out.UpdatesTotal); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &out, nil
}

func (tl *TimescaleLogger) UpsertIndexUsageState(ctx context.Context, engine, serverID, dbName, schemaName, tableName, indexName string, seeks, scans, lookups, updates int64) error {
	q := `
		INSERT INTO monitor.index_usage_state (
			engine, server_id, db_name, schema_name, table_name, index_name,
			seeks_total, scans_total, lookups_total, updates_total, last_seen
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,NOW())
		ON CONFLICT (engine, server_id, db_name, schema_name, table_name, index_name)
		DO UPDATE SET
			seeks_total = EXCLUDED.seeks_total,
			scans_total = EXCLUDED.scans_total,
			lookups_total = EXCLUDED.lookups_total,
			updates_total = EXCLUDED.updates_total,
			last_seen = NOW()
	`
	_, err := tl.pool.Exec(ctx, q, engine, serverID, dbName, schemaName, tableName, indexName, seeks, scans, lookups, updates)
	return err
}

func (tl *TimescaleLogger) InsertIndexUsageStat(ctx context.Context, s models.IndexUsageStat) error {
	query := `
		INSERT INTO monitor.index_usage_stats (
			time, engine, server_id, db_name, schema_name, table_name, index_name,
			seeks, scans, lookups, updates, index_size_mb, is_unique, is_pk, fillfactor,
			last_user_seek, last_user_scan, last_user_lookup
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
		ON CONFLICT DO NOTHING
	`
	_, err := tl.pool.Exec(ctx, query,
		s.Time.UTC(), s.Engine, s.ServerID, s.DBName, s.SchemaName, s.TableName, s.IndexName,
		s.Seeks, s.Scans, s.Lookups, s.Updates, s.IndexSizeMB, s.IsUnique, s.IsPK, s.FillFactor,
		s.LastUserSeek, s.LastUserScan, s.LastUserLookup,
	)
	if err != nil {
		log.Printf("[TSLogger] InsertIndexUsageStat failed: %v", err)
	}
	return err
}

func (tl *TimescaleLogger) GetLastTableUsageSnapshot(ctx context.Context, engine, serverID, dbName, schemaName, tableName string) (*models.TableUsageStat, error) {
	query := `
		SELECT time, engine, server_id, db_name, schema_name, table_name,
		       seq_scans, idx_scans, rows_read, rows_modified,
		       COALESCE(table_size_mb, 0)::float8,
		       COALESCE(index_size_mb, 0)::float8,
		       COALESCE(row_count, 0)
		FROM monitor.table_usage_stats
		WHERE engine = $1 AND server_id = $2 AND db_name = $3 AND schema_name = $4 AND table_name = $5
		ORDER BY time DESC
		LIMIT 1
	`

	row := tl.pool.QueryRow(ctx, query, engine, serverID, dbName, schemaName, tableName)
	var out models.TableUsageStat
	var seq, idx, rr, rm sql.NullInt64
	if err := row.Scan(
		&out.Time, &out.Engine, &out.ServerID, &out.DBName, &out.SchemaName, &out.TableName,
		&seq, &idx, &rr, &rm,
		&out.TableSizeMB, &out.IndexSizeMB, &out.RowCount,
	); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	out.SeqScans = seq.Int64
	out.IdxScans = idx.Int64
	out.RowsRead = rr.Int64
	out.RowsModified = rm.Int64
	return &out, nil
}

func (tl *TimescaleLogger) GetTableUsageState(ctx context.Context, engine, serverID, dbName, schemaName, tableName string) (*tableUsageStateRow, error) {
	q := `
		SELECT seq_scans_total, idx_scans_total, rows_read_total, rows_modified_total
		FROM monitor.table_usage_state
		WHERE engine = $1 AND server_id = $2 AND db_name = $3 AND schema_name = $4 AND table_name = $5
	`
	row := tl.pool.QueryRow(ctx, q, engine, serverID, dbName, schemaName, tableName)
	var out tableUsageStateRow
	if err := row.Scan(&out.SeqScansTotal, &out.IdxScansTotal, &out.RowsReadTotal, &out.RowsModifiedTotal); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &out, nil
}

func (tl *TimescaleLogger) UpsertTableUsageState(ctx context.Context, engine, serverID, dbName, schemaName, tableName string, seqScans, idxScans, rowsRead, rowsModified int64) error {
	q := `
		INSERT INTO monitor.table_usage_state (
			engine, server_id, db_name, schema_name, table_name,
			seq_scans_total, idx_scans_total, rows_read_total, rows_modified_total, last_seen
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,NOW())
		ON CONFLICT (engine, server_id, db_name, schema_name, table_name)
		DO UPDATE SET
			seq_scans_total = EXCLUDED.seq_scans_total,
			idx_scans_total = EXCLUDED.idx_scans_total,
			rows_read_total = EXCLUDED.rows_read_total,
			rows_modified_total = EXCLUDED.rows_modified_total,
			last_seen = NOW()
	`
	_, err := tl.pool.Exec(ctx, q, engine, serverID, dbName, schemaName, tableName, seqScans, idxScans, rowsRead, rowsModified)
	return err
}

func (tl *TimescaleLogger) InsertTableUsageStat(ctx context.Context, s models.TableUsageStat) error {
	query := `
		INSERT INTO monitor.table_usage_stats (
			time, engine, server_id, db_name, schema_name, table_name,
			seq_scans, idx_scans, rows_read, rows_modified,
			table_size_mb, index_size_mb, row_count
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		ON CONFLICT DO NOTHING
	`
	_, err := tl.pool.Exec(ctx, query,
		s.Time.UTC(), s.Engine, s.ServerID, s.DBName, s.SchemaName, s.TableName,
		s.SeqScans, s.IdxScans, s.RowsRead, s.RowsModified,
		s.TableSizeMB, s.IndexSizeMB, s.RowCount,
	)
	if err != nil {
		log.Printf("[TSLogger] InsertTableUsageStat failed: %v", err)
	}
	return err
}

func (tl *TimescaleLogger) InsertTableSizeHistory(ctx context.Context, s models.TableSizeHistory) error {
	query := `
		INSERT INTO monitor.table_size_history (
			time, engine, server_id, db_name, schema_name, table_name,
			table_size_mb, index_size_mb, row_count
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT DO NOTHING
	`
	_, err := tl.pool.Exec(ctx, query,
		s.Time.UTC(), s.Engine, s.ServerID, s.DBName, s.SchemaName, s.TableName,
		s.TableSizeMB, s.IndexSizeMB, s.RowCount,
	)
	if err != nil {
		log.Printf("[TSLogger] InsertTableSizeHistory failed: %v", err)
	}
	return err
}

// QueryStorageIndexHealthIndexUsage returns raw points for charts/grids (UI derives).
func (tl *TimescaleLogger) QueryStorageIndexHealthIndexUsage(ctx context.Context, engine, serverID, from, to string, limit int) ([]models.IndexUsageStat, error) {
	if limit <= 0 {
		limit = 500
	}
	if limit > 5000 {
		limit = 5000
	}
	q := `
		SELECT time, engine, server_id, db_name, schema_name, table_name, index_name,
		       COALESCE(seeks,0), COALESCE(scans,0), COALESCE(lookups,0), COALESCE(updates,0),
		       COALESCE(index_size_mb,0)::float8,
		       COALESCE(is_unique,false), COALESCE(is_pk,false), COALESCE(fillfactor,0)
		FROM monitor.index_usage_stats
		WHERE engine = $1 AND server_id = $2
		  AND time >= $3::timestamptz AND time <= $4::timestamptz
		ORDER BY time DESC
		LIMIT $5
	`
	rows, err := tl.pool.Query(ctx, q, engine, serverID, from, to, limit)
	if err != nil {
		if isMissingRelation(err) {
			return nil, schemaMissingErr("monitor.index_usage_stats")
		}
		return nil, err
	}
	defer rows.Close()

	var out []models.IndexUsageStat
	for rows.Next() {
		var r models.IndexUsageStat
		if err := rows.Scan(&r.Time, &r.Engine, &r.ServerID, &r.DBName, &r.SchemaName, &r.TableName, &r.IndexName,
			&r.Seeks, &r.Scans, &r.Lookups, &r.Updates, &r.IndexSizeMB, &r.IsUnique, &r.IsPK, &r.FillFactor); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (tl *TimescaleLogger) QueryStorageIndexHealthTableUsage(ctx context.Context, engine, serverID, from, to string, limit int) ([]models.TableUsageStat, error) {
	if limit <= 0 {
		limit = 500
	}
	if limit > 5000 {
		limit = 5000
	}
	q := `
		SELECT time, engine, server_id, db_name, schema_name, table_name,
		       COALESCE(seq_scans,0), COALESCE(idx_scans,0), COALESCE(rows_read,0), COALESCE(rows_modified,0),
		       COALESCE(table_size_mb,0)::float8, COALESCE(index_size_mb,0)::float8, COALESCE(row_count,0)
		FROM monitor.table_usage_stats
		WHERE engine = $1 AND server_id = $2
		  AND time >= $3::timestamptz AND time <= $4::timestamptz
		ORDER BY time DESC
		LIMIT $5
	`
	rows, err := tl.pool.Query(ctx, q, engine, serverID, from, to, limit)
	if err != nil {
		if isMissingRelation(err) {
			return nil, schemaMissingErr("monitor.table_usage_stats")
		}
		return nil, err
	}
	defer rows.Close()

	var out []models.TableUsageStat
	for rows.Next() {
		var r models.TableUsageStat
		if err := rows.Scan(&r.Time, &r.Engine, &r.ServerID, &r.DBName, &r.SchemaName, &r.TableName,
			&r.SeqScans, &r.IdxScans, &r.RowsRead, &r.RowsModified, &r.TableSizeMB, &r.IndexSizeMB, &r.RowCount); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (tl *TimescaleLogger) QueryStorageIndexHealthTableGrowth(ctx context.Context, engine, serverID string, from, to string, limit int) ([]models.TableSizeHistory, error) {
	if limit <= 0 {
		limit = 500
	}
	if limit > 5000 {
		limit = 5000
	}
	q := `
		SELECT time, engine, server_id, db_name, schema_name, table_name,
		       COALESCE(table_size_mb,0)::float8, COALESCE(index_size_mb,0)::float8, COALESCE(row_count,0)
		FROM monitor.table_size_history
		WHERE engine = $1 AND server_id = $2
		  AND time >= $3::timestamptz AND time <= $4::timestamptz
		ORDER BY time DESC
		LIMIT $5
	`
	rows, err := tl.pool.Query(ctx, q, engine, serverID, from, to, limit)
	if err != nil {
		if isMissingRelation(err) {
			return nil, schemaMissingErr("monitor.table_size_history")
		}
		return nil, err
	}
	defer rows.Close()

	var out []models.TableSizeHistory
	for rows.Next() {
		var r models.TableSizeHistory
		if err := rows.Scan(&r.Time, &r.Engine, &r.ServerID, &r.DBName, &r.SchemaName, &r.TableName,
			&r.TableSizeMB, &r.IndexSizeMB, &r.RowCount); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (tl *TimescaleLogger) InsertIndexDefinition(ctx context.Context, d models.IndexDefinition) error {
	q := `
		INSERT INTO monitor.index_definitions (
			time, engine, server_id, db_name, schema_name, table_name, index_name,
			key_columns, include_columns, filter_definition, is_unique, is_pk, index_type
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		ON CONFLICT DO NOTHING
	`
	_, err := tl.pool.Exec(ctx, q,
		d.Time.UTC(), d.Engine, d.ServerID, d.DBName, d.SchemaName, d.TableName, d.IndexName,
		d.KeyColumns, d.IncludeColumns, d.FilterDefinition, d.IsUnique, d.IsPK, d.IndexType,
	)
	return err
}

// IndexDefinitionRow is returned by QueryIndexDefinition.
type IndexDefinitionRow struct {
	DBName           string
	SchemaName       string
	TableName        string
	IndexName        string
	KeyColumns       string
	IncludeColumns   string
	FilterDefinition string
	IsUnique         bool
	IsPK             bool
	IndexType        string
}

// QueryIndexDefinition returns the latest stored definitions from monitor.index_definitions
// for the given engine and serverID. The dbName, schemaName, and indexName parameters are
// optional filters; when none are provided the query returns all definitions for the instance.
func (tl *TimescaleLogger) QueryIndexDefinition(ctx context.Context, engine, serverID, dbName, schemaName, indexName string) ([]IndexDefinitionRow, error) {
	if engine == "" || serverID == "" {
		return nil, fmt.Errorf("engine and serverID are required")
	}
	args := []interface{}{engine, serverID}
	clauses := []string{"engine=$1", "server_id=$2"}
	if dbName != "" {
		args = append(args, dbName)
		clauses = append(clauses, fmt.Sprintf("db_name=$%d", len(args)))
	}
	if schemaName != "" {
		args = append(args, schemaName)
		clauses = append(clauses, fmt.Sprintf("schema_name=$%d", len(args)))
	}
	if indexName != "" {
		args = append(args, indexName)
		clauses = append(clauses, fmt.Sprintf("index_name=$%d", len(args)))
	}
	q := fmt.Sprintf(`
		SELECT DISTINCT ON (db_name, schema_name, index_name)
			db_name, schema_name, table_name, index_name,
			COALESCE(key_columns,'') AS key_columns,
			COALESCE(include_columns,'') AS include_columns,
			COALESCE(filter_definition,'') AS filter_definition,
			COALESCE(is_unique, false) AS is_unique,
			COALESCE(is_pk, false) AS is_pk,
			COALESCE(index_type,'') AS index_type
		FROM monitor.index_definitions
		WHERE %s
		ORDER BY db_name, schema_name, index_name, time DESC
		LIMIT 100
	`, strings.Join(clauses, " AND "))
	rows, err := tl.pool.Query(ctx, q, args...)
	if err != nil {
		if isMissingRelation(err) {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []IndexDefinitionRow
	for rows.Next() {
		var r IndexDefinitionRow
		if err := rows.Scan(&r.DBName, &r.SchemaName, &r.TableName, &r.IndexName,
			&r.KeyColumns, &r.IncludeColumns, &r.FilterDefinition, &r.IsUnique, &r.IsPK, &r.IndexType); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (tl *TimescaleLogger) QueryStorageIndexHealthFilterOptions(ctx context.Context, engine, serverID, from, to string, dbName, schemaName string) (*SIHFilterOptions, error) {
	baseWhereTime := `engine=$1 AND server_id=$2 AND time >= $3::timestamptz AND time <= $4::timestamptz`
	baseArgsTime := []interface{}{engine, serverID, from, to}
	baseWhereAny := `engine=$1 AND server_id=$2`
	baseArgsAny := []interface{}{engine, serverID}

	countIn := func(rel string) (int64, error) {
		q := `SELECT COUNT(*)::bigint FROM ` + rel + ` WHERE ` + baseWhereTime
		var n int64
		err := tl.pool.QueryRow(ctx, q, baseArgsTime...).Scan(&n)
		if err != nil {
			if isMissingRelation(err) {
				return 0, nil
			}
			return 0, err
		}
		return n, nil
	}

	var counts SIHFilterSourceRowCounts
	var err error
	if counts.TableSizeHistory, err = countIn("monitor.table_size_history"); err != nil {
		return nil, err
	}
	if counts.TableUsageStats, err = countIn("monitor.table_usage_stats"); err != nil {
		return nil, err
	}
	if counts.IndexUsageStats, err = countIn("monitor.index_usage_stats"); err != nil {
		return nil, err
	}

	// Distinct values across all SIH hypertables so filters populate even if only index_usage
	// or table_usage has rows (e.g. before first 6h growth snapshot).
	queryDistinctUnion := func(col, whereSQL string, args []interface{}) ([]string, error) {
		q := fmt.Sprintf(`
			SELECT DISTINCT v FROM (
				SELECT %s::text AS v FROM monitor.table_size_history WHERE %s
				UNION
				SELECT %s::text AS v FROM monitor.table_usage_stats WHERE %s
				UNION
				SELECT %s::text AS v FROM monitor.index_usage_stats WHERE %s
			) x
			WHERE v IS NOT NULL AND btrim(v) <> ''
			ORDER BY v
			LIMIT 1000
		`, col, whereSQL, col, whereSQL, col, whereSQL)
		rows, err := tl.pool.Query(ctx, q, args...)
		if err != nil {
			if isMissingRelation(err) {
				return nil, schemaMissingErr("monitor.table_size_history")
			}
			return nil, err
		}
		defer rows.Close()
		out := make([]string, 0, 64)
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err == nil && strings.TrimSpace(s) != "" {
				out = append(out, s)
			}
		}
		return out, rows.Err()
	}

	dbs, err := queryDistinctUnion("db_name", baseWhereTime, baseArgsTime)
	if err != nil {
		return nil, err
	}
	if len(dbs) == 0 {
		dbs, err = queryDistinctUnion("db_name", baseWhereAny, baseArgsAny)
		if err != nil {
			return nil, err
		}
	}

	whereSchemasTime := baseWhereTime
	argsSchemasTime := append([]interface{}{}, baseArgsTime...)
	if strings.TrimSpace(dbName) != "" {
		whereSchemasTime += " AND db_name = $5"
		argsSchemasTime = append(argsSchemasTime, dbName)
	}
	schemas, err := queryDistinctUnion("schema_name", whereSchemasTime, argsSchemasTime)
	if err != nil {
		return nil, err
	}
	if len(schemas) == 0 {
		whereSchemasAny := baseWhereAny
		argsSchemasAny := append([]interface{}{}, baseArgsAny...)
		if strings.TrimSpace(dbName) != "" {
			whereSchemasAny += " AND db_name = $3"
			argsSchemasAny = append(argsSchemasAny, dbName)
		}
		schemas, err = queryDistinctUnion("schema_name", whereSchemasAny, argsSchemasAny)
		if err != nil {
			return nil, err
		}
	}

	whereTablesTime := baseWhereTime
	argsTablesTime := append([]interface{}{}, baseArgsTime...)
	n := 5
	if strings.TrimSpace(dbName) != "" {
		whereTablesTime += " AND db_name = $" + fmt.Sprint(n)
		argsTablesTime = append(argsTablesTime, dbName)
		n++
	}
	if strings.TrimSpace(schemaName) != "" {
		whereTablesTime += " AND schema_name = $" + fmt.Sprint(n)
		argsTablesTime = append(argsTablesTime, schemaName)
	}
	tables, err := queryDistinctUnion("table_name", whereTablesTime, argsTablesTime)
	if err != nil {
		return nil, err
	}
	if len(tables) == 0 {
		whereTablesAny := baseWhereAny
		argsTablesAny := append([]interface{}{}, baseArgsAny...)
		n2 := 3
		if strings.TrimSpace(dbName) != "" {
			whereTablesAny += " AND db_name = $" + fmt.Sprint(n2)
			argsTablesAny = append(argsTablesAny, dbName)
			n2++
		}
		if strings.TrimSpace(schemaName) != "" {
			whereTablesAny += " AND schema_name = $" + fmt.Sprint(n2)
			argsTablesAny = append(argsTablesAny, schemaName)
		}
		tables, err = queryDistinctUnion("table_name", whereTablesAny, argsTablesAny)
		if err != nil {
			return nil, err
		}
	}

	if dbs == nil {
		dbs = []string{}
	}
	if schemas == nil {
		schemas = []string{}
	}
	if tables == nil {
		tables = []string{}
	}
	return &SIHFilterOptions{Databases: dbs, Schemas: schemas, Tables: tables, SourceRowCounts: counts}, nil
}

// RefreshIndexUnusedCandidatesDaily stores the top unused-index candidates for alerts/UI.
// It replaces rows for (run_at UTC midnight of analysis day, engine, server_id).
// Criteria: zero reads in the prior 24h window ending at analysisEnd, updates >= minUpdates, not PK.
func (tl *TimescaleLogger) RefreshIndexUnusedCandidatesDaily(ctx context.Context, engine, serverID string, analysisEnd time.Time, minUpdates int64) (int64, error) {
	if minUpdates <= 0 {
		minUpdates = 100
	}
	u := analysisEnd.UTC()
	runAt := time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
	end := u

	tx, err := tl.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	del := `DELETE FROM monitor.index_unused_candidates_daily WHERE run_at = $1 AND engine = $2 AND server_id = $3`
	if _, err := tx.Exec(ctx, del, runAt, engine, serverID); err != nil {
		if isMissingRelation(err) {
			return 0, schemaMissingErr("monitor.index_unused_candidates_daily")
		}
		return 0, err
	}

	ins := `
		INSERT INTO monitor.index_unused_candidates_daily (
			run_at, engine, server_id, db_name, schema_name, table_name, index_name,
			updates_24h, index_size_mb, last_user_seek, rank
		)
		SELECT $1::timestamptz, $2, $3, x.db_name, x.schema_name, x.table_name, x.index_name,
		       x.upd, x.size_mb, x.last_user_seek, x.rnk::smallint
		FROM (
			SELECT
				w.db_name, w.schema_name, w.table_name, w.index_name,
				w.upd::bigint AS upd,
				l.size_mb,
				l.last_user_seek,
				ROW_NUMBER() OVER (ORDER BY l.size_mb DESC NULLS LAST, w.upd DESC) AS rnk
			FROM (
				SELECT db_name, schema_name, table_name, index_name,
				       SUM(COALESCE(updates,0)) AS upd,
				       SUM(COALESCE(seeks,0)+COALESCE(scans,0)+COALESCE(lookups,0)) AS reads
				FROM monitor.index_usage_stats
				WHERE engine = $2 AND server_id = $3
				  AND time > ($4::timestamptz - INTERVAL '24 hours')
				  AND time <= $4::timestamptz
				GROUP BY db_name, schema_name, table_name, index_name
			) w
			JOIN (
				SELECT DISTINCT ON (db_name, schema_name, table_name, index_name)
					db_name, schema_name, table_name, index_name,
					COALESCE(index_size_mb,0)::float8 AS size_mb,
					last_user_seek,
					COALESCE(is_pk,false) AS is_pk
				FROM monitor.index_usage_stats
				WHERE engine = $2 AND server_id = $3 AND time <= $4::timestamptz
				ORDER BY db_name, schema_name, table_name, index_name, time DESC
			) l USING (db_name, schema_name, table_name, index_name)
			WHERE w.reads = 0 AND l.is_pk = false
			  AND (
			    ($2::text = 'sqlserver' AND w.upd >= $5)
			    OR ($2::text = 'postgres' AND COALESCE(l.size_mb,0) >= 0.01)
			  )
		) x
		WHERE x.rnk <= 100
	`
	tag, err := tx.Exec(ctx, ins, runAt, engine, serverID, end, minUpdates)
	if err != nil {
		if isMissingRelation(err) {
			return 0, schemaMissingErr("monitor.index_unused_candidates_daily")
		}
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
