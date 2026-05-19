// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Audit logging repository for administrative and operational actions.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AuditLogRepository struct {
	pool *pgxpool.Pool
}

func NewAuditLogRepository(pool *pgxpool.Pool) *AuditLogRepository {
	return &AuditLogRepository{pool: pool}
}

func (r *AuditLogRepository) Log(ctx context.Context, eventType string, serverID uuid.UUID, actor string, ipAddress string, metadata map[string]interface{}) error {
	metaJSON, _ := json.Marshal(metadata)
	q := `
		INSERT INTO optima_audit_logs (event_type, server_id, actor, ip_address, metadata)
		VALUES ($1, $2, $3, $4, $5)
	`
	_, err := r.pool.Exec(ctx, q, eventType, serverID, actor, ipAddress, metaJSON)
	return err
}
