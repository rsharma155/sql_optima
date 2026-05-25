// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Background worker for SQL Server backup posture and history collection.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package service

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	backupdomain "github.com/rsharma155/sql_optima/internal/domain/sqlserver_backup_recovery/domain"
	"github.com/rsharma155/sql_optima/internal/domain/sqlserver_backup_recovery/domain/repositories"
	"github.com/rsharma155/sql_optima/internal/repository"
	"github.com/rsharma155/sql_optima/internal/storage/hot"
)

// StartSqlServerBackupCollector runs backup posture and history collection loops.
func (s *MetricsService) StartSqlServerBackupCollector(ctx context.Context) {
	if s.tsLogger == nil || s.MsRepo == nil {
		slog.Info("[Backup-Collector] disabled — TimescaleDB or SQL Server repo unavailable")
		return
	}
	slog.Info("[Backup-Collector] starting background worker")

	interval := s.GetCollectorInterval(ctx, "sqlserver_backup_posture", 300*time.Second)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	lastPosture := make(map[uuid.UUID]time.Time)
	lastHistory := make(map[uuid.UUID]time.Time)
	policyRepo := repositories.NewSQLServerBackupPolicyRepository(s.GetTimescaleDBPool())

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			newInterval := s.GetCollectorInterval(ctx, "sqlserver_backup_posture", 300*time.Second)
			if newInterval > 0 && newInterval != interval {
				interval = newInterval
				ticker.Reset(interval)
			}
			postureFreq := s.GetCollectorInterval(ctx, "sqlserver_backup_posture", 300*time.Second)
			historyFreq := s.GetCollectorInterval(ctx, "sqlserver_backup_history", 900*time.Second)

			for _, inst := range s.Config.Instances {
				if strings.ToLower(inst.Type) != "sqlserver" {
					continue
				}
				if s.MsRepo.GetInstanceStatus(inst.Name) != "online" {
					continue
				}
				serverID := inst.ServerID
				instanceName := inst.Name

				if time.Since(lastPosture[serverID]) >= postureFreq {
					lastPosture[serverID] = time.Now()
					collectBackupPosture(ctx, s.MsRepo, s.GetTimescaleDBLogger(), policyRepo, serverID, instanceName)
				}
				if time.Since(lastHistory[serverID]) >= historyFreq {
					lastHistory[serverID] = time.Now()
					collectBackupHistory(ctx, s.MsRepo, s.GetTimescaleDBLogger(), serverID, instanceName)
				}
			}
		}
	}
}

func collectBackupPosture(ctx context.Context, msRepo *repository.SqlServerRepository, logger *hot.TimescaleLogger, policyRepo *repositories.SQLServerBackupPolicyRepository, serverID uuid.UUID, instanceName string) {
	if logger == nil {
		return
	}
	posture, compression, err := msRepo.FetchBackupPostureLive(ctx, instanceName)
	if err != nil {
		slog.Error("[Backup-Collector] posture fetch failed", "target", instanceName, "err", err)
		return
	}
	policy, _ := policyRepo.Get(ctx, serverID)
	backupdomain.ApplyPolicyFreshness(posture, policy)
	now := time.Now().UTC()
	rows := repository.MapPostureToHotRows(serverID, now, posture, compression)
	for i := range rows {
		rows[i].FullFreshOK = posture[i].FullFreshOK
		rows[i].LogFreshOK = posture[i].LogFreshOK
	}
	if err := logger.LogSQLServerBackupPosture(ctx, rows); err != nil {
		slog.Error("[Backup-Collector] posture persist failed", "target", instanceName, "err", err)
	}
}

func collectBackupHistory(ctx context.Context, msRepo *repository.SqlServerRepository, logger *hot.TimescaleLogger, serverID uuid.UUID, instanceName string) {
	if logger == nil {
		return
	}
	history, err := msRepo.FetchBackupHistoryLive(ctx, instanceName, 500)
	if err != nil {
		slog.Error("[Backup-Collector] history fetch failed", "target", instanceName, "err", err)
		return
	}
	now := time.Now().UTC()
	rows := repository.MapHistoryToHotRows(serverID, now, history)
	if err := logger.LogSQLServerBackupHistory(ctx, rows); err != nil {
		slog.Error("[Backup-Collector] history persist failed", "target", instanceName, "err", err)
	}
}
