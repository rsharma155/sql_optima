// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Background collector for SQL Server database catalog (sys.databases snapshot).
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

	"github.com/rsharma155/sql_optima/internal/storage/hot"
)

// StartSqlServerDatabaseCatalogCollector runs a periodic snapshot of
// sys.databases for every SQL Server instance, storing results in the
// sqlserver_database_catalog hypertable.
func (s *MetricsService) StartSqlServerDatabaseCatalogCollector(ctx context.Context) {
	const defaultInterval = time.Hour
	interval := s.GetCollectorInterval(ctx, "SQL Server Database Catalog", defaultInterval)
	slog.Info("[SQLServerDBCatalog] Starting background collector (interval: %v)", "val", interval)

	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		s.collectSqlServerDatabaseCatalog(ctx)

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				newInterval := s.GetCollectorInterval(ctx, "SQL Server Database Catalog", defaultInterval)
				if newInterval != interval && newInterval > 0 {
					slog.Info("[SQLServerDBCatalog] Interval changed", "arg1", interval, "arg2", newInterval)
					interval = newInterval
					ticker.Reset(interval)
				}
				if interval > 0 {
					s.collectSqlServerDatabaseCatalog(ctx)
				}
			}
		}
	}()
}

func (s *MetricsService) collectSqlServerDatabaseCatalog(ctx context.Context) {
	now := time.Now().UTC()

	for _, inst := range s.Config.Instances {
		if strings.ToLower(inst.Type) != "sqlserver" {
			continue
		}

		catalogRows, err := s.MsRepo.FetchDatabaseCatalog(ctx, inst.Name)
		if err != nil {
			slog.Error("[SQLServerDBCatalog]", "target", inst.Name, "err", err)
			_ = s.tsLogger.LogCollectorError(ctx, inst.ServerID, "database catalog collection error: "+err.Error())
			continue
		}
		if len(catalogRows) == 0 {
			continue
		}

		var hotRows []hot.SQLServerDatabaseCatalogRow
		for _, r := range catalogRows {
			hotRows = append(hotRows, hot.SQLServerDatabaseCatalogRow{
				CaptureTimestamp:              now,
				ServerID:                      inst.ServerID,
				DatabaseID:                    r.DatabaseID,
				DatabaseName:                  r.Name,
				CreateDate:                    r.CreateDate,
				CompatibilityLevel:            r.CompatibilityLevel,
				CollationName:                 r.CollationName,
				StateDesc:                     r.StateDesc,
				UserAccessDesc:                r.UserAccessDesc,
				IsReadOnly:                    r.IsReadOnly,
				IsCleanlyShutdown:             r.IsCleanlyShutdown,
				RecoveryModelDesc:             r.RecoveryModelDesc,
				LogReuseWaitDesc:              r.LogReuseWaitDesc,
				DelayedDurabilityDesc:         r.DelayedDurabilityDesc,
				TargetRecoveryTimeInSeconds:   r.TargetRecoveryTimeInSeconds,
				IsAcceleratedDatabaseRecovery: r.IsAcceleratedDatabaseRecovery,
				IsAutoCloseOn:                 r.IsAutoCloseOn,
				IsAutoShrinkOn:                r.IsAutoShrinkOn,
				PageVerifyOptionDesc:          r.PageVerifyOptionDesc,
				IsReadCommittedSnapshotOn:     r.IsReadCommittedSnapshotOn,
				SnapshotIsolationStateDesc:    r.SnapshotIsolationStateDesc,
				IsEncrypted:                   r.IsEncrypted,
				IsCdcEnabled:                  r.IsCdcEnabled,
				IsBrokerEnabled:               r.IsBrokerEnabled,
				IsFulltextEnabled:             r.IsFulltextEnabled,
				IsMemoryOptimizedEnabled:      r.IsMemoryOptimizedEnabled,
				OwnerName:                     r.OwnerName,
				ContainmentDesc:               r.ContainmentDesc,
				IsTrustworthyOn:               r.IsTrustworthyOn,
				IsPublished:                   r.IsPublished,
				IsSubscribed:                  r.IsSubscribed,
				IsDistributor:                 r.IsDistributor,
				GroupDatabaseID:               r.GroupDatabaseID,
			})
		}

		if err := s.tsLogger.LogDatabaseCatalog(ctx, hotRows); err != nil {
			slog.Error("[SQLServerDBCatalog]", "target", inst.Name, "err", err)
		} else {
			slog.Info("[SQLServerDBCatalog]", "arg1", inst.Name, "arg2", len(hotRows))
		}
	}
}
