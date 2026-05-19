// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose : Repository interface contract for the SQL Server HA & Replication
//           domain.  Defines the read-side methods used by API handlers.
//
// Author  : Ravi Sharma <ravisharma155@gmail.com>
// Created : 2026-05-14
// License : MIT
package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/rsharma155/sql_optima/internal/domain/sqlserver_ha_replication/domain"
)

// HAReplicationRepositoryInterface defines the read contract for the HA & Replication domain.
// All methods use server_id (UUID) not instance name — callers resolve the UUID upstream.
type HAReplicationRepositoryInterface interface {
	// Feature gate.
	// When database is "" or "all", returns instance-level detection.
	// When database is a specific name, checks per-DB HA membership and replication publisher status.
	GetFeatureDetection(ctx context.Context, serverID uuid.UUID, database string) (domain.FeatureDetection, error)

	// HA KPIs
	GetRPOTrend(ctx context.Context, serverID uuid.UUID, from, to time.Time) ([]map[string]interface{}, error)
	GetRTOTrend(ctx context.Context, serverID uuid.UUID, from, to time.Time) ([]map[string]interface{}, error)

	// Replica health
	GetCurrentReplicaHealth(ctx context.Context, serverID uuid.UUID) ([]domain.ReplicaHealthRow, error)
	GetCurrentDatabaseSyncState(ctx context.Context, serverID uuid.UUID) ([]domain.DatabaseSyncState, error)

	// Failover
	GetFailoverHistory(ctx context.Context, serverID uuid.UUID, from, to time.Time) ([]domain.FailoverEvent, error)

	// Replication
	GetReplicationKPIs(ctx context.Context, serverID uuid.UUID) (domain.ReplicationKPIs, error)
	GetReplicationTopology(ctx context.Context, serverID uuid.UUID) ([]domain.ReplicationTopologyRow, error)
	GetReplicationBacklogTrend(ctx context.Context, serverID uuid.UUID, from, to time.Time) ([]domain.ReplicationLatencyPoint, error)
	GetReplicationArticles(ctx context.Context, serverID uuid.UUID) ([]domain.ReplicationArticle, error)

	// Coverage
	GetDatabaseCoverage(ctx context.Context, serverID uuid.UUID) ([]domain.DatabaseCoverage, error)

	// Alerts
	GetAlertsTimeline(ctx context.Context, serverID uuid.UUID, from, to time.Time) ([]domain.AlertEvent, error)
}
