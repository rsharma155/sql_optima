// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: TimescaleDB-backed MaintenanceStore and AlertRuleStore implementations.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rsharma155/sql_optima/internal/domain/alerts"
)

// AlertMaintenanceRepository implements alerts.MaintenanceStore.
type AlertMaintenanceRepository struct {
	pool *pgxpool.Pool
}

func NewAlertMaintenanceRepository(pool *pgxpool.Pool) *AlertMaintenanceRepository {
	return &AlertMaintenanceRepository{pool: pool}
}

func (r *AlertMaintenanceRepository) Create(ctx context.Context, mw alerts.MaintenanceWindow) (alerts.MaintenanceWindow, error) {
	if mw.ID == uuid.Nil {
		mw.ID = uuid.New()
	}

	const q = `
		INSERT INTO optima_maintenance_windows (id, instance_name, engine, reason, starts_at, ends_at, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING created_at`

	err := r.pool.QueryRow(ctx, q,
		mw.ID, mw.InstanceName, string(mw.Engine), mw.Reason,
		mw.StartsAt, mw.EndsAt, mw.CreatedBy,
	).Scan(&mw.CreatedAt)
	return mw, err
}

func (r *AlertMaintenanceRepository) IsUnderMaintenance(ctx context.Context, instanceName string, engine alerts.Engine, at time.Time) (bool, error) {
	const q = `
		SELECT EXISTS(
			SELECT 1 FROM optima_maintenance_windows
			WHERE instance_name = $1
			  AND engine = $2
			  AND starts_at <= $3
			  AND ends_at > $3
		)`

	var exists bool
	err := r.pool.QueryRow(ctx, q, instanceName, string(engine), at).Scan(&exists)
	return exists, err
}

func (r *AlertMaintenanceRepository) ListActive(ctx context.Context, at time.Time) ([]alerts.MaintenanceWindow, error) {
	const q = `
		SELECT id, instance_name, engine, reason, starts_at, ends_at, created_by, created_at
		FROM optima_maintenance_windows
		WHERE starts_at <= $1 AND ends_at > $1
		ORDER BY ends_at ASC`

	rows, err := r.pool.Query(ctx, q, at)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []alerts.MaintenanceWindow
	for rows.Next() {
		var mw alerts.MaintenanceWindow
		if err := rows.Scan(
			&mw.ID, &mw.InstanceName, &mw.Engine, &mw.Reason,
			&mw.StartsAt, &mw.EndsAt, &mw.CreatedBy, &mw.CreatedAt,
		); err != nil {
			return nil, err
		}
		result = append(result, mw)
	}
	return result, rows.Err()
}

func (r *AlertMaintenanceRepository) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, "DELETE FROM optima_maintenance_windows WHERE id = $1", id)
	return err
}

// AlertRuleRepository implements alerts.AlertRuleStore.
type AlertRuleRepository struct {
	pool *pgxpool.Pool
}

func NewAlertRuleRepository(pool *pgxpool.Pool) *AlertRuleRepository {
	return &AlertRuleRepository{pool: pool}
}

func (r *AlertRuleRepository) ListEnabled(ctx context.Context, engine alerts.Engine) ([]alerts.AlertRule, error) {
	const q = `
		SELECT id, name, engine, category, default_severity, description, is_enabled, config, created_at, updated_at
		FROM optima_alert_rules
		WHERE is_enabled = true AND (engine = $1 OR engine = 'all')
		ORDER BY name`

	rows, err := r.pool.Query(ctx, q, string(engine))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []alerts.AlertRule
	for rows.Next() {
		var ar alerts.AlertRule
		var configJSON []byte
		if err := rows.Scan(
			&ar.ID, &ar.Name, &ar.Engine, &ar.Category,
			&ar.DefaultSeverity, &ar.Description, &ar.IsEnabled,
			&configJSON, &ar.CreatedAt, &ar.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if len(configJSON) > 0 {
			_ = json.Unmarshal(configJSON, &ar.Config)
		}
		result = append(result, ar)
	}
	return result, rows.Err()
}

func (r *AlertRuleRepository) GetByName(ctx context.Context, name string) (alerts.AlertRule, error) {
	const q = `
		SELECT id, name, engine, category, default_severity, description, is_enabled, config, created_at, updated_at
		FROM optima_alert_rules
		WHERE name = $1`

	var ar alerts.AlertRule
	var configJSON []byte
	err := r.pool.QueryRow(ctx, q, name).Scan(
		&ar.ID, &ar.Name, &ar.Engine, &ar.Category,
		&ar.DefaultSeverity, &ar.Description, &ar.IsEnabled,
		&configJSON, &ar.CreatedAt, &ar.UpdatedAt,
	)
	if err != nil {
		return ar, err
	}
	if len(configJSON) > 0 {
		_ = json.Unmarshal(configJSON, &ar.Config)
	}
	return ar, nil
}
