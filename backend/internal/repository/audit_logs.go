package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AuditLogRepository struct {
	pool *pgxpool.Pool
}

func NewAuditLogRepository(pool *pgxpool.Pool) *AuditLogRepository {
	return &AuditLogRepository{pool: pool}
}

func (r *AuditLogRepository) Log(ctx context.Context, eventType string, serverID string, actor string, ipAddress string, metadata map[string]interface{}) error {
	if r == nil || r.pool == nil {
		return fmt.Errorf("timescale not configured")
	}
	eventType = strings.TrimSpace(eventType)
	if eventType == "" {
		return fmt.Errorf("event_type is required")
	}

	var sid *uuid.UUID
	if strings.TrimSpace(serverID) != "" {
		u, err := uuid.Parse(strings.TrimSpace(serverID))
		if err == nil {
			sid = &u
		}
	}

	metaJSON := []byte("{}")
	if metadata != nil {
		if b, err := json.Marshal(metadata); err == nil {
			metaJSON = b
		}
	}

	_, err := r.pool.Exec(ctx, `
		INSERT INTO optima_audit_logs(event_type, server_id, actor, ip_address, metadata)
		VALUES ($1,$2,$3,$4,$5::jsonb)
	`, eventType, sid, strings.TrimSpace(actor), strings.TrimSpace(ipAddress), string(metaJSON))
	return err
}
