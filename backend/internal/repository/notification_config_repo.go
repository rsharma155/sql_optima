// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Repository for managing outbound notification channel config in TimescaleDB.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NotificationChannelConfig is one row from optima_notification_config.
type NotificationChannelConfig struct {
	ID        int       `json:"id"`
	Channel   string    `json:"channel"`
	URL       string    `json:"url"`
	IsEnabled bool      `json:"is_enabled"`
	UpdatedAt time.Time `json:"updated_at"`
	UpdatedBy string    `json:"updated_by"`
}

type NotificationConfigRepository struct {
	pool *pgxpool.Pool
}

func NewNotificationConfigRepository(pool *pgxpool.Pool) *NotificationConfigRepository {
	return &NotificationConfigRepository{pool: pool}
}

func (r *NotificationConfigRepository) ListAll(ctx context.Context) ([]NotificationChannelConfig, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, channel, url, is_enabled, updated_at, COALESCE(updated_by,'')
		FROM optima_notification_config
		ORDER BY channel`)
	if err != nil {
		if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == "42P01" {
			// Table not yet created — schema migration pending; return empty.
			return []NotificationChannelConfig{}, nil
		}
		return nil, err
	}
	defer rows.Close()

	var out []NotificationChannelConfig
	for rows.Next() {
		var c NotificationChannelConfig
		if err := rows.Scan(&c.ID, &c.Channel, &c.URL, &c.IsEnabled, &c.UpdatedAt, &c.UpdatedBy); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *NotificationConfigRepository) Upsert(ctx context.Context, channel, url string, enabled bool, updatedBy string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO optima_notification_config (channel, url, is_enabled, updated_at, updated_by)
		VALUES ($1, $2, $3, now(), $4)
		ON CONFLICT (channel) DO UPDATE SET
			url        = EXCLUDED.url,
			is_enabled = EXCLUDED.is_enabled,
			updated_at = now(),
			updated_by = EXCLUDED.updated_by`,
		channel, url, enabled, updatedBy)
	return err
}
