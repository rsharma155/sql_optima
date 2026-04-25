package postgres

import (
	"context"
	"database/sql"

	"github.com/rsharma155/sql_optima/internal/collectors/domain"
)

type PGSnapshotRepository struct {
	db *sql.DB
}

func NewPGSnapshotRepository(db *sql.DB) *PGSnapshotRepository {
	return &PGSnapshotRepository{db: db}
}

const pgStatStatementsSQL = `
SELECT
 now() AS capture_time,
 userid,
 dbid,
 queryid,
 query,
 calls,
 total_exec_time,
 rows,
 shared_blks_hit,
 shared_blks_read,
 temp_blks_written
FROM pg_stat_statements
WHERE dbid NOT IN (
 SELECT oid FROM pg_database
 WHERE datname IN ('postgres','template0','template1')
);
`

const pgStatActivitySQL = `
SELECT
  now() AS capture_time,
  pid,
  COALESCE(query_id, 0) as query_id,
  usename,
  datname,
  application_name,
  client_addr,
  state
FROM pg_stat_activity
WHERE state='active'
AND usename <> 'postgres';
`

func (r *PGSnapshotRepository) FetchSnapshot(ctx context.Context) ([]domain.PGQuerySnapshot, error) {
	rows, err := r.db.QueryContext(ctx, pgStatStatementsSQL)
	if err != nil {
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
