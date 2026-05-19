// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Collector for pg_stat_monitor bucketed query metrics.
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
	"github.com/rsharma155/sql_optima/internal/domain/postgres_monitoring/instance_health"
	repo "github.com/rsharma155/sql_optima/internal/repository/postgres"
)

type PgStatMonitorCollector struct {
	repo       *repo.PgStatMonitorRepository
	tsDB       *sql.DB
	healthRepo *instance_health.InstanceHealthRepository
}

func NewPgStatMonitorCollector(monitorRepo *repo.PgStatMonitorRepository, tsDB *sql.DB, healthRepo *instance_health.InstanceHealthRepository) *PgStatMonitorCollector {
	return &PgStatMonitorCollector{
		repo:       monitorRepo,
		tsDB:       tsDB,
		healthRepo: healthRepo,
	}
}

func (c *PgStatMonitorCollector) Collect(ctx context.Context, inst config.Instance, db *sql.DB) error {
	instanceName := inst.Name
	serverID := inst.ServerID

	// 1. Detect last completed bucket in the target DB
	targetBucket, err := c.repo.GetLastCompletedBucket(ctx, db)
	if err != nil {
		slog.Error("[PgStatMonitorCollector] ERROR: Failed to get last completed bucket", "target", instanceName, "err", err)
		return err
	}

	// 2. Detect last collected bucket in TimescaleDB
	lastCollected, err := c.repo.GetLastCollectedBucket(ctx, c.tsDB, instanceName)
	if err != nil {
		slog.Error("[PgStatMonitorCollector] ERROR: Failed to get last collected bucket", "target", instanceName, "err", err)
		return err
	}

	// 3. Skip if already collected
	if targetBucket <= lastCollected {
		return nil
	}

	slog.Info("[PgStatMonitorCollector] INFO: Collecting bucket", "arg1", targetBucket, "arg2", instanceName)

	// 4. Fetch metrics for the bucket
	rows, err := c.repo.FetchBucketMetrics(ctx, db, targetBucket)
	if err != nil {
		slog.Error("[PgStatMonitorCollector] ERROR: Failed to fetch bucket metrics", "target", instanceName, "err", err)
		return err
	}

	// 5. Log metrics to hypertable
	if err := c.repo.LogBucketMetrics(ctx, c.tsDB, instanceName, rows); err != nil {
		slog.Error("[PgStatMonitorCollector] ERROR: Failed to log bucket metrics", "target", instanceName, "err", err)
		return err
	}

	// 6. Fetch and log pg_stat_monitor-specific aggregated metrics.
	// tps, cache_hit_ratio, wal_mb_per_min are intentionally omitted here — pg_snapshot_collector
	// already writes those every 60s. Writing them again from a different source would double the
	// rows in pg_ts_metrics and blend two different measurement methods when the table is queried
	// with time_bucket + avg().
	agg, err := c.repo.FetchAggregatedMetrics(ctx, db, targetBucket)
	if err != nil {
		slog.Error("[PgStatMonitorCollector] ERROR: Failed to fetch aggregated metrics", "target", instanceName, "err", err)
	} else if agg != nil {
		_ = c.healthRepo.LogMetric(ctx, serverID, "query_calls", float64(agg.Calls))
		_ = c.healthRepo.LogMetric(ctx, serverID, "total_exec_ms", agg.TotalExecMs)
	}

	return nil
}
