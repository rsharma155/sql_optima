// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL Backup & DR data collectors.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package collectors

import (
	"context"
	"database/sql"

	"github.com/google/uuid"
	"github.com/rsharma155/sql_optima/internal/domain/postgres_backup_dr/domain/repositories"
)

type PostgresBackupCollector struct {
	repo       *repositories.PostgresBackupRepository
	serverID uuid.UUID
}

func NewPostgresBackupCollector(repo *repositories.PostgresBackupRepository, serverID uuid.UUID) *PostgresBackupCollector {
	return &PostgresBackupCollector{
		repo:       repo,
		serverID: serverID,
	}
}

// Collect calls the unified database collector function.
func (c *PostgresBackupCollector) Collect(ctx context.Context, db *sql.DB) error {
	return c.repo.CollectBackupDR(ctx, c.serverID)
}

// Deprecated methods kept for backward compatibility if needed

func (c *PostgresBackupCollector) CollectArchiverStats(ctx context.Context, db *sql.DB) error {
	return c.Collect(ctx, db)
}

func (c *PostgresBackupCollector) CollectWALRate(ctx context.Context, db *sql.DB) error {
	return c.Collect(ctx, db)
}

func (c *PostgresBackupCollector) CollectBaseBackupHistory(ctx context.Context, db *sql.DB) error {
	return c.Collect(ctx, db)
}
