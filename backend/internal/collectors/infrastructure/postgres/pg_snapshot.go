package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"

	"github.com/rsharma155/sql_optima/internal/collectors/domain"
)

type PGSnapshotRepository struct {
	db             *sql.DB
	resolvedQuery  string
	resolvedSchema string
	mu             sync.Mutex
}

func NewPGSnapshotRepository(db *sql.DB) *PGSnapshotRepository {
	return &PGSnapshotRepository{db: db}
}

const pgStatStatementsBaseSQL = `
/* SQL_OPTIMA */
SELECT
 now() AS capture_time,
 userid,
 dbid,
 queryid,
 LEFT(query, 4096) AS query,  -- truncate long query text
 calls,
 total_exec_time,
 rows,
 shared_blks_hit,
 shared_blks_read,
 temp_blks_written
FROM %s
WHERE dbid NOT IN (
 SELECT oid FROM pg_database
 WHERE datname IN ('template0','template1')
)
ORDER BY total_exec_time DESC
LIMIT 200;
`

func (r *PGSnapshotRepository) resolvePGSS(ctx context.Context) (string, error) {
	r.mu.Lock()
	if r.resolvedQuery != "" {
		q := r.resolvedQuery
		r.mu.Unlock()
		return q, nil
	}
	r.mu.Unlock()

	// Try to find the schema where pg_stat_statements is installed via pg_views.
	var schema string
	err := r.db.QueryRowContext(ctx, `
		SELECT schemaname
		FROM pg_views
		WHERE viewname = 'pg_stat_statements'
		LIMIT 1
	`).Scan(&schema)

	if err != nil || schema == "" {
		// Fallback: pg_views may not surface the view if the extension was installed
		// in a non-default schema. Check pg_class directly.
		var fallbackSchema string
		_ = r.db.QueryRowContext(ctx, `
			SELECT n.nspname
			FROM pg_class cl
			JOIN pg_namespace n ON n.oid = cl.relnamespace
			WHERE cl.relname = 'pg_stat_statements'
			  AND cl.relkind IN ('v','r','m')
			LIMIT 1`).Scan(&fallbackSchema)

		if fallbackSchema == "" {
			// Determine which database this connection targets for a clearer error.
			var dbName string
			_ = r.db.QueryRowContext(ctx, "SELECT current_database()").Scan(&dbName)
			return "", fmt.Errorf("pg_stat_statements not found in database %q. Run 'CREATE EXTENSION pg_stat_statements;' in that database.", dbName)
		}
		schema = fallbackSchema
	}

	r.mu.Lock()
	r.resolvedSchema = schema
	r.resolvedQuery = fmt.Sprintf(pgStatStatementsBaseSQL, fmt.Sprintf("%s.pg_stat_statements", schema))
	q := r.resolvedQuery
	r.mu.Unlock()

	return q, nil
}

const pgStatActivitySQL = `
/* SQL_OPTIMA */
SELECT
  now() AS capture_time,
  pid,
  COALESCE(query_id, 0) as query_id,
  COALESCE(usename, '') AS usename,
  COALESCE(datname, '') AS datname,
  COALESCE(application_name, '') AS application_name,
  client_addr,
  state
FROM pg_stat_activity
WHERE state='active'
AND usename <> 'postgres';
`

func (r *PGSnapshotRepository) FetchSnapshot(ctx context.Context) ([]domain.PGQuerySnapshot, error) {
	query, err := r.resolvePGSS(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		// If it fails even after resolution (e.g. permissions), we reset the cache
		if strings.Contains(err.Error(), "does not exist") {
			r.mu.Lock()
			r.resolvedQuery = ""
			r.mu.Unlock()
		}
		return nil, err
	}
	defer rows.Close()

	var snapshots []domain.PGQuerySnapshot
	for rows.Next() {
		var s domain.PGQuerySnapshot
		err := rows.Scan(
			&s.CaptureTime,
			&s.UserID,
			&s.DBID,
			&s.QueryID,
			&s.Query,
			&s.Calls,
			&s.TotalExecTime,
			&s.Rows,
			&s.SharedBlksHit,
			&s.SharedBlksRead,
			&s.TempBlksWritten,
		)
		if err != nil {
			return nil, err
		}
		snapshots = append(snapshots, s)
	}
	return snapshots, nil
}

func (r *PGSnapshotRepository) FetchActivityEnrichment(ctx context.Context) ([]domain.PGActivityEnrichment, error) {
	rows, err := r.db.QueryContext(ctx, pgStatActivitySQL)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var enrichments []domain.PGActivityEnrichment
	for rows.Next() {
		var e domain.PGActivityEnrichment
		err := rows.Scan(
			&e.CaptureTime,
			&e.PID,
			&e.QueryID,
			&e.UserName,
			&e.DatabaseName,
			&e.ApplicationName,
			&e.ClientAddr,
			&e.State,
		)
		if err != nil {
			return nil, err
		}
		enrichments = append(enrichments, e)
	}
	return enrichments, nil
}
