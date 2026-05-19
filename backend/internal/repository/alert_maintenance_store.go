// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Repository implementation for alert rules and maintenance windows.
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

type AlertMaintenanceStore struct {
	pool *pgxpool.Pool
}

func NewAlertMaintenanceStore(pool *pgxpool.Pool) *AlertMaintenanceStore {
	return &AlertMaintenanceStore{pool: pool}
}

func (s *AlertMaintenanceStore) Create(ctx context.Context, mw alerts.MaintenanceWindow) (alerts.MaintenanceWindow, error) {
	q := `
		INSERT INTO optima_maintenance_windows (
			server_id, engine, category, start_time, end_time, description, created_by
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at
	`
	err := s.pool.QueryRow(ctx, q,
		mw.ServerID, string(mw.Engine), mw.Category,
		mw.StartTime, mw.EndTime, mw.Description, mw.CreatedBy,
	).Scan(&mw.ID, &mw.CreatedAt)

	return mw, err
}

func (s *AlertMaintenanceStore) IsUnderMaintenance(ctx context.Context, serverID uuid.UUID, engine alerts.Engine, at time.Time) (bool, error) {
	q := `
		SELECT EXISTS (
			SELECT 1 FROM optima_maintenance_windows
			WHERE server_id = $1 AND engine = $2
			  AND $3 BETWEEN start_time AND end_time
		)
	`
	var active bool
	err := s.pool.QueryRow(ctx, q, serverID, string(engine), at).Scan(&active)
	return active, err
}

func (s *AlertMaintenanceStore) ListActive(ctx context.Context, at time.Time) ([]alerts.MaintenanceWindow, error) {
	q := `
		SELECT id, server_id, engine, category, start_time, end_time, description, created_by, created_at
		FROM optima_maintenance_windows
		WHERE $1 BETWEEN start_time AND end_time
	`
	rows, err := s.pool.Query(ctx, q, at)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []alerts.MaintenanceWindow
	for rows.Next() {
		var mw alerts.MaintenanceWindow
		var eng string
		err := rows.Scan(
			&mw.ID, &mw.ServerID, &eng, &mw.Category,
			&mw.StartTime, &mw.EndTime, &mw.Description, &mw.CreatedBy, &mw.CreatedAt,
		)
		if err != nil {
			continue
		}
		mw.Engine = alerts.Engine(eng)
		out = append(out, mw)
	}
	return out, nil
}

func (s *AlertMaintenanceStore) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := s.pool.Exec(ctx, "DELETE FROM optima_maintenance_windows WHERE id = $1", id)
	return err
}

// Alert Rule Methods

func (s *AlertMaintenanceStore) GetBuiltInRules(ctx context.Context, engine alerts.Engine) ([]alerts.AlertRule, error) {
	q := `
		SELECT name, engine, category, default_severity, description, config, is_enabled
		FROM optima_alert_rules
		WHERE engine = $1
	`
	rows, err := s.pool.Query(ctx, q, string(engine))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []alerts.AlertRule
	for rows.Next() {
		var r alerts.AlertRule
		var eng, sev string
		var configJSON []byte
		err := rows.Scan(&r.Name, &eng, &r.Category, &sev, &r.Description, &configJSON, &r.IsEnabled)
		if err != nil {
			continue
		}
		r.Engine = alerts.Engine(eng)
		r.Severity = alerts.Severity(sev)
		_ = json.Unmarshal(configJSON, &r.Config)
		out = append(out, r)
	}
	return out, nil
}

func (s *AlertMaintenanceStore) UpdateRuleConfig(ctx context.Context, name string, config map[string]interface{}) error {
	configJSON, _ := json.Marshal(config)
	_, err := s.pool.Exec(ctx, `
		UPDATE optima_alert_rules SET config = $2, updated_at = now()
		WHERE name = $1
	`, name, configJSON)
	return err
}
