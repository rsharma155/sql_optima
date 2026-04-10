package hot

import (
	"context"
	"strings"
	"time"
)

type PostgresBackupRunRow struct {
	CaptureTimestamp   time.Time              `json:"capture_timestamp"`
	ServerInstanceName string                 `json:"server_instance_name"`
	Tool               string                 `json:"tool"`
	BackupType         string                 `json:"backup_type"`
	Status             string                 `json:"status"`
	StartedAt          *time.Time             `json:"started_at,omitempty"`
	FinishedAt         *time.Time             `json:"finished_at,omitempty"`
	DurationSeconds    int64                  `json:"duration_seconds"`
	WalArchivedUntil   *time.Time             `json:"wal_archived_until,omitempty"`
	Repo               string                 `json:"repo,omitempty"`
	SizeBytes          int64                  `json:"size_bytes"`
	ErrorMessage       string                 `json:"error_message,omitempty"`
	Metadata           map[string]interface{} `json:"metadata,omitempty"`
}

func normalizeBackupStatus(s string) string {
	v := strings.ToLower(strings.TrimSpace(s))
	switch v {
	case "ok", "success", "succeeded", "passed":
		return "success"
	case "fail", "failed", "error":
		return "failed"
	case "warn", "warning":
		return "warning"
	default:
		if v == "" {
			return "unknown"
		}
		return v
	}
}

func (tl *TimescaleLogger) LogPostgresBackupRun(ctx context.Context, r PostgresBackupRunRow) error {
	r.Status = normalizeBackupStatus(r.Status)
	if strings.TrimSpace(r.Tool) == "" {
		r.Tool = "custom"
	}
	if strings.TrimSpace(r.BackupType) == "" {
		r.BackupType = "unknown"
	}

	q := `
		INSERT INTO postgres_backup_runs (
			capture_timestamp, server_instance_name,
			tool, backup_type, status,
			started_at, finished_at, duration_seconds,
			wal_archived_until, repo, size_bytes, error_message, metadata
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
	`

	now := time.Now().UTC()
	_, err := tl.pool.Exec(ctx, q,
		now, r.ServerInstanceName,
		r.Tool, r.BackupType, r.Status,
		r.StartedAt, r.FinishedAt, r.DurationSeconds,
		r.WalArchivedUntil, r.Repo, r.SizeBytes, r.ErrorMessage, r.Metadata,
	)
	return err
}

func (tl *TimescaleLogger) GetLatestPostgresBackupRun(ctx context.Context, instanceName string) (*PostgresBackupRunRow, error) {
	q := `
		SELECT capture_timestamp, server_instance_name,
		       tool, backup_type, status,
		       started_at, finished_at, duration_seconds,
		       wal_archived_until, COALESCE(repo,''), size_bytes, COALESCE(error_message,''), metadata
		FROM postgres_backup_runs
		WHERE server_instance_name = $1
		ORDER BY capture_timestamp DESC
		LIMIT 1
	`
	var r PostgresBackupRunRow
	var startedAt, finishedAt, walUntil *time.Time
	if err := tl.pool.QueryRow(ctx, q, instanceName).Scan(
		&r.CaptureTimestamp, &r.ServerInstanceName,
		&r.Tool, &r.BackupType, &r.Status,
		&startedAt, &finishedAt, &r.DurationSeconds,
		&walUntil, &r.Repo, &r.SizeBytes, &r.ErrorMessage, &r.Metadata,
	); err != nil {
		return nil, err
	}
	r.StartedAt = startedAt
	r.FinishedAt = finishedAt
	r.WalArchivedUntil = walUntil
	return &r, nil
}

func (tl *TimescaleLogger) GetPostgresBackupRunHistory(ctx context.Context, instanceName string, limit int) ([]PostgresBackupRunRow, error) {
	if limit <= 0 {
		limit = 100
	}
	q := `
		SELECT capture_timestamp, server_instance_name,
		       tool, backup_type, status,
		       started_at, finished_at, duration_seconds,
		       wal_archived_until, COALESCE(repo,''), size_bytes, COALESCE(error_message,''), metadata
		FROM postgres_backup_runs
		WHERE server_instance_name = $1
		ORDER BY capture_timestamp DESC
		LIMIT $2
	`
	rows, err := tl.pool.Query(ctx, q, instanceName, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PostgresBackupRunRow
	for rows.Next() {
		var r PostgresBackupRunRow
		var startedAt, finishedAt, walUntil *time.Time
		if err := rows.Scan(
			&r.CaptureTimestamp, &r.ServerInstanceName,
			&r.Tool, &r.BackupType, &r.Status,
			&startedAt, &finishedAt, &r.DurationSeconds,
			&walUntil, &r.Repo, &r.SizeBytes, &r.ErrorMessage, &r.Metadata,
		); err != nil {
			continue
		}
		r.StartedAt = startedAt
		r.FinishedAt = finishedAt
		r.WalArchivedUntil = walUntil
		out = append(out, r)
	}
	return out, rows.Err()
}

