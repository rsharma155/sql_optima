// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Background worker for the enhanced PostgreSQL dashboard metrics (v2).
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package service

import (
	"log/slog"
	"context"
	"strings"
	"time"

	"github.com/rsharma155/sql_optima/internal/collectors"
	"github.com/rsharma155/sql_optima/internal/collectors/postgres"
	"github.com/rsharma155/sql_optima/internal/config"
)

type PostgresNewDashboardsWorker struct {
	cfg               *config.Config
	snapshotCollector *collectors.PgSnapshotCollector
	queryRouter       *postgres.QueryMetricsRouter
	metricsSvc        *MetricsService
	stopChan          chan struct{}
}

func NewPostgresNewDashboardsWorker(cfg *config.Config, snap *collectors.PgSnapshotCollector, router *postgres.QueryMetricsRouter, metricsSvc *MetricsService) *PostgresNewDashboardsWorker {
	return &PostgresNewDashboardsWorker{
		cfg:               cfg,
		snapshotCollector: snap,
		queryRouter:       router,
		metricsSvc:        metricsSvc,
		stopChan:          make(chan struct{}),
	}
}

func (w *PostgresNewDashboardsWorker) Start(ctx context.Context) {
	var interval time.Duration
	if w.metricsSvc != nil {
		interval = w.metricsSvc.GetCollectorInterval(ctx, "pg_new_dashboards_worker", 30*time.Second)
	} else {
		interval = 30 * time.Second
	}
	slog.Info("[PostgresWorker] Enhanced PG Dashboards worker started (%v interval)", "val", interval)
	ticker := time.NewTicker(interval)

	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-w.stopChan:
				return
			case <-ticker.C:
				if w.metricsSvc != nil {
					newInterval := w.metricsSvc.GetCollectorInterval(ctx, "pg_new_dashboards_worker", 30*time.Second)
					if newInterval > 0 && newInterval != interval {
						interval = newInterval
						ticker.Reset(interval)
					}
				}
				w.runIteration(ctx)
			}
		}
	}()
}

func (w *PostgresNewDashboardsWorker) Stop() {
	close(w.stopChan)
}

func (w *PostgresNewDashboardsWorker) runIteration(ctx context.Context) {
	for _, inst := range w.cfg.Instances {
		if strings.ToLower(inst.Type) != "postgres" {
			continue
		}

		go func(instance config.Instance) {
			// 1. Snapshot Collection
			if err := w.snapshotCollector.Collect(ctx, instance); err != nil {
				slog.Error("[PostgresWorker] ERROR: Snapshot collection failed", "target", instance.Name, "err", err)
			}

			// 2. Query Metrics (Router handles extension detection)
			db, err := config.ConnectToInstance(instance)
			if err != nil {
				slog.Error("[PostgresWorker] ERROR: Connection failed", "target", instance.Name, "err", err)
				return
			}
			defer db.Close()

			if err := w.queryRouter.Collect(ctx, instance, db); err != nil {
				slog.Error("[PostgresWorker] ERROR: Query collection failed", "target", instance.Name, "err", err)
			}
		}(inst)
	}
}
