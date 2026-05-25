// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Engine-agnostic Timescale persistence and raw queries for storage index health collectors and APIs.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package hot

import (
	"log/slog"
	"context"
	"database/sql"
	"fmt"
	"github.com/google/uuid"
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
	IndexSizeMB  float64
}

type tableUsageStateRow struct {
	SeqScansTotal     int64
	IdxScansTotal     int64
	RowsReadTotal     int64
	RowsModifiedTotal int64
	TableSizeMB       float64
	IndexSizeMB       float64
	RowCount          int64
}

// GetLastIndexUsageSnapshot fetches the most recent cumulative counters (not delta) for a single index identity.
func (tl *TimescaleLogger) GetLastIndexUsageSnapshot(ctx context.Context, engine, serverID, dbName, schemaName, tableName, indexName string) (*models.IndexUsageStat, error) {
	query := `
		SELECT capture_timestamp, engine, server_id, db_name, schema_name, table_name, index_name,
		       seeks, scans, lookups, updates,
		       COALESCE(index_size_mb, 0)::float8,
		       COALESCE(is_unique, false),
		       COALESCE(is_pk, false),
		       COALESCE(fillfactor, 0)
		FROM monitor.index_usage_stats
		WHERE engine = $1 AND server_id = $2::uuid AND db_name = $3 AND schema_name = $4 AND table_name = $5 AND index_name = $6
		ORDER BY capture_timestamp DESC
		LIMIT 1
	`

	row := tl.pool.QueryRow(ctx, query, engine, serverID, dbName, schemaName, tableName, indexName)
	var out models.IndexUsageStat
	var seeks, scans, lookups, updates sql.NullInt64
	if err := row.Scan(
		&out.Timestamp, &out.Engine, &out.ServerID, &out.DBName, &out.SchemaName, &out.TableName, &out.IndexName,
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
		SELECT seeks_total, scans_total, lookups_total, updates_total, COALESCE(index_size_mb, 0)::float8
		FROM monitor.index_usage_state
		WHERE engine = $1 AND server_id = $2::uuid AND db_name = $3 AND schema_name = $4 AND table_name = $5 AND index_name = $6
	`
	row := tl.pool.QueryRow(ctx, q, engine, serverID, dbName, schemaName, tableName, indexName)
	var out indexUsageStateRow
	if err := row.Scan(&out.SeeksTotal, &out.ScansTotal, &out.LookupsTotal, &out.UpdatesTotal, &out.IndexSizeMB); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &out, nil
}

func (tl *TimescaleLogger) UpsertIndexUsageState(ctx context.Context, engine, serverID, dbName, schemaName, tableName, indexName string, seeks, scans, lookups, updates int64, sizeMB float64) error {
	q := `
		INSERT INTO monitor.index_usage_state (
			engine, server_id, db_name, schema_name, table_name, index_name,
			seeks_total, scans_total, lookups_total, updates_total, index_size_mb, last_seen
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,NOW())
		ON CONFLICT (engine, server_id, db_name, schema_name, table_name, index_name)
		DO UPDATE SET
			seeks_total = EXCLUDED.seeks_total,
			scans_total = EXCLUDED.scans_total,
			lookups_total = EXCLUDED.lookups_total,
			updates_total = EXCLUDED.updates_total,
			index_size_mb = EXCLUDED.index_size_mb,
			last_seen = NOW()
	`
	_, err := tl.pool.Exec(ctx, q, engine, serverID, dbName, schemaName, tableName, indexName, seeks, scans, lookups, updates, sizeMB)
	return err
}

func (tl *TimescaleLogger) InsertIndexUsageStat(ctx context.Context, s models.IndexUsageStat) error {
	query := `
		INSERT INTO monitor.index_usage_stats (
			capture_timestamp, engine, server_id, db_name, schema_name, table_name, index_name,
			seeks, scans, lookups, updates, index_size_mb, is_unique, is_pk, fillfactor,
			last_user_seek, last_user_scan, last_user_lookup
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
		ON CONFLICT DO NOTHING
	`
	_, err := tl.pool.Exec(ctx, query,
		s.Timestamp.UTC(), s.Engine, s.ServerID, s.DBName, s.SchemaName, s.TableName, s.IndexName,
		s.Seeks, s.Scans, s.Lookups, s.Updates, s.IndexSizeMB, s.IsUnique, s.IsPK, s.FillFactor,
		s.LastUserSeek, s.LastUserScan, s.LastUserLookup,
	)
	if err != nil {
		slog.Error("[TSLogger] InsertIndexUsageStat failed", "err", err)
	}
	return err
}

func (tl *TimescaleLogger) GetLastTableUsageSnapshot(ctx context.Context, engine, serverID, dbName, schemaName, tableName string) (*models.TableUsageStat, error) {
	query := `
		SELECT capture_timestamp, engine, server_id, db_name, schema_name, table_name,
		       seq_scans, idx_scans, rows_read, rows_modified,
		       COALESCE(table_size_mb, 0)::float8,
		       COALESCE(index_size_mb, 0)::float8,
		       COALESCE(row_count, 0)
		FROM monitor.table_usage_stats
		WHERE engine = $1 AND server_id = $2::uuid AND db_name = $3 AND schema_name = $4 AND table_name = $5
		ORDER BY capture_timestamp DESC
		LIMIT 1
	`

	row := tl.pool.QueryRow(ctx, query, engine, serverID, dbName, schemaName, tableName)
	var out models.TableUsageStat
	var seq, idx, rr, rm sql.NullInt64
	if err := row.Scan(
		&out.Timestamp, &out.Engine, &out.ServerID, &out.DBName, &out.SchemaName, &out.TableName,
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
		SELECT seq_scans_total, idx_scans_total, rows_read_total, rows_modified_total, 
		       COALESCE(table_size_mb, 0)::float8, COALESCE(index_size_mb, 0)::float8, COALESCE(row_count, 0)
		FROM monitor.table_usage_state
		WHERE engine = $1 AND server_id = $2::uuid AND db_name = $3 AND schema_name = $4 AND table_name = $5
	`
	row := tl.pool.QueryRow(ctx, q, engine, serverID, dbName, schemaName, tableName)
	var out tableUsageStateRow
	if err := row.Scan(&out.SeqScansTotal, &out.IdxScansTotal, &out.RowsReadTotal, &out.RowsModifiedTotal, &out.TableSizeMB, &out.IndexSizeMB, &out.RowCount); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &out, nil
}

func (tl *TimescaleLogger) UpsertTableUsageState(ctx context.Context, engine, serverID, dbName, schemaName, tableName string, seqScans, idxScans, rowsRead, rowsModified int64, tableSizeMB, indexSizeMB float64, rowCount int64) error {
	q := `
		INSERT INTO monitor.table_usage_state (
			engine, server_id, db_name, schema_name, table_name,
			seq_scans_total, idx_scans_total, rows_read_total, rows_modified_total, table_size_mb, index_size_mb, row_count, last_seen
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,NOW())
		ON CONFLICT (engine, server_id, db_name, schema_name, table_name)
		DO UPDATE SET
			seq_scans_total = EXCLUDED.seq_scans_total,
			idx_scans_total = EXCLUDED.idx_scans_total,
			rows_read_total = EXCLUDED.rows_read_total,
			rows_modified_total = EXCLUDED.rows_modified_total,
			table_size_mb = EXCLUDED.table_size_mb,
			index_size_mb = EXCLUDED.index_size_mb,
			row_count = EXCLUDED.row_count,
			last_seen = NOW()
	`
	_, err := tl.pool.Exec(ctx, q, engine, serverID, dbName, schemaName, tableName, seqScans, idxScans, rowsRead, rowsModified, tableSizeMB, indexSizeMB, rowCount)
	return err
}

func (tl *TimescaleLogger) InsertTableUsageStat(ctx context.Context, s models.TableUsageStat) error {
	query := `
		INSERT INTO monitor.table_usage_stats (
			capture_timestamp, engine, server_id, db_name, schema_name, table_name,
			seq_scans, idx_scans, rows_read, rows_modified,
			table_size_mb, index_size_mb, row_count
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		ON CONFLICT DO NOTHING
	`
	_, err := tl.pool.Exec(ctx, query,
		s.Timestamp.UTC(), s.Engine, s.ServerID, s.DBName, s.SchemaName, s.TableName,
		s.SeqScans, s.IdxScans, s.RowsRead, s.RowsModified,
		s.TableSizeMB, s.IndexSizeMB, s.RowCount,
	)
	if err != nil {
		slog.Error("[TSLogger] InsertTableUsageStat failed", "err", err)
	}
	return err
}

func (tl *TimescaleLogger) InsertTableSizeHistory(ctx context.Context, s models.TableSizeHistory) error {
	query := `
		INSERT INTO monitor.table_size_history (
			capture_timestamp, engine, server_id, db_name, schema_name, table_name,
			table_size_mb, index_size_mb, row_count
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT DO NOTHING
	`
	_, err := tl.pool.Exec(ctx, query,
		s.Timestamp.UTC(), s.Engine, s.ServerID, s.DBName, s.SchemaName, s.TableName,
		s.TableSizeMB, s.IndexSizeMB, s.RowCount,
	)
	if err != nil {
		slog.Error("[TSLogger] InsertTableSizeHistory failed", "err", err)
	}
	return err
}

func (tl *TimescaleLogger) InsertIndexDefinition(ctx context.Context, d models.IndexDefinition) error {
	q := `
		INSERT INTO monitor.index_definitions (
			capture_timestamp, engine, server_id, db_name, schema_name, table_name, index_name,
			key_columns, include_columns, filter_definition, is_unique, is_pk, index_type
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		ON CONFLICT DO NOTHING
	`
	_, err := tl.pool.Exec(ctx, q,
		d.Timestamp.UTC(), d.Engine, d.ServerID, d.DBName, d.SchemaName, d.TableName, d.IndexName,
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
// for the given engine and serverID.
func (tl *TimescaleLogger) QueryIndexDefinition(ctx context.Context, engine, serverID, dbName, schemaName, indexName string) ([]IndexDefinitionRow, error) {
	if engine == "" || serverID == "" {
		return nil, fmt.Errorf("engine and serverID are required")
	}
	args := []interface{}{engine, serverID}
	clauses := []string{"engine=$1", "server_id=$2::uuid"}
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
		ORDER BY db_name, schema_name, index_name, capture_timestamp DESC
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
			return nil, fmt.Errorf("index_definitions scan: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (tl *TimescaleLogger) QueryStorageIndexHealthIndexUsage(ctx context.Context, engine, serverID, from, to, db, schema, table string, limit int) ([]models.IndexUsageStat, error) {
	where := "engine = $1 AND server_id = $2::uuid AND capture_timestamp >= $3::timestamptz AND capture_timestamp <= $4::timestamptz"
	args := []interface{}{engine, serverID, from, to}

	if db != "" {
		args = append(args, db)
		where += fmt.Sprintf(" AND db_name = $%d", len(args))
	}
	if schema != "" {
		args = append(args, schema)
		where += fmt.Sprintf(" AND schema_name = $%d", len(args))
	}
	if table != "" {
		args = append(args, table)
		where += fmt.Sprintf(" AND table_name = $%d", len(args))
	}

	args = append(args, limit)
	q := fmt.Sprintf(`SELECT 
			MAX(capture_timestamp) as capture_timestamp, 
			engine, server_id, db_name, schema_name, table_name, index_name, 
			SUM(COALESCE(seeks,0)) as seeks, 
			SUM(COALESCE(scans,0)) as scans, 
			SUM(COALESCE(lookups,0)) as lookups, 
			SUM(COALESCE(updates,0)) as updates, 
			MAX(COALESCE(index_size_mb,0))::float8 as index_size_mb, 
			BOOL_OR(COALESCE(is_unique,false)) as is_unique, 
			BOOL_OR(COALESCE(is_pk,false)) as is_pk, 
			MAX(COALESCE(fillfactor,0)) as fillfactor,
			MAX(last_user_seek) as last_user_seek,
			MAX(last_user_scan) as last_user_scan,
			MAX(last_user_lookup) as last_user_lookup
	      FROM monitor.index_usage_stats 
		  WHERE %s
		  GROUP BY engine, server_id, db_name, schema_name, table_name, index_name
	      ORDER BY (SUM(COALESCE(seeks,0)) + SUM(COALESCE(scans,0)) + SUM(COALESCE(lookups,0))) DESC, SUM(COALESCE(updates,0)) DESC 
		  LIMIT $%d`, where, len(args))

	rows, err := tl.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.IndexUsageStat
	for rows.Next() {
		var r models.IndexUsageStat
		if err := rows.Scan(&r.Timestamp, &r.Engine, &r.ServerID, &r.DBName, &r.SchemaName, &r.TableName, &r.IndexName,
			&r.Seeks, &r.Scans, &r.Lookups, &r.Updates, &r.IndexSizeMB, &r.IsUnique, &r.IsPK, &r.FillFactor,
			&r.LastUserSeek, &r.LastUserScan, &r.LastUserLookup); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, rows.Err()
	}

	func (tl *TimescaleLogger) QueryStorageIndexHealthTableUsage(ctx context.Context, engine, serverID, from, to string, limit int) ([]models.TableUsageStat, error) {
	q := `SELECT capture_timestamp, engine, server_id, db_name, schema_name, table_name, seq_scans, idx_scans, rows_read, rows_modified, table_size_mb, index_size_mb, row_count
	      FROM monitor.table_usage_stats WHERE engine = $1 AND server_id = $2::uuid AND capture_timestamp >= $3::timestamptz AND capture_timestamp <= $4::timestamptz
	      ORDER BY capture_timestamp DESC LIMIT $5`
	rows, err := tl.pool.Query(ctx, q, engine, serverID, from, to, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.TableUsageStat
	for rows.Next() {
		var r models.TableUsageStat
		if err := rows.Scan(&r.Timestamp, &r.Engine, &r.ServerID, &r.DBName, &r.SchemaName, &r.TableName, &r.SeqScans, &r.IdxScans, &r.RowsRead, &r.RowsModified, &r.TableSizeMB, &r.IndexSizeMB, &r.RowCount); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, nil
	}

	func (tl *TimescaleLogger) QueryStorageIndexHealthTableGrowth(ctx context.Context, engine, serverID string, from, to string, limit int) ([]models.TableSizeHistory, error) {
	if limit <= 0 {
		limit = 500
	}
	q := `SELECT capture_timestamp, engine, server_id, db_name, schema_name, table_name, table_size_mb, index_size_mb, row_count
	      FROM monitor.table_size_history WHERE engine = $1 AND server_id = $2::uuid AND capture_timestamp >= $3::timestamptz AND capture_timestamp <= $4::timestamptz
	      ORDER BY capture_timestamp DESC LIMIT $5`
	rows, err := tl.pool.Query(ctx, q, engine, serverID, from, to, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.TableSizeHistory
	for rows.Next() {
		var r models.TableSizeHistory
		if err := rows.Scan(&r.Timestamp, &r.Engine, &r.ServerID, &r.DBName, &r.SchemaName, &r.TableName, &r.TableSizeMB, &r.IndexSizeMB, &r.RowCount); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, nil
	}
func (tl *TimescaleLogger) RefreshIndexUnusedCandidatesDaily(ctx context.Context, engine, serverID string, analysisEnd time.Time, minUpdates int64) (int64, error) {
	if minUpdates <= 0 {
		minUpdates = 100
	}
	u := analysisEnd.UTC()
	runAt := time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
	tx, err := tl.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	del := `DELETE FROM monitor.index_unused_candidates_daily WHERE run_at = $1 AND engine = $2 AND server_id = $3`
	if _, err := tx.Exec(ctx, del, runAt, engine, serverID); err != nil {
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
				  AND capture_timestamp > ($4::timestamptz - INTERVAL '24 hours')
				  AND capture_timestamp <= $4::timestamptz
				GROUP BY db_name, schema_name, table_name, index_name
			) w
			JOIN (
				SELECT DISTINCT ON (db_name, schema_name, table_name, index_name)
					db_name, schema_name, table_name, index_name,
					COALESCE(index_size_mb,0)::float8 AS size_mb,
					last_user_seek,
					COALESCE(is_pk,false) AS is_pk
				FROM monitor.index_usage_stats
				WHERE engine = $2 AND server_id = $3 AND capture_timestamp <= $4::timestamptz
				ORDER BY db_name, schema_name, table_name, index_name, capture_timestamp DESC
			) l USING (db_name, schema_name, table_name, index_name)
			WHERE w.reads = 0 AND l.is_pk = false
			  AND (
			    ($2::text = 'sqlserver' AND w.upd >= $5)
			    OR ($2::text = 'postgres' AND COALESCE(l.size_mb,0) >= 0.01)
			  )
		) x
		WHERE x.rnk <= 100
	`
	tag, err := tx.Exec(ctx, ins, runAt, engine, serverID, analysisEnd.UTC(), minUpdates)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (tl *TimescaleLogger) QueryStorageIndexHealthFilterOptions(ctx context.Context, engine, serverID, from, to string, dbName, schemaName string) (*SIHFilterOptions, error) {
	baseWhereTime := `engine=$1 AND server_id=$2::uuid AND capture_timestamp >= $3::timestamptz AND capture_timestamp <= $4::timestamptz`
	baseArgsTime := []interface{}{engine, serverID, from, to}

	baseWhereAny := `engine=$1 AND server_id=$2::uuid`
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
	counts.TableSizeHistory, _ = countIn("monitor.table_size_history")
	counts.TableUsageStats, _ = countIn("monitor.table_usage_stats")
	counts.IndexUsageStats, _ = countIn("monitor.index_usage_stats")

	queryDistinctUnion := func(col, whereSQL string, args []interface{}) ([]string, error) {
		q := fmt.Sprintf(`
			SELECT DISTINCT v FROM (
				SELECT %s::text AS v FROM monitor.table_size_history WHERE %s
				UNION
				SELECT %s::text AS v FROM monitor.table_usage_stats WHERE %s
				UNION
				SELECT %s::text AS v FROM monitor.index_usage_stats WHERE %s
			) x
			WHERE v IS NOT NULL AND btrim(v) <> '' ORDER BY v LIMIT 1000
		`, col, whereSQL, col, whereSQL, col, whereSQL)
		rows, err := tl.pool.Query(ctx, q, args...)
		if err != nil {
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
		return out, nil
	}

	dbs, _ := queryDistinctUnion("db_name", baseWhereTime, baseArgsTime)
	if len(dbs) == 0 {
		dbs, _ = queryDistinctUnion("db_name", baseWhereAny, baseArgsAny)
	}

	whereSchemasTime := baseWhereTime
	argsSchemasTime := append([]interface{}{}, baseArgsTime...)
	if dbName != "" {
		whereSchemasTime += " AND db_name = $5"
		argsSchemasTime = append(argsSchemasTime, dbName)
	}
	schemas, _ := queryDistinctUnion("schema_name", whereSchemasTime, argsSchemasTime)

	whereTablesTime := baseWhereTime
	argsTablesTime := append([]interface{}{}, baseArgsTime...)
	n := 5
	if dbName != "" {
		whereTablesTime += " AND db_name = $" + fmt.Sprint(n)
		argsTablesTime = append(argsTablesTime, dbName)
		n++
	}
	if schemaName != "" {
		whereTablesTime += " AND schema_name = $" + fmt.Sprint(n)
		argsTablesTime = append(argsTablesTime, schemaName)
	}
	tables, _ := queryDistinctUnion("table_name", whereTablesTime, argsTablesTime)

	return &SIHFilterOptions{
		Databases: dbs, Schemas: schemas, Tables: tables, SourceRowCounts: counts,
	}, nil
}

// ---- New Time-Series Storage & Index History Methods ----

type TableSizeHistoryRow struct {
	CaptureTimestamp time.Time `json:"capture_timestamp"`
	ServerID         uuid.UUID `json:"server_id"`
	DatabaseName     string    `json:"database_name"`
	SchemaName       string    `json:"schema_name"`
	TableName        string    `json:"table_name"`
	RowCount         int64     `json:"row_count"`
	TotalMB          float64   `json:"total_mb"`
	DataMB           float64   `json:"data_mb"`
	IndexMB          float64   `json:"index_mb"`
}

func (tl *TimescaleLogger) LogTableSizeHistoryWithChangeDetection(ctx context.Context, serverID uuid.UUID, rows []TableSizeHistoryRow) error {
	if len(rows) == 0 {
		return nil
	}
	dbName := rows[0].DatabaseName
	sig := tl.FingerprintTableSizeRows(serverID, rows)
	if tl.EnterpriseSnapshotUnchanged(serverID, enterpriseKindTableSize, sig, dbName) {
		return nil
	}
	err := tl.LogTableSizeHistory(ctx, rows)
	if err == nil {
		tl.RememberEnterpriseSnapshot(serverID, enterpriseKindTableSize, sig, dbName)
	}
	return err
}

func (tl *TimescaleLogger) LogTableSizeHistory(ctx context.Context, rows []TableSizeHistoryRow) error {
	if len(rows) == 0 {
		return nil
	}
	const q = `INSERT INTO snapshot.sqlserver_table_size_history (capture_timestamp, server_id, database_name, schema_name, table_name, row_count, total_mb, data_mb, index_mb)
	      VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT DO NOTHING`
	batch := &pgx.Batch{}
	for _, r := range rows {
		batch.Queue(q, r.CaptureTimestamp, r.ServerID, r.DatabaseName, r.SchemaName, r.TableName, r.RowCount, r.TotalMB, r.DataMB, r.IndexMB)
	}
	br := tl.pool.SendBatch(ctx, batch)
	defer br.Close()
	for range rows {
		_, _ = br.Exec()
	}
	return nil
}

func (tl *TimescaleLogger) GetTableSizeHistory(ctx context.Context, engine, serverID, from, to string, db, schema, table string) ([]models.TableSizeHistory, error) {
	var q string
	var args []interface{}

	// Step 1: Try monitor.table_usage_stats (has 15m cadence deltas/snapshots)
	// Column names in monitor.* are fixed: server_id, db_name
	q = `SELECT capture_timestamp, server_id, db_name, schema_name, table_name, row_count, table_size_mb, index_size_mb
	      FROM monitor.table_usage_stats
	      WHERE engine = $1 AND server_id = $2::uuid AND capture_timestamp >= $3::timestamptz AND capture_timestamp <= $4::timestamptz`
	args = []interface{}{engine, serverID, from, to}

	if db != "" && db != "all" {
		args = append(args, db)
		q += fmt.Sprintf(" AND db_name = $%d", len(args))
	}
	if schema != "" && schema != "all" {
		args = append(args, schema)
		q += fmt.Sprintf(" AND schema_name = $%d", len(args))
	}
	if table != "" && table != "all" {
		args = append(args, table)
		q += fmt.Sprintf(" AND table_name = $%d", len(args))
	}

	q += " ORDER BY capture_timestamp ASC"

	rows, err := tl.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.TableSizeHistory
	for rows.Next() {
		var r models.TableSizeHistory
		if err := rows.Scan(&r.Timestamp, &r.ServerID, &r.DBName, &r.SchemaName, &r.TableName, &r.RowCount, &r.TableSizeMB, &r.IndexSizeMB); err != nil {
			continue
		}
		r.Engine = engine
		out = append(out, r)
	}

	// Step 2: Fallback to table_size_history or engine-specific history if usage stats empty
	if len(out) == 0 {
		if engine == "postgres" {
			q = `SELECT capture_timestamp, server_id, db_name, schema_name, table_name, row_count, table_size_mb, index_size_mb
			      FROM monitor.table_size_history
			      WHERE engine = $1 AND server_id = $2 AND capture_timestamp >= $3::timestamptz AND capture_timestamp <= $4::timestamptz`
			args = []interface{}{engine, serverID, from, to}
		} else {
			// SQLSERVER snapshot table uses server_id and database_name
			q = `SELECT capture_timestamp, server_id, database_name, schema_name, table_name, row_count, total_mb, index_mb
			      FROM snapshot.sqlserver_table_size_history
			      WHERE server_id = $1::uuid AND capture_timestamp >= $2::timestamptz AND capture_timestamp <= $3::timestamptz`
			args = []interface{}{serverID, from, to}
		}

		if db != "" && db != "all" {
			args = append(args, db)
			q += fmt.Sprintf(" AND %s = $%d", tl.dbCol(engine), len(args))
		}
		if schema != "" && schema != "all" {
			args = append(args, schema)
			q += fmt.Sprintf(" AND schema_name = $%d", len(args))
		}
		if table != "" && table != "all" {
			args = append(args, table)
			q += fmt.Sprintf(" AND table_name = $%d", len(args))
		}

		if engine == "postgres" {
			q += " ORDER BY capture_timestamp ASC"
		} else {
			q += " ORDER BY capture_timestamp ASC"
		}

		rows2, _ := tl.pool.Query(ctx, q, args...)
		if rows2 != nil {
			defer rows2.Close()
			for rows2.Next() {
				var r models.TableSizeHistory
				if err := rows2.Scan(&r.Timestamp, &r.ServerID, &r.DBName, &r.SchemaName, &r.TableName, &r.RowCount, &r.TableSizeMB, &r.IndexSizeMB); err != nil {
					continue
				}
				r.Engine = engine
				out = append(out, r)
			}
		}
	}
	return out, nil
}

func (tl *TimescaleLogger) dbCol(engine string) string {
	if engine == "postgres" {
		return "db_name"
	}
	return "database_name"
}

func (tl *TimescaleLogger) GetIndexUsageHistory(ctx context.Context, engine, serverID, from, to string, db, schema, table string) ([]models.IndexUsageStat, error) {
	var q string
	var args []interface{}

	// Step 1: Try monitor.index_usage_stats (15m cadence)
	q = `SELECT capture_timestamp, server_id, db_name, schema_name, table_name, index_name, index_size_mb, 
	            COALESCE(seeks,0), COALESCE(scans,0), COALESCE(lookups,0), COALESCE(updates,0)
	      FROM monitor.index_usage_stats
	      WHERE engine = $1 AND server_id = $2::uuid AND capture_timestamp >= $3::timestamptz AND capture_timestamp <= $4::timestamptz`
	args = []interface{}{engine, serverID, from, to}

	if db != "" && db != "all" {
		args = append(args, db)
		q += fmt.Sprintf(" AND db_name = $%d", len(args))
	}
	if schema != "" && schema != "all" {
		args = append(args, schema)
		q += fmt.Sprintf(" AND schema_name = $%d", len(args))
	}
	if table != "" && table != "all" {
		args = append(args, table)
		q += fmt.Sprintf(" AND table_name = $%d", len(args))
	}

	q += " ORDER BY capture_timestamp ASC"

	rows, err := tl.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.IndexUsageStat
	for rows.Next() {
		var r models.IndexUsageStat
		if err := rows.Scan(&r.Timestamp, &r.ServerID, &r.DBName, &r.SchemaName, &r.TableName, &r.IndexName, &r.IndexSizeMB, &r.Seeks, &r.Scans, &r.Lookups, &r.Updates); err != nil {
			continue
		}
		r.Engine = engine
		out = append(out, r)
	}

	return out, nil
}

// GetIndexUsageTrend returns aggregated time-series of seeks vs scans for an serverID.
func (tl *TimescaleLogger) GetIndexUsageTrend(ctx context.Context, engine, serverID, from, to string) ([]map[string]interface{}, error) {
	q := `
		SELECT 
			time_bucket('15 minutes', capture_timestamp) AS bucket,
			SUM(COALESCE(seeks, 0))::bigint AS seeks,
			SUM(COALESCE(scans, 0))::bigint AS scans,
			SUM(COALESCE(lookups, 0))::bigint AS lookups
		FROM monitor.index_usage_stats
		WHERE engine = $1 AND server_id = $2::uuid AND capture_timestamp >= $3::timestamptz AND capture_timestamp <= $4::timestamptz
		GROUP BY bucket
		ORDER BY bucket ASC
	`
	rows, err := tl.pool.Query(ctx, q, engine, serverID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var bucket time.Time
		var seeks, scans, lookups int64
		if err := rows.Scan(&bucket, &seeks, &scans, &lookups); err == nil {
			results = append(results, map[string]interface{}{
				"bucket":  bucket,
				"seeks":   seeks,
				"scans":   scans,
				"lookups": lookups,
			})
		}
	}
	return results, nil
}
