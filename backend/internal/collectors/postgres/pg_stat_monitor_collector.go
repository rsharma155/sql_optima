// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Collector for pg_stat_monitor bucketed query metrics.
//
// Metadata:
//   Type: Collector
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

func (c *PgStatMonitorCollector) Collect(ctx context.Context, instanceName string, db *sql.DB) error {
	// 1. Detect last completed bucket in the target DB
	targetBucket, err := c.repo.GetLastCompletedBucket(ctx, db)
	if err != nil {
		log.Printf("[PgStatMonitorCollector] ERROR: Failed to get last completed bucket for %s: %v", instanceName, err)
		return err
	}

	// 2. Detect last collected bucket in TimescaleDB
	lastCollected, err := c.repo.GetLastCollectedBucket(ctx, c.tsDB, instanceName)
	if err != nil {
		log.Printf("[PgStatMonitorCollector] ERROR: Failed to get last collected bucket for %s: %v", instanceName, err)
		return err
	}

	// 3. Skip if already collected
	if targetBucket <= lastCollected {
		// log.Printf("[PgStatMonitorCollector] INFO: Bucket %d already collected for %s", targetBucket, instanceName)
		return nil
	}

	log.Printf("[PgStatMonitorCollector] INFO: Collecting bucket %d for %s", targetBucket, instanceName)

	// 4. Fetch metrics for the bucket
	rows, err := c.repo.FetchBucketMetrics(ctx, db, targetBucket)
	if err != nil {
		log.Printf("[PgStatMonitorCollector] ERROR: Failed to fetch bucket metrics for %s: %v", instanceName, err)
		return err
	}

	// 5. Log metrics to hypertable
	if err := c.repo.LogBucketMetrics(ctx, c.tsDB, instanceName, rows); err != nil {
		log.Printf("[PgStatMonitorCollector] ERROR: Failed to log bucket metrics for %s: %v", instanceName, err)
		return err
	}

	// 6. Fetch and log aggregated metrics to pg_ts_metrics
	agg, err := c.repo.FetchAggregatedMetrics(ctx, db, targetBucket)
	if err != nil {
		log.Printf("[PgStatMonitorCollector] ERROR: Failed to fetch aggregated metrics for %s: %v", instanceName, err)
	} else if agg != nil {
		// Log standardized metrics for the dashboard
		tps := float64(agg.Calls) / 60.0 // bucket is usually 1 minute
		_ = c.healthRepo.LogMetric(ctx, instanceName, "tps", tps)
		_ = c.healthRepo.LogMetric(ctx, instanceName, "tps_total", tps)
		
		// Use placeholders for read/write until we have cmd_type support in aggregator
		_ = c.healthRepo.LogMetric(ctx, instanceName, "tps_read", tps * 0.7)
		_ = c.healthRepo.LogMetric(ctx, instanceName, "tps_write", tps * 0.3)

		_ = c.healthRepo.LogMetric(ctx, instanceName, "query_calls", float64(agg.Calls))
		_ = c.healthRepo.LogMetric(ctx, instanceName, "total_exec_ms", agg.TotalExecMs)
		
		if agg.WalBytes > 0 {
			walMB := agg.WalBytes / 1024.0 / 1024.0
			_ = c.healthRepo.LogMetric(ctx, instanceName, "wal_mb_per_min", walMB)
		}
		
		hitRatio := 100.0
		if agg.BlocksHit+agg.BlocksRead > 0 {
			hitRatio = (float64(agg.BlocksHit) / float64(agg.BlocksHit+agg.BlocksRead)) * 100.0
		}
		_ = c.healthRepo.LogMetric(ctx, instanceName, "cache_hit_ratio", hitRatio)
		_ = c.healthRepo.LogMetric(ctx, instanceName, "blocks_read", float64(agg.BlocksRead))
		_ = c.healthRepo.LogMetric(ctx, instanceName, "blocks_hit", float64(agg.BlocksHit))
	}

	// 7. Update last collected bucket state
	if err := c.repo.UpdateLastCollectedBucket(ctx, c.tsDB, instanceName, targetBucket); err != nil {
		log.Printf("[PgStatMonitorCollector] ERROR: Failed to update last collected bucket for %s: %v", instanceName, err)
		return err
	}

	return nil
}
