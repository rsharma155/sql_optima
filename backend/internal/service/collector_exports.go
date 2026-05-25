// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Public entry points for triggering collection cycles manually or from background tickers.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package service

import (
	"log/slog"
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rsharma155/sql_optima/internal/config"
)

// TriggerBackgroundCollectorsOnce runs a single sweep of all background metrics (historical).
func (s *MetricsService) TriggerBackgroundCollectorsOnce() {
	s.runHistoricalStorageWithContext(context.Background())
}

// RunLiveCollectorOnce runs a single sweep of live diagnostics (active sessions, locks).
func (s *MetricsService) RunLiveCollectorOnce(ctx context.Context) {
	s.runLiveDiagnosticsWithContext(ctx)
}

// RunLiveCollectorForInstance runs live diagnostics for a specific instance.
func (s *MetricsService) RunLiveCollectorForInstance(ctx context.Context, serverID uuid.UUID) {
	s.runLiveDiagnosticsForInstance(ctx, serverID)
}

// Internal implementations shifted to prevent direct export and match expected naming in errors

func (s *MetricsService) runLiveDiagnosticsWithContext(ctx context.Context) {
	for _, inst := range s.Config.Instances {
		s.runLiveDiagnosticsForInstance(ctx, inst.ServerID)
	}
}

func (s *MetricsService) runLiveDiagnosticsForInstance(ctx context.Context, serverID uuid.UUID) {
}

func (s *MetricsService) runHistoricalStorageWithContext(ctx context.Context) {
	slog.Info("[HistoricalStorage] Starting sweep for %d instances", "val", len(s.Config.Instances))
	for _, inst := range s.Config.Instances {
		serverID := inst.ServerID
		if strings.ToLower(inst.Type) == "postgres" {
			s.EnqueueCollection(serverID, func() {
				s.CollectPgIncidents(serverID)
			})
		}
		// SQL Server health KPIs are collected by StartSqlServerHealthCollector (30s interval).
		// Running collectSqlServerHealthStats here would double-write disk_history and other tables.
	}
}

// RunHistoricalCollectorOnce runs a single sweep of historical storage collection.
func (s *MetricsService) RunHistoricalCollectorOnce(ctx context.Context) {
	s.runHistoricalStorageWithContext(ctx)
}

// StartBackgroundCollector runs the table-driven frequency background collector loop.
func (s *MetricsService) StartBackgroundCollector(ctx context.Context) {
	// Historical and Tier 4 metrics usually run every 5 minutes
	interval := s.GetCollectorInterval(ctx, "Base Collector Ticker", 5*time.Minute)
	slog.Info("[BackgroundCollector] Starting loop (interval: %v)", "val", interval)
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		// Initial run
		s.TriggerBackgroundCollectorsOnce()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Dynamic interval check
				newInterval := s.GetCollectorInterval(ctx, "Base Collector Ticker", 5*time.Minute)
				if newInterval != interval && newInterval > 0 {
					slog.Info("[BackgroundCollector] Frequency changed from", "arg1", interval, "arg2", newInterval)
					interval = newInterval
					ticker.Reset(interval)
				}

				if interval > 0 {
					s.TriggerBackgroundCollectorsOnce()
				}
			}
		}
	}()
}

// StartPostgresNewDashboardsCollectors runs collectors for new Postgres dashboard endpoints.
func (s *MetricsService) StartPostgresNewDashboardsCollectors(ctx context.Context) {
	interval := s.GetCollectorInterval(ctx, "Postgres Dashboard Snapshot", 1*time.Minute)
	slog.Info("[PostgresDashboards] Starting background collector (interval: %v)", "val", interval)
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Dynamic interval check
				newInterval := s.GetCollectorInterval(ctx, "Postgres Dashboard Snapshot", 1*time.Minute)
				if newInterval != interval && newInterval > 0 {
					slog.Info("[PostgresDashboards] Frequency changed from", "arg1", interval, "arg2", newInterval)
					interval = newInterval
					ticker.Reset(interval)
				}

				if interval > 0 {
					// Memory intelligence iterates instances internally
					s.CollectPostgresMemoryIntelligence(ctx)

					// Call PG snapshot collection for each instance
					for _, inst := range s.Config.Instances {
						if strings.ToLower(inst.Type) == "postgres" {
							s.PgRepo.UpdateInstanceStatus(inst.Name)
							serverID := inst.ServerID
							s.EnqueueCollection(serverID, func() {
								db, ok := s.PgRepo.GetConn(inst.Name)
								if !ok || s.PgRepo.GetInstanceStatus(inst.Name) != "online" {
									return
								}

								if s.PgSnapshotCollector != nil {
									_ = s.PgSnapshotCollector.Collect(ctx, inst)
								}
								if s.PgComprehensiveCollector != nil {
									s.PgComprehensiveCollector.Collect(ctx, inst)
								}
								if s.PgTimescaleColl != nil {
									_ = s.PgTimescaleColl.Collect(ctx, inst, db)
								}
								if s.PgObsCollector != nil {
									_ = s.PgObsCollector.CollectQueryWaitProfile(ctx, serverID, db)
									_ = s.PgObsCollector.CollectSessionActivity(ctx, serverID, db)
								}
								if s.PgSecurityCollector != nil {
									_ = s.PgSecurityCollector.CollectRoleSnapshot(ctx, serverID, db)
									_ = s.PgSecurityCollector.CollectDDLActivity(ctx, serverID, db)
								}
								if s.PgBackupRepo != nil {
									_ = s.PgBackupRepo.CollectBackupDR(ctx, serverID)
								}
							})
						}
					}
				}
			}
		}
	}()
}

// StartPostgresEnterpriseCollector runs the Postgres enterprise metrics collector loop.
func (s *MetricsService) StartPostgresEnterpriseCollector(ctx context.Context) {
	interval := s.GetCollectorInterval(ctx, "Postgres Enterprise Metrics", 5*time.Minute)
	ticker := time.NewTicker(interval)
	slog.Info("[PostgresEnterprise] Starting collector (%v interval)", "val", interval)

	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				for _, inst := range s.Config.Instances {
					if strings.ToLower(inst.Type) == "postgres" {
						serverID := inst.ServerID
						s.EnqueueCollection(serverID, func() {
							db, ok := s.PgRepo.GetConn(inst.Name)
							if !ok {
								return
							}
							// Config Drift / Settings Snapshot
							// Logs summary
							_ = s.collectPostgresLogs(ctx, inst, db)
						})
					}
				}
			}
		}
	}()
}

func (s *MetricsService) collectPostgresLogs(ctx context.Context, inst config.Instance, db *sql.DB) error {
	// Simple log collection from pg_log if available (requires extension or custom view)
	// For now, we'll just log that it ran
	return nil
}

// GetCollectorInterval looks up the configured interval for a named collector.
// If the entry is missing, it is automatically inserted with defaultVal as the frequency
// so it becomes visible and editable in the Admin panel. Returns defaultVal when
// TimescaleDB is unavailable or the insert fails.
func (s *MetricsService) GetCollectorInterval(ctx context.Context, name string, defaultVal time.Duration) time.Duration {
	if s.ConfigRepo == nil {
		return defaultVal
	}
	cfg, err := s.ConfigRepo.GetByName(ctx, name)
	if err != nil {
		// Entry not found — insert it so the admin panel can manage it.
		defaultSec := int(defaultVal.Seconds())
		if defaultSec <= 0 {
			defaultSec = 60
		}
		_ = s.ConfigRepo.InsertDefault(ctx, name, inferModule(name), defaultSec)
		return defaultVal
	}
	if !cfg.IsActive {
		return 0
	}
	return time.Duration(cfg.FrequencySeconds) * time.Second
}

// inferModule maps a collector name to its module category based on naming convention.
func inferModule(name string) string {
	lower := strings.ToLower(name)
	switch {
	case strings.HasPrefix(lower, "sqlserver_") || strings.HasPrefix(lower, "sql server"):
		return "SQLSERVER"
	case strings.HasPrefix(lower, "pg_") || strings.HasPrefix(lower, "postgres"):
		return "Postgres"
	default:
		return "System"
	}
}
