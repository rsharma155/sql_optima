// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: TimescaleDB implementation for alert persistence and retrieval.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rsharma155/sql_optima/internal/domain/alerts"
)

type TimescaleAlertStore struct {
	pool *pgxpool.Pool
}

func NewTimescaleAlertStore(pool *pgxpool.Pool) *TimescaleAlertStore {
	return &TimescaleAlertStore{pool: pool}
}

func (s *TimescaleAlertStore) Upsert(ctx context.Context, a alerts.Alert) (alerts.Alert, error) {
	evidenceJSON, _ := json.Marshal(a.Evidence)

	// Defensive check: Ensure server_id exists in optima_servers to prevent FK violation.
	// If it doesn't exist, we skip insertion for now to avoid crashing the runner.
	var exists bool
	_ = s.pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM optima_servers WHERE id = $1)", a.ServerID).Scan(&exists)
	if !exists {
		return a, fmt.Errorf("server_id %s not found in optima_servers, skipping alert", a.ServerID)
	}

	q := `
		INSERT INTO optima_alerts (
			fingerprint, server_id, instance_name, engine, severity, status, category, title, description, evidence, first_seen_at, last_seen_at, hit_count
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, 1)
		ON CONFLICT (fingerprint) WHERE status IN ('open', 'acknowledged') DO UPDATE SET
			last_seen_at = EXCLUDED.last_seen_at,
			hit_count = optima_alerts.hit_count + 1,
			severity = EXCLUDED.severity,
			description = EXCLUDED.description,
			evidence = EXCLUDED.evidence,
			updated_at = now()
		RETURNING id, hit_count, status, first_seen_at
	`

	err := s.pool.QueryRow(ctx, q,
		a.Fingerprint, a.ServerID, a.ServerName, string(a.Engine), string(a.Severity), string(a.Status),
		a.Category, a.Title, a.Description, evidenceJSON, a.FirstSeenAt, a.LastSeenAt,
	).Scan(&a.ID, &a.HitCount, &a.Status, &a.FirstSeenAt)

	return a, err
}

func (s *TimescaleAlertStore) GetByID(ctx context.Context, id uuid.UUID) (alerts.Alert, error) {
	q := `
		SELECT id, fingerprint, server_id, instance_name, engine, severity, status, category, title, description, evidence, first_seen_at, last_seen_at, hit_count, acknowledged_by, acknowledged_at, resolved_by, resolved_at, created_at, updated_at
		FROM optima_alerts WHERE id = $1
	`
	var a alerts.Alert
	var engine, severity, status string
	var evidenceJSON []byte
	err := s.pool.QueryRow(ctx, q, id).Scan(
		&a.ID, &a.Fingerprint, &a.ServerID, &a.ServerName, &engine, &severity, &status, &a.Category, &a.Title, &a.Description, &evidenceJSON,
		&a.FirstSeenAt, &a.LastSeenAt, &a.HitCount, &a.AcknowledgedBy, &a.AcknowledgedAt, &a.ResolvedBy, &a.ResolvedAt, &a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		return a, err
	}
	a.Engine = alerts.Engine(engine)
	a.Severity = alerts.Severity(severity)
	a.Status = alerts.Status(status)
	_ = json.Unmarshal(evidenceJSON, &a.Evidence)
	return a, nil
}

func (s *TimescaleAlertStore) List(ctx context.Context, f alerts.AlertFilter) ([]alerts.Alert, error) {
	q := `
		SELECT id, fingerprint, server_id, instance_name, engine, severity, status, category, title, description, evidence, first_seen_at, last_seen_at, hit_count, acknowledged_by, acknowledged_at, resolved_by, resolved_at, created_at, updated_at
		FROM optima_alerts
		WHERE 1=1
	`
	args := []interface{}{}
	argIdx := 1

	if f.ServerID != nil {
		q += fmt.Sprintf(" AND server_id = $%d", argIdx)
		args = append(args, *f.ServerID)
		argIdx++
	}
	if f.Engine != nil {
		q += fmt.Sprintf(" AND engine = $%d", argIdx)
		args = append(args, string(*f.Engine))
		argIdx++
	}
	if f.Severity != nil {
		q += fmt.Sprintf(" AND severity = $%d", argIdx)
		args = append(args, string(*f.Severity))
		argIdx++
	}
	if f.Status != nil {
		q += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, string(*f.Status))
		argIdx++
	}
	if f.Category != nil && *f.Category != "" {
		q += fmt.Sprintf(" AND category = $%d", argIdx)
		args = append(args, *f.Category)
	}

	q += " ORDER BY last_seen_at DESC"
	if f.Limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", f.Limit)
	}

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []alerts.Alert
	for rows.Next() {
		var a alerts.Alert
		var engine, severity, status string
		var evidenceJSON []byte
		err := rows.Scan(
			&a.ID, &a.Fingerprint, &a.ServerID, &a.ServerName, &engine, &severity, &status, &a.Category, &a.Title, &a.Description, &evidenceJSON,
			&a.FirstSeenAt, &a.LastSeenAt, &a.HitCount, &a.AcknowledgedBy, &a.AcknowledgedAt, &a.ResolvedBy, &a.ResolvedAt, &a.CreatedAt, &a.UpdatedAt,
		)
		if err != nil {
			continue
		}
		a.Engine = alerts.Engine(engine)
		a.Severity = alerts.Severity(severity)
		a.Status = alerts.Status(status)
		_ = json.Unmarshal(evidenceJSON, &a.Evidence)
		out = append(out, a)
	}
	return out, nil
}

func (s *TimescaleAlertStore) UpdateStatus(ctx context.Context, id uuid.UUID, status alerts.Status, actor string, reason string, at time.Time) error {
	var q string
	if status == alerts.StatusAcknowledged {
		q = "UPDATE optima_alerts SET status = $2, acknowledged_by = $3, acknowledged_at = $4, updated_at = $4 WHERE id = $1"
	} else if status == alerts.StatusResolved {
		q = "UPDATE optima_alerts SET status = $2, resolved_by = $3, resolved_at = $4, updated_at = $4 WHERE id = $1"
	} else {
		q = "UPDATE optima_alerts SET status = $2, updated_at = $4 WHERE id = $1"
	}
	_, err := s.pool.Exec(ctx, q, id, string(status), actor, at)
	return err
}

func (s *TimescaleAlertStore) PruneResolved(ctx context.Context, olderThan time.Duration) (int64, error) {
	t := time.Now().UTC().Add(-olderThan)
	res, err := s.pool.Exec(ctx, "DELETE FROM optima_alerts WHERE status = 'resolved' AND resolved_at < $1", t)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected(), nil
}

func (s *TimescaleAlertStore) CountOpen(ctx context.Context, serverID uuid.UUID) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, "SELECT count(*) FROM optima_alerts WHERE server_id = $1 AND status IN ('open', 'acknowledged')", serverID).Scan(&count)
	return count, err
}

func (s *TimescaleAlertStore) ResolveByFingerprint(ctx context.Context, fingerprint, actor, reason string, at time.Time) (bool, error) {
	res, err := s.pool.Exec(ctx, `
		UPDATE optima_alerts
		SET status = 'resolved', resolved_by = $2, resolved_at = $3, updated_at = $3
		WHERE fingerprint = $1 AND status IN ('open', 'acknowledged')
	`, fingerprint, actor, at)
	if err != nil {
		return false, err
	}
	return res.RowsAffected() > 0, nil
}
