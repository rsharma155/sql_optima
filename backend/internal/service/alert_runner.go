// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Alert Evaluation background runner.
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

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/domain/alerts"
)

func runOnce(ctx context.Context, cfg *config.Config, alertSvc *AlertService) {
	if cfg == nil || alertSvc == nil {
		return
	}
	for _, inst := range cfg.Instances {
		engine, ok := engineForInstanceType(inst.Type)
		if !ok {
			continue
		}
		_, _ = alertSvc.RunEvaluation(ctx, inst.ServerID, engine)
	}
}

// StartAlertEvaluationLoop is a package-level entry point for the alert evaluation background loop.
func StartAlertEvaluationLoop(ctx context.Context, pool *pgxpool.Pool, cfg *config.Config, alertSvc *AlertService, interval time.Duration) {
	if pool == nil || cfg == nil || alertSvc == nil {
		return
	}
	ticker := time.NewTicker(interval)
	slog.Info("[AlertRunner] StartAlertEvaluationLoop: interval=", "val", interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, inst := range cfg.Instances {
				var engine alerts.Engine
				switch strings.ToLower(inst.Type) {
				case "postgres":
					engine = alerts.EnginePostgres
				case "sqlserver":
					engine = alerts.EngineSQLServer
				default:
					continue
				}
				serverID := inst.ServerID
				go func(sid uuid.UUID, eng alerts.Engine) {
					_, _ = alertSvc.RunEvaluation(ctx, sid, eng)
				}(serverID, engine)
			}
		}
	}
}

func (s *MetricsService) StartAlertEvaluation(ctx context.Context, alertSvc *AlertService) {
	interval := s.GetCollectorInterval(ctx, "Alert Evaluation Loop", 1*time.Minute)
	slog.Info("[AlertRunner] Starting evaluation loop (%v interval)", "val", interval)
	ticker := time.NewTicker(interval)

	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				newInterval := s.GetCollectorInterval(ctx, "Alert Evaluation Loop", 1*time.Minute)
				if newInterval > 0 && newInterval != interval {
					slog.Info("[AlertRunner] interval changed from", "arg1", interval, "arg2", newInterval)
					interval = newInterval
					ticker.Reset(interval)
				}
				s.RunPostgresAlertEvaluation(ctx, alertSvc)
				s.RunSQLServerAlertEvaluation(ctx, alertSvc)
			}
		}
	}()
}

func (s *MetricsService) RunPostgresAlertEvaluation(ctx context.Context, alertSvc *AlertService) {
	for _, inst := range s.Config.Instances {
		if strings.ToLower(inst.Type) != "postgres" {
			continue
		}

		go func(instanceName string, serverID uuid.UUID) {
			_, err := alertSvc.RunEvaluation(ctx, serverID, alerts.EnginePostgres)
			if err != nil {
				slog.Error("[AlertRunner] ERROR: PG evaluation failed", "target", instanceName, "err", err)
			}
		}(inst.Name, inst.ServerID)
	}
}

func (s *MetricsService) RunSQLServerAlertEvaluation(ctx context.Context, alertSvc *AlertService) {
	for _, inst := range s.Config.Instances {
		if strings.ToLower(inst.Type) != "sqlserver" {
			continue
		}

		go func(instanceName string, serverID uuid.UUID) {
			_, err := alertSvc.RunEvaluation(ctx, serverID, alerts.EngineSQLServer)
			if err != nil {
				slog.Error("[AlertRunner] ERROR: SQLServer evaluation failed", "target", instanceName, "err", err)
			}
		}(inst.Name, inst.ServerID)
	}
}

func engineForInstanceType(typ string) (alerts.Engine, bool) {
	switch strings.ToLower(typ) {
	case "postgres":
		return alerts.EnginePostgres, true
	case "sqlserver":
		return alerts.EngineSQLServer, true
	default:
		return "", false
	}
}
