// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Read/write SQL Server backup RPO policy in optima_server_dr_policy.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repositories

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rsharma155/sql_optima/internal/domain/sqlserver_backup_recovery/domain"
)

type SQLServerBackupPolicyRepository struct {
	pool *pgxpool.Pool
}

func NewSQLServerBackupPolicyRepository(pool *pgxpool.Pool) *SQLServerBackupPolicyRepository {
	return &SQLServerBackupPolicyRepository{pool: pool}
}

func (r *SQLServerBackupPolicyRepository) ensureLogBackupColumn(ctx context.Context) {
	if r.pool == nil {
		return
	}
	_, _ = r.pool.Exec(ctx, `
		ALTER TABLE optima_server_dr_policy
		ADD COLUMN IF NOT EXISTS rpo_log_backup_minutes INT NOT NULL DEFAULT 15`)
}

func isMissingRelationOrColumn(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "does not exist") ||
		strings.Contains(msg, "undefined_table") ||
		strings.Contains(msg, "undefined_column")
}

func (r *SQLServerBackupPolicyRepository) Get(ctx context.Context, serverID uuid.UUID) (domain.BackupPolicy, error) {
	p := domain.DefaultBackupPolicy(serverID)
	if r.pool == nil {
		return p, nil
	}
	r.ensureLogBackupColumn(ctx)

	const fullQ = `
		SELECT server_id, rpo_backup_hours,
		       COALESCE(rpo_log_backup_minutes, 15),
		       updated_at, COALESCE(updated_by, '')
		FROM optima_server_dr_policy
		WHERE server_id = $1`
	err := r.pool.QueryRow(ctx, fullQ, serverID).Scan(
		&p.ServerID, &p.RPOFullBackupHours, &p.RPOLogBackupMinutes, &p.UpdatedAt, &p.UpdatedBy,
	)
	if err == nil {
		return p, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return p, nil
	}
	if isMissingRelationOrColumn(err) {
		const legacyQ = `
			SELECT server_id, rpo_backup_hours, updated_at, COALESCE(updated_by, '')
			FROM optima_server_dr_policy
			WHERE server_id = $1`
		err2 := r.pool.QueryRow(ctx, legacyQ, serverID).Scan(
			&p.ServerID, &p.RPOFullBackupHours, &p.UpdatedAt, &p.UpdatedBy,
		)
		if errors.Is(err2, pgx.ErrNoRows) || isMissingRelationOrColumn(err2) {
			return p, nil
		}
		if err2 != nil {
			return p, err2
		}
		return p, nil
	}
	return p, err
}

func (r *SQLServerBackupPolicyRepository) Upsert(ctx context.Context, p domain.BackupPolicy, updatedBy string) error {
	if r.pool == nil {
		return errors.New("database unavailable")
	}
	if p.RPOFullBackupHours < 1 || p.RPOFullBackupHours > 168 {
		return errors.New("rpo_full_backup_hours must be between 1 and 168")
	}
	if p.RPOLogBackupMinutes < 5 || p.RPOLogBackupMinutes > 1440 {
		return errors.New("rpo_log_backup_minutes must be between 5 and 1440")
	}
	r.ensureLogBackupColumn(ctx)

	const fullQ = `
		INSERT INTO optima_server_dr_policy (
			server_id, rpo_backup_hours, rpo_archive_minutes, rpo_replay_seconds,
			max_slot_retention_gb, rpo_log_backup_minutes, updated_at, updated_by
		) VALUES ($1, $2, 5, 60, 10, $3, now(), $4)
		ON CONFLICT (server_id) DO UPDATE SET
			rpo_backup_hours = EXCLUDED.rpo_backup_hours,
			rpo_log_backup_minutes = EXCLUDED.rpo_log_backup_minutes,
			updated_at = now(),
			updated_by = EXCLUDED.updated_by`
	_, err := r.pool.Exec(ctx, fullQ, p.ServerID, p.RPOFullBackupHours, p.RPOLogBackupMinutes, updatedBy)
	if err == nil {
		return nil
	}
	if !isMissingRelationOrColumn(err) {
		return err
	}
	const legacyQ = `
		INSERT INTO optima_server_dr_policy (
			server_id, rpo_backup_hours, rpo_archive_minutes, rpo_replay_seconds,
			max_slot_retention_gb, updated_at, updated_by
		) VALUES ($1, $2, 5, 60, 10, now(), $3)
		ON CONFLICT (server_id) DO UPDATE SET
			rpo_backup_hours = EXCLUDED.rpo_backup_hours,
			updated_at = now(),
			updated_by = EXCLUDED.updated_by`
	_, err2 := r.pool.Exec(ctx, legacyQ, p.ServerID, p.RPOFullBackupHours, updatedBy)
	return err2
}
