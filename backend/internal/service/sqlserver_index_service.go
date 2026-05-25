// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Background service that collects SQL Server index fragmentation
// and missing index recommendations every 6 hours, writing results to
// sqlserver_index_fragmentation and sqlserver_missing_indexes hypertables.
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
	sqlserver "github.com/rsharma155/sql_optima/internal/collectors/infrastructure/sqlserver"
	"github.com/rsharma155/sql_optima/internal/storage/hot"
)

const indexCollectorInterval = 6 * time.Hour

// StartIndexCollector launches a background goroutine that collects index
// fragmentation and missing index data every 6 hours per SQL Server instance.
func (s *MetricsService) StartIndexCollector(ctx context.Context) {
	interval := indexCollectorInterval
	slog.Info("[IndexCollector] Starting", "interval", interval)

	fragCollector := &sqlserver.IndexFragmentationCollector{}
	missingCollector := &sqlserver.MissingIndexCollector{}

	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		// Run once immediately on startup, then on each tick.
		s.runIndexCollection(ctx, fragCollector, missingCollector)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.runIndexCollection(ctx, fragCollector, missingCollector)
			}
		}
	}()
}

func (s *MetricsService) runIndexCollection(
	ctx context.Context,
	fragCollector *sqlserver.IndexFragmentationCollector,
	missingCollector *sqlserver.MissingIndexCollector,
) {
	for _, inst := range s.Config.Instances {
		if strings.ToLower(inst.Type) != "sqlserver" {
			continue
		}
		instance := inst
		s.EnqueueCollection(instance.ServerID, func() {
			s.collectIndexesForInstance(ctx, fragCollector, missingCollector, instance.ServerID, instance.Name)
		})
	}
}

func (s *MetricsService) collectIndexesForInstance(
	ctx context.Context,
	fragCollector *sqlserver.IndexFragmentationCollector,
	missingCollector *sqlserver.MissingIndexCollector,
	serverID uuid.UUID,
	instanceName string,
) {
	if s.MsRepo == nil || s.tsLogger == nil {
		return
	}

	// Collect missing indexes (instance-level, no per-db loop needed).
	s.collectMissingIndexes(ctx, missingCollector, serverID, instanceName)

	// Collect fragmentation per user database.
	dbs, err := s.MsRepo.ListSQLServerUserDatabases(ctx, instanceName)
	if err != nil {
		slog.Error("[IndexCollector] list databases failed", "instance", instanceName, "err", err)
		return
	}
	for _, dbName := range dbs {
		s.collectFragmentationForDB(ctx, fragCollector, serverID, instanceName, dbName)
	}
}

func (s *MetricsService) collectFragmentationForDB(
	ctx context.Context,
	collector *sqlserver.IndexFragmentationCollector,
	serverID uuid.UUID,
	instanceName string,
	databaseName string,
) {
	rawDB, ok := s.MsRepo.GetConn(instanceName)
	if !ok || rawDB == nil {
		return
	}

	rows, err := collector.Fetch(ctx, rawDB, databaseName)
	if err != nil {
		slog.Error("[IndexCollector] fragmentation fetch failed",
			"instance", instanceName, "db", databaseName, "err", err)
		return
	}
	if len(rows) == 0 {
		return
	}

	now := time.Now().UTC()
	writeRows := make([]hot.IndexFragmentationWriteRow, 0, len(rows))
	for _, r := range rows {
		writeRows = append(writeRows, hot.IndexFragmentationWriteRow{
			DatabaseName:         r.DatabaseName,
			SchemaName:           r.SchemaName,
			TableName:            r.TableName,
			IndexName:            r.IndexName,
			IndexID:              r.IndexID,
			IndexTypeDesc:        r.IndexTypeDesc,
			AvgFragmentationPct:  r.AvgFragmentationPct,
			PageCount:            r.PageCount,
			AvgPageSpaceUsedPct:  r.AvgPageSpaceUsedPct,
			RecordCount:          r.RecordCount,
			FragmentCount:        r.FragmentCount,
			AvgFragmentSizePages: r.AvgFragmentSizePages,
		})
	}

	if err := s.tsLogger.LogSqlServerIndexFragmentation(ctx, serverID, now, writeRows); err != nil {
		slog.Error("[IndexCollector] fragmentation write failed",
			"instance", instanceName, "db", databaseName, "err", err)
	}
}

func (s *MetricsService) collectMissingIndexes(
	ctx context.Context,
	collector *sqlserver.MissingIndexCollector,
	serverID uuid.UUID,
	instanceName string,
) {
	rawDB, ok := s.MsRepo.GetConn(instanceName)
	if !ok || rawDB == nil {
		return
	}

	rows, err := collector.Fetch(ctx, rawDB, 100)
	if err != nil {
		slog.Error("[IndexCollector] missing indexes fetch failed",
			"instance", instanceName, "err", err)
		return
	}
	if len(rows) == 0 {
		return
	}

	now := time.Now().UTC()
	writeRows := make([]hot.MissingIndexWriteRow, 0, len(rows))
	for _, r := range rows {
		var lastSeek, lastScan *time.Time
		if r.LastUserSeek != nil {
			if t, err := time.Parse("2006-01-02T15:04:05", *r.LastUserSeek); err == nil {
				lastSeek = &t
			}
		}
		if r.LastUserScan != nil {
			if t, err := time.Parse("2006-01-02T15:04:05", *r.LastUserScan); err == nil {
				lastScan = &t
			}
		}
		writeRows = append(writeRows, hot.MissingIndexWriteRow{
			DatabaseName:      r.DatabaseName,
			SchemaName:        r.SchemaName,
			TableName:         r.TableName,
			EqualityColumns:   r.EqualityColumns,
			InequalityColumns: r.InequalityColumns,
			IncludedColumns:   r.IncludedColumns,
			UserSeeks:         r.UserSeeks,
			UserScans:         r.UserScans,
			AvgTotalUserCost:  r.AvgTotalUserCost,
			AvgUserImpact:     r.AvgUserImpact,
			ImprovementScore:  r.ImprovementScore,
			LastUserSeek:      lastSeek,
			LastUserScan:      lastScan,
		})
	}

	if err := s.tsLogger.LogSqlServerMissingIndexes(ctx, serverID, now, writeRows); err != nil {
		slog.Error("[IndexCollector] missing indexes write failed",
			"instance", instanceName, "err", err)
	}
}
