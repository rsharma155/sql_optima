// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Router for PostgreSQL query metrics collection, selecting between pg_stat_statements and pg_stat_monitor.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package postgres

import (
	"log/slog"
	"context"
	"database/sql"

	"github.com/rsharma155/sql_optima/internal/config"
	repo "github.com/rsharma155/sql_optima/internal/repository/postgres"
)

type QueryMetricsRouter struct {
	monitorRepo      *repo.PgStatMonitorRepository
	monitorCollector *PgStatMonitorCollector
	legacyCollector  *PgStatStatementsLegacyCollector
}

func NewQueryMetricsRouter(monitorRepo *repo.PgStatMonitorRepository, monitorCollector *PgStatMonitorCollector, legacyCollector *PgStatStatementsLegacyCollector) *QueryMetricsRouter {
	return &QueryMetricsRouter{
		monitorRepo:      monitorRepo,
		monitorCollector: monitorCollector,
		legacyCollector:  legacyCollector,
	}
}

func (r *QueryMetricsRouter) Collect(ctx context.Context, inst config.Instance, db *sql.DB) error {
	instanceName := inst.Name
	serverID := inst.ServerID

	// 1. Detect extension support
	hasMonitor, err := r.monitorRepo.CheckExtension(ctx, db)
	if err != nil {
		slog.Error("[QueryMetricsRouter] ERROR: Failed to check extension support", "target", instanceName, "err", err)
		return err
	}

	// 2. Update instance metadata
	if err := r.monitorRepo.UpdateInstanceMetadata(ctx, serverID, hasMonitor); err != nil {
		slog.Error("[QueryMetricsRouter] ERROR: Failed to update metadata", "target", instanceName, "err", err)
	}

	// 3. Delegate to appropriate collector
	if hasMonitor {
		return r.monitorCollector.Collect(ctx, inst, db)
	}

	// Fallback to legacy pg_stat_statements
	return r.legacyCollector.Collect(ctx, inst, db)
}
