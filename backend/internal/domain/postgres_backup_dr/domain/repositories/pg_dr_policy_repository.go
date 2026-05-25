// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: TimescaleDB persistence for optima_server_dr_policy (read and upsert
//          per monitored PostgreSQL instance RPO/RTO configuration).
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repositories

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rsharma155/sql_optima/internal/domain/postgres_backup_dr/domain/entities"
)

type DRPolicyRepository struct {
	pool *pgxpool.Pool
}

func NewDRPolicyRepository(pool *pgxpool.Pool) *DRPolicyRepository {
	return &DRPolicyRepository{pool: pool}
}

func (r *DRPolicyRepository) Get(ctx context.Context, serverID uuid.UUID) (entities.DRPolicy, error) {
	def := entities.DefaultDRPolicy(serverID)
	q := `
		SELECT server_id, rpo_backup_hours, rpo_archive_minutes, rpo_replay_seconds,
		       max_slot_retention_gb, rto_failover_minutes, updated_at, COALESCE(updated_by,'')
		FROM optima_server_dr_policy
		WHERE server_id = $1`
	var rto *int
	err := r.pool.QueryRow(ctx, q, serverID).Scan(
		&def.ServerID, &def.RPOBackupHours, &def.RPOArchiveMinutes, &def.RPOReplaySeconds,
		&def.MaxSlotRetentionGB, &rto, &def.UpdatedAt, &def.UpdatedBy,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return def, nil
	}
	if err != nil {
		return def, err
	}
	def.RTOFailoverMinutes = rto
	return def, nil
}

func (r *DRPolicyRepository) Upsert(ctx context.Context, p entities.DRPolicy, updatedBy string) error {
	q := `
		INSERT INTO optima_server_dr_policy (
			server_id, rpo_backup_hours, rpo_archive_minutes, rpo_replay_seconds,
			max_slot_retention_gb, rto_failover_minutes, updated_at, updated_by
		) VALUES ($1,$2,$3,$4,$5,$6,now(),$7)
		ON CONFLICT (server_id) DO UPDATE SET
			rpo_backup_hours = EXCLUDED.rpo_backup_hours,
			rpo_archive_minutes = EXCLUDED.rpo_archive_minutes,
			rpo_replay_seconds = EXCLUDED.rpo_replay_seconds,
			max_slot_retention_gb = EXCLUDED.max_slot_retention_gb,
			rto_failover_minutes = EXCLUDED.rto_failover_minutes,
			updated_at = now(),
			updated_by = EXCLUDED.updated_by`
	_, err := r.pool.Exec(ctx, q,
		p.ServerID, p.RPOBackupHours, p.RPOArchiveMinutes, p.RPOReplaySeconds,
		p.MaxSlotRetentionGB, p.RTOFailoverMinutes, updatedBy,
	)
	return err
}
