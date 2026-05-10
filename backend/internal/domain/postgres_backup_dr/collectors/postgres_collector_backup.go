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
	"github.com/rsharma155/sql_optima/internal/domain/postgres_backup_dr/domain/repositories"
)

type PostgresBackupCollector struct {
	repo       *repositories.PostgresBackupRepository
	instanceID string
}

func NewPostgresBackupCollector(repo *repositories.PostgresBackupRepository, instanceID string) *PostgresBackupCollector {
	return &PostgresBackupCollector{
		repo:       repo,
		instanceID: instanceID,
	}
}

// Collect calls the unified database collector function.
func (c *PostgresBackupCollector) Collect(ctx context.Context, db *sql.DB) error {
	return c.repo.CollectBackupDR(ctx, c.instanceID)
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
