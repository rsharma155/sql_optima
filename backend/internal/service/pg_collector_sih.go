// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Background collector for PostgreSQL Storage & Index Health (SIH) snapshots.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rsharma155/sql_optima/internal/collectors"
	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/storage/hot"
)

func (s *MetricsService) StartPostgresStorageIndexHealthCollector(ctx context.Context) {
	slog.Info("[PostgresSIH] Starting background collector")

	// Run once immediately
	s.CollectPostgresStorageIndexHealth(ctx)

	interval := s.GetCollectorInterval(ctx, "pg_storage_index_health", 5*time.Minute)
	slog.Info("[PostgresSIH] check interval", "val", interval)
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				newInterval := s.GetCollectorInterval(ctx, "pg_storage_index_health", 5*time.Minute)
				if newInterval > 0 && newInterval != interval {
					slog.Info("[PostgresSIH] interval changed from", "arg1", interval, "arg2", newInterval)
					interval = newInterval
					ticker.Reset(interval)
				}
				s.CollectPostgresStorageIndexHealth(ctx)
			}
		}
	}()
}

func (s *MetricsService) CollectPostgresStorageIndexHealth(ctx context.Context) {
	if s.tsLogger == nil || s.PgRepo == nil || s.Config == nil {
		return
	}
	for _, inst := range s.Config.Instances {
		if strings.ToLower(inst.Type) != "postgres" {
			continue
		}

		serverID := inst.ServerID

		// Every 15 minutes (configurable): Table/Index usage
		if s.sihDue(serverID, "pg_storage_index_health_index15m", s.GetCollectorInterval(ctx, "pg_storage_index_health_index15m", 15*time.Minute)) {
			go s.collectPgTableIndexUsage(ctx, inst)
		}

		// Every 6 hours (configurable): Database/Table growth estimation
		if s.sihDue(serverID, "pg_storage_index_health_growth6h", s.GetCollectorInterval(ctx, "pg_storage_index_health_growth6h", 6*time.Hour)) {
			go s.collectPgGrowth(ctx, inst)
		}

		// Daily (configurable): Full index definitions (heavy)
		if s.sihDue(serverID, "pg_storage_index_health_defs_daily", s.GetCollectorInterval(ctx, "pg_storage_index_health_defs_daily", 24*time.Hour)) {
			go s.collectPgIndexDefinitions(ctx, inst)
		}
	}
}

func (s *MetricsService) sihDue(serverID uuid.UUID, kind string, interval time.Duration) bool {
	if interval <= 0 {
		return false
	}
	s.sihMu.Lock()
	defer s.sihMu.Unlock()

	var last time.Time
	var ok bool

	switch kind {
	case "index15m", "pg_storage_index_health_index15m", "sqlserver_storage_index_health_index15m":
		last, ok = s.sihLastIndex15m[serverID]
	case "table15m", "pg_storage_index_health_table15m", "sqlserver_storage_index_health_table15m":
		last, ok = s.sihLastTable15m[serverID]
	case "growth6h", "pg_storage_index_health_growth6h", "sqlserver_storage_index_health_growth6h":
		last, ok = s.sihLastGrowth6h[serverID]
	case "defsDaily", "pg_storage_index_health_defs_daily":
		last, ok = s.sihLastDefsDaily[serverID]
	}

	if !ok || time.Since(last) >= interval {
		switch kind {
		case "index15m", "pg_storage_index_health_index15m", "sqlserver_storage_index_health_index15m":
			s.sihLastIndex15m[serverID] = time.Now()
		case "table15m", "pg_storage_index_health_table15m", "sqlserver_storage_index_health_table15m":
			s.sihLastTable15m[serverID] = time.Now()
		case "growth6h", "pg_storage_index_health_growth6h", "sqlserver_storage_index_health_growth6h":
			s.sihLastGrowth6h[serverID] = time.Now()
		case "defsDaily", "pg_storage_index_health_defs_daily":
			s.sihLastDefsDaily[serverID] = time.Now()
		}
		return true
	}
	return false
}

func (s *MetricsService) collectPgTableIndexUsage(ctx context.Context, inst config.Instance) {
	dbs, err := s.PgRepo.GetDatabases(ctx, inst.Name)
	if err != nil || len(dbs) == 0 {
		slog.Error("[PostgresSIH] collectPgTableIndexUsage: cannot list databases", "target", inst.Name, "err", err)
		return
	}

	now := time.Now().UTC()
	for _, dbName := range dbs {
		db, err := s.PgRepo.GetConnForDB(inst.Name, dbName)
		if err != nil {
			slog.Error(fmt.Sprintf("[PostgresSIH] collectPgTableIndexUsage: cannot connect to %s/%s: %v", inst.Name, dbName, err))
			continue
		}

		tables, err := collectors.CollectPostgresTableUsageAndSize(ctx, db)
		if err != nil {
			slog.Error("[PostgresSIH] table usage collect failed", "target", inst.Name, "db", dbName, "err", err)
		} else if len(tables) > 0 {
			if n, pErr := collectors.PersistPostgresTableUsageDeltas(ctx, s.tsLogger, inst.ServerID, tables, now); pErr != nil {
				slog.Error("[PostgresSIH] table usage persist failed", "target", inst.Name, "db", dbName, "err", pErr)
			} else if n > 0 {
				slog.Debug("[PostgresSIH] persisted table usage", "db", dbName, "rows", n)
			}
			if _, pErr := collectors.PersistPostgresTableSizeHistory(ctx, s.tsLogger, inst.ServerID, tables, now); pErr != nil {
				slog.Error("[PostgresSIH] table size history persist failed", "target", inst.Name, "db", dbName, "err", pErr)
			}
		}

		indexes, err := collectors.CollectPostgresIndexUsage(ctx, db)
		if err != nil {
			slog.Error("[PostgresSIH] index usage collect failed", "target", inst.Name, "db", dbName, "err", err)
		} else if len(indexes) > 0 {
			if n, pErr := collectors.PersistPostgresIndexUsageDeltas(ctx, s.tsLogger, inst.ServerID, indexes, now); pErr != nil {
				slog.Error("[PostgresSIH] index usage persist failed", "target", inst.Name, "db", dbName, "err", pErr)
			} else if n > 0 {
				slog.Debug("[PostgresSIH] persisted index usage", "db", dbName, "rows", n)
			}
		}
	}
}

func (s *MetricsService) collectPgGrowth(ctx context.Context, inst config.Instance) {
	dbs, err := s.PgRepo.GetDatabases(ctx, inst.Name)
	if err != nil || len(dbs) == 0 {
		slog.Error("[PostgresSIH] collectPgGrowth: cannot list databases", "target", inst.Name, "err", err)
		return
	}

	now := time.Now().UTC()
	for _, dbName := range dbs {
		db, err := s.PgRepo.GetConnForDB(inst.Name, dbName)
		if err != nil {
			slog.Error(fmt.Sprintf("[PostgresSIH] collectPgGrowth: cannot connect to %s/%s: %v", inst.Name, dbName, err))
			continue
		}

		tables, err := collectors.CollectPostgresTableUsageAndSize(ctx, db)
		if err != nil {
			continue
		}
		if _, pErr := collectors.PersistPostgresTableSizeHistory(ctx, s.tsLogger, inst.ServerID, tables, now); pErr != nil {
			slog.Error("[PostgresSIH] collectPgGrowth persist failed", "target", inst.Name, "db", dbName, "err", pErr)
		}
		_ = s.tsLogger.LogTableSizeHistoryWithChangeDetection(ctx, inst.ServerID, pgTableUsageToHistoryRows(inst.ServerID, dbName, tables, now))
	}
}

func (s *MetricsService) collectPgIndexDefinitions(ctx context.Context, inst config.Instance) {
	if s.PgIndexDefCollector != nil {
		if err := s.PgIndexDefCollector.Collect(ctx, inst); err != nil {
			slog.Error("[SIH] ERROR Index definitions", "target", inst.Name, "err", err)
		}
	}
}

func pgTableUsageToHistoryRows(serverID uuid.UUID, dbName string, tables []collectors.PgTableUsageRow, now time.Time) []hot.TableSizeHistoryRow {
	out := make([]hot.TableSizeHistoryRow, 0, len(tables))
	for _, t := range tables {
		out = append(out, hot.TableSizeHistoryRow{
			CaptureTimestamp: now,
			ServerID:         serverID,
			DatabaseName:     dbName,
			SchemaName:       t.SchemaName,
			TableName:        t.TableName,
			RowCount:         t.RowCount,
			TotalMB:          t.TableSizeMB,
		})
	}
	return out
}
