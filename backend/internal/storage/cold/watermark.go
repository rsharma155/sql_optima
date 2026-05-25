// SQL Optima — https://github.com/rsharma155/sql_optima
//
// File: backend/internal/storage/cold/watermark.go
// Purpose: Persistent tracking of archival progress (high-water marks) to ensure at-least-once delivery.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package cold

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// DB is an interface that matches both pgxpool.Pool and pgxmock.Pool
type DB interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (commandTag pgconn.CommandTag, err error)
}

// WatermarkStore tracks the last successfully exported timestamp per table
// and server, persisted in TimescaleDB so it survives restarts.
type WatermarkStore struct {
	db DB
}

// NewWatermarkStore creates a new WatermarkStore.
func NewWatermarkStore(db DB) *WatermarkStore {
	return &WatermarkStore{db: db}
}

// Get returns the last exported-at timestamp for a table+server combination.
// Returns the zero time if no watermark exists (meaning: export from the beginning).
func (s *WatermarkStore) Get(ctx context.Context, tableName, serverID string) (time.Time, error) {
	const q = `
		SELECT last_exported_at
		FROM coldstorage.watermarks
		WHERE table_name = $1 AND server_id = $2`

	var t time.Time
	err := s.db.QueryRow(ctx, q, tableName, serverID).Scan(&t)
	if err != nil {
		if err == pgx.ErrNoRows {
			return time.Time{}, nil
		}
		return time.Time{}, fmt.Errorf("watermark/get %s/%s: %w", tableName, serverID, err)
	}
	return t, nil
}

// Set updates (upserts) the watermark for a table+server after a successful export.
func (s *WatermarkStore) Set(ctx context.Context, tableName, serverID string, exportedAt time.Time) error {
	const q = `
		INSERT INTO coldstorage.watermarks (table_name, server_id, last_exported_at, updated_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (table_name, server_id)
		DO UPDATE SET last_exported_at = EXCLUDED.last_exported_at,
		              updated_at       = EXCLUDED.updated_at`

	_, err := s.db.Exec(ctx, q, tableName, serverID, exportedAt)
	if err != nil {
		return fmt.Errorf("watermark/set %s/%s: %w", tableName, serverID, err)
	}
	return nil
}
