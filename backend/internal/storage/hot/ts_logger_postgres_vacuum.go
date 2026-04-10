package hot

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

type PostgresVacuumProgressRow struct {
	CaptureTimestamp   time.Time `json:"capture_timestamp"`
	ServerInstanceName string    `json:"server_instance_name"`
	PID                int64     `json:"pid"`
	DatabaseName       string    `json:"database_name,omitempty"`
	UserName           string    `json:"user_name,omitempty"`
	RelationName       string    `json:"relation_name,omitempty"`
	Phase              string    `json:"phase,omitempty"`
	HeapBlksTotal      int64     `json:"heap_blks_total"`
	HeapBlksScanned    int64     `json:"heap_blks_scanned"`
	HeapBlksVacuumed   int64     `json:"heap_blks_vacuumed"`
	IndexVacuumCount   int64     `json:"index_vacuum_count"`
	MaxDeadTuples      int64     `json:"max_dead_tuples"`
	NumDeadTuples      int64     `json:"num_dead_tuples"`
}

func (tl *TimescaleLogger) LogPostgresVacuumProgress(ctx context.Context, instanceName string, rows []PostgresVacuumProgressRow) error {
	if len(rows) == 0 {
		return nil
	}
	q := `
		INSERT INTO postgres_vacuum_progress (
			capture_timestamp, server_instance_name,
			pid, database_name, user_name, relation_name, phase,
			heap_blks_total, heap_blks_scanned, heap_blks_vacuumed,
			index_vacuum_count, max_dead_tuples, num_dead_tuples
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
	`
	now := time.Now().UTC()
	b := &pgx.Batch{}
	for _, r := range rows {
		b.Queue(q,
			now, instanceName,
			r.PID, r.DatabaseName, r.UserName, r.RelationName, r.Phase,
			r.HeapBlksTotal, r.HeapBlksScanned, r.HeapBlksVacuumed,
			r.IndexVacuumCount, r.MaxDeadTuples, r.NumDeadTuples,
		)
	}
	br := tl.pool.SendBatch(ctx, b)
	defer br.Close()
	for range rows {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return nil
}

func (tl *TimescaleLogger) GetPostgresVacuumProgress(ctx context.Context, instanceName string, limit int) ([]PostgresVacuumProgressRow, error) {
	if limit <= 0 {
		limit = 200
	}
	q := `
		SELECT capture_timestamp, server_instance_name,
		       pid, COALESCE(database_name,''), COALESCE(user_name,''), COALESCE(relation_name,''), COALESCE(phase,''),
		       heap_blks_total, heap_blks_scanned, heap_blks_vacuumed,
		       index_vacuum_count, max_dead_tuples, num_dead_tuples
		FROM postgres_vacuum_progress
		WHERE server_instance_name = $1
		ORDER BY capture_timestamp DESC, heap_blks_scanned DESC
		LIMIT $2
	`
	rows, err := tl.pool.Query(ctx, q, instanceName, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PostgresVacuumProgressRow
	for rows.Next() {
		var r PostgresVacuumProgressRow
		if err := rows.Scan(
			&r.CaptureTimestamp, &r.ServerInstanceName,
			&r.PID, &r.DatabaseName, &r.UserName, &r.RelationName, &r.Phase,
			&r.HeapBlksTotal, &r.HeapBlksScanned, &r.HeapBlksVacuumed,
			&r.IndexVacuumCount, &r.MaxDeadTuples, &r.NumDeadTuples,
		); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

