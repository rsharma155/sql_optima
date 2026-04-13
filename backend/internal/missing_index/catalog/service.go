package catalog

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rsharma155/sql_optima/internal/missing_index/logging"
	"github.com/rsharma155/sql_optima/internal/missing_index/types"
)

var ErrConnectionFailed = errors.New("database connection failed")

func NewConnection(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, err
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, ErrConnectionFailed
	}

	return pool, nil
}

type TableInfo struct {
	Table       types.TableRef
	RowCount    int64
	SizeBytes   int64
	ColumnStats []types.ColumnStats
	IndexStats  []types.ExistingIndex
}

func CheckHypoPG(ctx context.Context, pool *pgxpool.Pool) (bool, error) {
	var exists bool
	err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM pg_extension
			WHERE extname = 'hypopg'
		)
	`).Scan(&exists)

	if err != nil {
		logging.Warn(ctx, "Failed to check HypoPG extension", map[string]any{"error": err.Error()})
		return false, err
	}

	return exists, nil
}

func GetTableInfo(ctx context.Context, pool *pgxpool.Pool, table types.TableRef) (TableInfo, error) {
	info := TableInfo{Table: table}

	err := pool.QueryRow(ctx, `
		SELECT COALESCE(reltuples::bigint, 0)
		FROM pg_class
		WHERE relname = $1
		AND relnamespace = (SELECT oid FROM pg_namespace WHERE nspname = $2)
	`, table.Name, table.Schema).Scan(&info.RowCount)

	if err != nil {
		logging.Warn(ctx, "Failed to get row count", map[string]any{"error": err.Error()})
	}

	err = pool.QueryRow(ctx, `
		SELECT COALESCE(pg_total_relation_size($1), 0)
	`, table.Schema+"."+table.Name).Scan(&info.SizeBytes)

	if err != nil {
		logging.Warn(ctx, "Failed to get table size", map[string]any{"error": err.Error()})
	}

	rows, err := pool.Query(ctx, `
		SELECT 
			attname,
			COALESCE(s.n_distinct, 0),
			s.most_common_vals,
			s.most_common_freqs,
			s.histogram_bounds,
			COALESCE(s.correlation, 0),
			COALESCE(s.null_frac, 0)
		FROM pg_attribute a
		LEFT JOIN pg_stats s ON s.tablename = a.relname AND s.attname = a.attname
		JOIN pg_class c ON c.oid = a.attrelid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = $1
		AND c.relname = $2
		AND a.attnum > 0
		AND NOT a.attisdropped
	`, table.Schema, table.Name)

	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var colStats types.ColumnStats
			if err := rows.Scan(
				&colStats.ColumnName,
				&colStats.NDistinct,
				&colStats.MostCommonVals,
				&colStats.MostCommonFreqs,
				&colStats.HistogramBounds,
				&colStats.Correlation,
				&colStats.NullFrac,
			); err == nil {
				colStats.TableName = table.Name
				info.ColumnStats = append(info.ColumnStats, colStats)
			}
		}
	}

	indexRows, err := pool.Query(ctx, `
		SELECT 
			i.relname,
			ix.indisunique,
			ix.indpred IS NOT NULL,
			ix.indpred,
			am.amname,
			pg_get_indexdef(ix.oid) as index_def
		FROM pg_index ix
		JOIN pg_class i ON i.oid = ix.indexrelid
		JOIN pg_class t ON t.oid = ix.indrelid
		JOIN pg_namespace n ON n.oid = t.relnamespace
		JOIN pg_am am ON am.oid = i.relam
		WHERE n.nspname = $1
		AND t.relname = $2
	`, table.Schema, table.Name)

	if err == nil {
		defer indexRows.Close()
		for indexRows.Next() {
			var idx types.ExistingIndex
			var indexDef string
			var isPartial bool

			if err := indexRows.Scan(
				&idx.IndexName,
				&idx.IsUnique,
				&isPartial,
				&idx.PartialPredicate,
				&idx.IndexMethod,
				&indexDef,
			); err == nil {
				idx.Table = table
				idx.IsPartial = isPartial
				idx.KeyColumns = extractIndexColumns(indexDef)
				idx.IncludeColumns = extractIncludeColumns(indexDef)
				info.IndexStats = append(info.IndexStats, idx)
			}
		}
	}

	logging.Info(ctx, "Retrieved table info", map[string]any{
		"table":     table.Name,
		"row_count": info.RowCount,
		"indexes":   len(info.IndexStats),
	})

	return info, nil
}

func extractIndexColumns(indexDef string) []string {
	return []string{}
}

func extractIncludeColumns(indexDef string) []string {
	return []string{}
}
