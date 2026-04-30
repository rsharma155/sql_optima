// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Router for PostgreSQL query metrics collection, choosing between pg_stat_monitor and pg_stat_statements.
//
// Metadata:
//   Type: Collector Orchestrator
//   Package: postgres
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package postgres

import (
	"context"
	"database/sql"
	"log"

	repo "github.com/rsharma155/sql_optima/internal/repository/postgres"
)

type QueryMetricsRouter struct {
	detector          *ExtensionDetector
	monitorCollector  *PgStatMonitorCollector
	legacyCollector   *PgStatStatementsLegacyCollector
	monitorRepo       *repo.PgStatMonitorRepository
	tsDB              *sql.DB
}

func NewQueryMetricsRouter(monitor *PgStatMonitorCollector, legacy *PgStatStatementsLegacyCollector, monitorRepo *repo.PgStatMonitorRepository, tsDB *sql.DB) *QueryMetricsRouter {
	return &QueryMetricsRouter{
		detector:         NewExtensionDetector(),
		monitorCollector: monitor,
		legacyCollector:  legacy,
		monitorRepo:      monitorRepo,
		tsDB:             tsDB,
	}
}

func (r *QueryMetricsRouter) CollectQueryMetrics(ctx context.Context, instanceName string, db *sql.DB) error {
	source := r.detector.GetSource(ctx, db)

	// Persist source in metadata table (best effort)
	if r.tsDB != nil {
		_ = r.monitorRepo.UpdateInstanceMetadata(ctx, r.tsDB, instanceName, string(source))
	}

	switch source {
	case PgStatMonitor:
		log.Printf("[PgQueryMetricsRouter] Using pg_stat_monitor for %s", instanceName)
		return r.monitorCollector.Collect(ctx, instanceName, db)
	case PgStatStatements:
		log.Printf("[PgQueryMetricsRouter] Using pg_stat_statements fallback for %s", instanceName)
		return r.legacyCollector.Collect(ctx, instanceName, db)
	default:
		log.Printf("[PgQueryMetricsRouter] WARN: No query statistics extension enabled for %s", instanceName)
		return nil
	}
}
