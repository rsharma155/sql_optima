// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: TimescaleDB-backed AlertStore implementation – upsert, list, status
//
//	transitions with audit history.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rsharma155/sql_optima/internal/domain/alerts"
)

// AlertRepository implements alerts.AlertStore backed by TimescaleDB / PostgreSQL.
type AlertRepository struct {
	pool *pgxpool.Pool
}

func NewAlertRepository(pool *pgxpool.Pool) *AlertRepository {
	return &AlertRepository{pool: pool}
}

// Upsert creates a new alert or bumps the hit count of an existing open
// alert with the same fingerprint. Relies on the partial unique index
// idx_alerts_open_fingerprint (fingerprint WHERE status IN ('open','acknowledged')).
func (r *AlertRepository) Upsert(ctx context.Context, a alerts.Alert) (alerts.Alert, error) {
	evidenceJSON, err := json.Marshal(a.Evidence)
	if err != nil {
		evidenceJSON = []byte("{}")
	}

	const q = `
		INSERT INTO optima_alerts (
			id, fingerprint, server_id, instance_name, engine,
			severity, status, category, title, description,
			evidence, first_seen_at, last_seen_at, hit_count
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8, $9, $10,
			$11::jsonb, $12, $13, $14
		)
		ON CONFLICT (fingerprint) WHERE status IN ('open', 'acknowledged')
		DO UPDATE SET
			last_seen_at = EXCLUDED.last_seen_at,
			hit_count    = optima_alerts.hit_count + 1,
			severity     = EXCLUDED.severity,
			evidence     = EXCLUDED.evidence,
			updated_at   = now()
		RETURNING id, first_seen_at, last_seen_at, hit_count, created_at, updated_at`

	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}

	err = r.pool.QueryRow(ctx, q,
		a.ID, a.Fingerprint, a.ServerID, a.InstanceName, string(a.Engine),
		string(a.Severity), string(a.Status), a.Category, a.Title, a.Description,
		evidenceJSON, a.FirstSeenAt, a.LastSeenAt, a.HitCount,
	).Scan(&a.ID, &a.FirstSeenAt, &a.LastSeenAt, &a.HitCount, &a.CreatedAt, &a.UpdatedAt)

	return a, err
}

// FindOpenByFingerprint returns the existing open alert for a given fingerprint, if any.
func (r *AlertRepository) FindOpenByFingerprint(ctx context.Context, fp string) (alerts.Alert, bool, error) {
	const q = `
		SELECT id, fingerprint, server_id, instance_name, engine,
		       severity, status, category, title, description,
		       evidence, first_seen_at, last_seen_at, hit_count,
		       acknowledged_by, acknowledged_at, resolved_by, resolved_at,
		       created_at, updated_at
		FROM optima_alerts
		WHERE fingerprint = $1 AND status != 'resolved'
		ORDER BY last_seen_at DESC
		LIMIT 1`

	var a alerts.Alert
	var evidenceJSON []byte
	err := r.pool.QueryRow(ctx, q, fp).Scan(
		&a.ID, &a.Fingerprint, &a.ServerID, &a.InstanceName, &a.Engine,
		&a.Severity, &a.Status, &a.Category, &a.Title, &a.Description,
		&evidenceJSON, &a.FirstSeenAt, &a.LastSeenAt, &a.HitCount,
		&a.AcknowledgedBy, &a.AcknowledgedAt, &a.ResolvedBy, &a.ResolvedAt,
		&a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		if err.Error() == "no rows in result set" {
			return a, false, nil
		}
		return a, false, err
	}
	if len(evidenceJSON) > 0 {
		_ = json.Unmarshal(evidenceJSON, &a.Evidence)
	}
	return a, true, nil
}

// GetByID fetches a single alert by its UUID.
func (r *AlertRepository) GetByID(ctx context.Context, id uuid.UUID) (alerts.Alert, error) {
	const q = `
		SELECT id, fingerprint, server_id, instance_name, engine,
		       severity, status, category, title, description,
		       evidence, first_seen_at, last_seen_at, hit_count,
		       acknowledged_by, acknowledged_at, resolved_by, resolved_at,
		       created_at, updated_at
		FROM optima_alerts
		WHERE id = $1`

	var a alerts.Alert
	var evidenceJSON []byte
	err := r.pool.QueryRow(ctx, q, id).Scan(
		&a.ID, &a.Fingerprint, &a.ServerID, &a.InstanceName, &a.Engine,
		&a.Severity, &a.Status, &a.Category, &a.Title, &a.Description,
		&evidenceJSON, &a.FirstSeenAt, &a.LastSeenAt, &a.HitCount,
		&a.AcknowledgedBy, &a.AcknowledgedAt, &a.ResolvedBy, &a.ResolvedAt,
		&a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		if err.Error() == "no rows in result set" {
			return a, alerts.ErrAlertNotFound
		}
		return a, err
	}
	if len(evidenceJSON) > 0 {
		_ = json.Unmarshal(evidenceJSON, &a.Evidence)
	}
	return a, nil
}

// List returns alerts matching the given filter.
func (r *AlertRepository) List(ctx context.Context, f alerts.AlertFilter) ([]alerts.Alert, int, error) {
	var (
		where []string
		args  []interface{}
		idx   = 1
	)

	addFilter := func(clause string, val interface{}) {
		where = append(where, fmt.Sprintf(clause, idx))
		args = append(args, val)
		idx++
	}

	if f.InstanceName != "" {
		addFilter("instance_name = $%d", f.InstanceName)
	}
	if f.Engine != "" && f.Engine.Valid() {
		addFilter("engine = $%d", string(f.Engine))
	}
	if f.Severity != "" && f.Severity.Valid() {
		addFilter("severity = $%d", string(f.Severity))
	}
	if f.Status != "" && f.Status.Valid() {
		addFilter("status = $%d", string(f.Status))
	}
	if f.Category != "" {
		addFilter("category = $%d", f.Category)
	}

	whereClause := ""
	if len(where) > 0 {
		whereClause = "WHERE " + strings.Join(where, " AND ")
	}

	// Count total
	countQ := "SELECT count(*) FROM optima_alerts " + whereClause
	var total int
	if err := r.pool.QueryRow(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Fetch page
	limit := f.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset := f.Offset
	if offset < 0 {
		offset = 0
	}

	dataQ := fmt.Sprintf(`
		SELECT id, fingerprint, server_id, instance_name, engine,
		       severity, status, category, title, description,
		       evidence, first_seen_at, last_seen_at, hit_count,
		       acknowledged_by, acknowledged_at, resolved_by, resolved_at,
		       created_at, updated_at
		FROM optima_alerts
		%s
		ORDER BY
			CASE severity WHEN 'critical' THEN 1 WHEN 'warning' THEN 2 ELSE 3 END,
			last_seen_at DESC
		LIMIT %d OFFSET %d`, whereClause, limit, offset)

	rows, err := r.pool.Query(ctx, dataQ, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var results []alerts.Alert
	for rows.Next() {
		var a alerts.Alert
		var evidenceJSON []byte
		if err := rows.Scan(
			&a.ID, &a.Fingerprint, &a.ServerID, &a.InstanceName, &a.Engine,
			&a.Severity, &a.Status, &a.Category, &a.Title, &a.Description,
			&evidenceJSON, &a.FirstSeenAt, &a.LastSeenAt, &a.HitCount,
			&a.AcknowledgedBy, &a.AcknowledgedAt, &a.ResolvedBy, &a.ResolvedAt,
			&a.CreatedAt, &a.UpdatedAt,
		); err != nil {
			return nil, 0, err
		}
		if len(evidenceJSON) > 0 {
			_ = json.Unmarshal(evidenceJSON, &a.Evidence)
		}
		results = append(results, a)
	}
	return results, total, rows.Err()
}

// UpdateStatus persists a status change and writes a history row in a single transaction.
func (r *AlertRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status alerts.Status, actor, reason string, at time.Time) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Fetch current status
	var oldStatus string
	err = tx.QueryRow(ctx,
		"SELECT status FROM optima_alerts WHERE id = $1 FOR UPDATE", id,
	).Scan(&oldStatus)
	if err != nil {
		if err.Error() == "no rows in result set" {
			return alerts.ErrAlertNotFound
		}
		return err
	}

	// Update alert
	updateQ := `UPDATE optima_alerts SET status = $1, updated_at = $2`
	updateArgs := []interface{}{string(status), at}
	argIdx := 3

	switch status {
	case alerts.StatusAcknowledged:
		updateQ += fmt.Sprintf(", acknowledged_by = $%d, acknowledged_at = $%d", argIdx, argIdx+1)
		updateArgs = append(updateArgs, actor, at)
		argIdx += 2
	case alerts.StatusResolved:
		updateQ += fmt.Sprintf(", resolved_by = $%d, resolved_at = $%d", argIdx, argIdx+1)
		updateArgs = append(updateArgs, actor, at)
		argIdx += 2
	}

	updateQ += fmt.Sprintf(" WHERE id = $%d", argIdx)
	updateArgs = append(updateArgs, id)

	if _, err := tx.Exec(ctx, updateQ, updateArgs...); err != nil {
		return err
	}

	// Insert history row
	_, err = tx.Exec(ctx,
		`INSERT INTO optima_alert_history (alert_id, old_status, new_status, changed_by, reason)
		 VALUES ($1, $2, $3, $4, $5)`,
		id, oldStatus, string(status), actor, reason,
	)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// CountOpen returns the number of non-resolved alerts for an instance.
func (r *AlertRepository) CountOpen(ctx context.Context, instanceName string) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx,
		"SELECT count(*) FROM optima_alerts WHERE instance_name = $1 AND status != 'resolved'",
		instanceName,
	).Scan(&count)
	return count, err
}
