// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Repository for managing collector configurations in TimescaleDB.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rsharma155/sql_optima/internal/models"
)

type CollectorConfigRepository struct {
	pool *pgxpool.Pool
}

func NewCollectorConfigRepository(pool *pgxpool.Pool) *CollectorConfigRepository {
	return &CollectorConfigRepository{pool: pool}
}

func (r *CollectorConfigRepository) ListAll(ctx context.Context) ([]models.CollectorConfig, error) {
	query := `SELECT id, collector_name, module, frequency_seconds, is_active, updated_at, COALESCE(updated_by, ''), run_order
	          FROM optima_collector_configs
	          ORDER BY run_order NULLS LAST, module, collector_name`
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query error: %w", err)
	}
	defer rows.Close()

	var configs []models.CollectorConfig
	for rows.Next() {
		var c models.CollectorConfig
		if err := rows.Scan(&c.ID, &c.CollectorName, &c.Module, &c.FrequencySeconds, &c.IsActive, &c.UpdatedAt, &c.UpdatedBy, &c.RunOrder); err != nil {
			return nil, fmt.Errorf("scan error: %w", err)
		}
		configs = append(configs, c)
	}
	return configs, nil
}

func (r *CollectorConfigRepository) UpdateFrequency(ctx context.Context, id int, frequency int, updatedBy string) error {
	query := `UPDATE optima_collector_configs SET frequency_seconds = $1, updated_at = now(), updated_by = $2 WHERE id = $3`
	_, err := r.pool.Exec(ctx, query, frequency, updatedBy, id)
	return err
}

func (r *CollectorConfigRepository) GetActiveConfigs(ctx context.Context) ([]models.CollectorConfig, error) {
	query := `SELECT id, collector_name, module, frequency_seconds, is_active, updated_at, COALESCE(updated_by, ''), run_order
	          FROM optima_collector_configs WHERE is_active = true`
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query error: %w", err)
	}
	defer rows.Close()

	var configs []models.CollectorConfig
	for rows.Next() {
		var c models.CollectorConfig
		if err := rows.Scan(&c.ID, &c.CollectorName, &c.Module, &c.FrequencySeconds, &c.IsActive, &c.UpdatedAt, &c.UpdatedBy, &c.RunOrder); err != nil {
			return nil, fmt.Errorf("scan error: %w", err)
		}
		configs = append(configs, c)
	}
	return configs, nil
}

func (r *CollectorConfigRepository) GetByName(ctx context.Context, name string) (*models.CollectorConfig, error) {
	query := `SELECT id, collector_name, module, frequency_seconds, is_active, updated_at, COALESCE(updated_by, ''), run_order FROM optima_collector_configs WHERE collector_name = $1`
	var c models.CollectorConfig
	err := r.pool.QueryRow(ctx, query, name).Scan(&c.ID, &c.CollectorName, &c.Module, &c.FrequencySeconds, &c.IsActive, &c.UpdatedAt, &c.UpdatedBy, &c.RunOrder)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// InsertDefault inserts a new collector config row with default values only if the
// collector_name does not already exist. It is safe to call on every startup because
// ON CONFLICT DO NOTHING makes it idempotent.
func (r *CollectorConfigRepository) InsertDefault(ctx context.Context, name, module string, freqSec int) error {
	query := `INSERT INTO optima_collector_configs (collector_name, module, frequency_seconds, is_active)
	          VALUES ($1, $2, $3, true)
	          ON CONFLICT (collector_name) DO NOTHING`
	_, err := r.pool.Exec(ctx, query, name, module, freqSec)
	return err
}
