// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Comprehensive PostgreSQL metrics collector for dashboard-backing tables.
//          Covers: vacuum progress, table maintenance, replication slots, replication
//          lag detail, settings snapshot, and deadlock deltas by database.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package collectors

import (
	"log/slog"
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/repository"
	"github.com/rsharma155/sql_optima/internal/storage/hot"
)

// PgComprehensiveCollector gathers the dashboard-backing metrics not covered by
// PgSnapshotCollector: vacuum, table maintenance, replication slots, replication
// lag detail, settings snapshots, and per-database deadlock deltas.
type PgComprehensiveCollector struct {
	pgRepo   *repository.PgRepository
	tsLogger *hot.TimescaleLogger
}

func NewPgComprehensiveCollector(pgRepo *repository.PgRepository, tsLogger *hot.TimescaleLogger) *PgComprehensiveCollector {
	return &PgComprehensiveCollector{pgRepo: pgRepo, tsLogger: tsLogger}
}

func (c *PgComprehensiveCollector) Collect(ctx context.Context, inst config.Instance) {
	name := inst.Name
	serverID := inst.ServerID

	c.collectVacuumProgress(ctx, name, serverID)
	c.collectTableMaintenance(ctx, name, serverID)
	c.collectReplicationSlots(ctx, name, serverID)
	c.collectReplicationLagDetail(ctx, name, serverID)
	c.collectSettingsSnapshot(ctx, name, serverID)
	c.collectDeadlockDeltas(ctx, name, serverID)
}

func (c *PgComprehensiveCollector) collectVacuumProgress(ctx context.Context, name string, serverID uuid.UUID) {
	rows, err := c.pgRepo.GetVacuumProgress(ctx, name)
	if err != nil {
		slog.Error("[PgComprehensive] GetVacuumProgress error", "target", name, "err", err)
		return
	}
	var hotRows []hot.PostgresVacuumProgressRow
	for _, r := range rows {
		hotRows = append(hotRows, hot.PostgresVacuumProgressRow{
			CaptureTimestamp: time.Now().UTC(),
			ServerID:         serverID,
			PID:              r.PID,
			DatabaseName:     r.DatabaseName,
			UserName:         r.UserName,
			RelationName:     r.RelationName,
			Phase:            r.Phase,
			HeapBlksTotal:    r.HeapBlksTotal,
			HeapBlksScanned:  r.HeapBlksScanned,
			HeapBlksVacuumed: r.HeapBlksVacuumed,
			IndexVacuumCount: r.IndexVacuumCount,
			MaxDeadTuples:    r.MaxDeadTuples,
			NumDeadTuples:    r.NumDeadTuples,
		})
	}
	if err := c.tsLogger.LogPostgresVacuumProgress(ctx, serverID, hotRows); err != nil {
		slog.Error("[PgComprehensive] LogPostgresVacuumProgress error", "target", name, "err", err)
	}
}

func (c *PgComprehensiveCollector) collectTableMaintenance(ctx context.Context, name string, serverID uuid.UUID) {
	rows, err := c.pgRepo.GetTableMaintenanceStats(ctx, name, 200)
	if err != nil {
		slog.Error("[PgComprehensive] GetTableMaintenanceStats error", "target", name, "err", err)
		return
	}
	var hotRows []hot.PostgresTableMaintRow
	for _, r := range rows {
		hotRows = append(hotRows, hot.PostgresTableMaintRow{
			DatabaseName:    name,
			SchemaName:      r.SchemaName,
			TableName:       r.TableName,
			TotalBytes:      r.TotalBytes,
			LiveTuples:      r.LiveTuples,
			DeadTuples:      r.DeadTuples,
			DeadPct:         r.DeadPct,
			SeqScans:        r.SeqScans,
			IdxScans:        r.IdxScans,
			LastVacuum:      r.LastVacuum,
			LastAutovacuum:  r.LastAutovacuum,
			LastAnalyze:     r.LastAnalyze,
			LastAutoanalyze: r.LastAutoanalyze,
		})
	}
	if err := c.tsLogger.LogPostgresTableMaintStats(ctx, serverID, hotRows); err != nil {
		slog.Error("[PgComprehensive] LogPostgresTableMaintStats error", "target", name, "err", err)
	}
}

func (c *PgComprehensiveCollector) collectReplicationSlots(ctx context.Context, name string, serverID uuid.UUID) {
	slots, err := c.pgRepo.GetReplicationSlotStats(ctx, name)
	if err != nil {
		slog.Error("[PgComprehensive] GetReplicationSlotStats error", "target", name, "err", err)
		return
	}
	var hotRows []hot.PostgresReplicationSlotRow
	now := time.Now().UTC()
	for _, s := range slots {
		hotRows = append(hotRows, hot.PostgresReplicationSlotRow{
			CaptureTimestamp:  now,
			ServerID:          serverID,
			SlotName:          s.SlotName,
			SlotType:          s.SlotType,
			Active:            s.Active,
			Temporary:         s.Temporary,
			RetainedWalMB:     s.RetainedWalMB,
			RestartLSN:        s.RestartLSN,
			ConfirmedFlushLSN: s.ConfirmedFlushLSN,
			Xmin:              s.Xmin,
			CatalogXmin:       s.CatalogXmin,
		})
	}
	if err := c.tsLogger.LogPostgresReplicationSlots(ctx, serverID, hotRows); err != nil {
		slog.Error("[PgComprehensive] LogPostgresReplicationSlots error", "target", name, "err", err)
	}
}

func (c *PgComprehensiveCollector) collectReplicationLagDetail(ctx context.Context, name string, serverID uuid.UUID) {
	stats, err := c.pgRepo.GetReplicationStats(ctx, name)
	if err != nil || stats == nil {
		return
	}
	var hotRows []hot.PostgresReplicationLagDetailRow
	now := time.Now().UTC()
	if stats.IsPrimary {
		for _, standby := range stats.Standbys {
			hotRows = append(hotRows, hot.PostgresReplicationLagDetailRow{
				CaptureTimestamp: now,
				ServerID:         serverID,
				ReplicaName:      standby.ReplicaPodName,
				LagMB:            standby.ReplayLagMB,
				State:            standby.State,
				SyncState:        standby.SyncState,
				WriteLagSec:      standby.WriteLagSec,
				FlushLagSec:      standby.FlushLagSec,
				ReplayLagSec:     standby.ReplayLagSec,
			})
		}
	} else {
		// Log local lag for the standby itself
		hotRows = append(hotRows, hot.PostgresReplicationLagDetailRow{
			CaptureTimestamp: now,
			ServerID:         serverID,
			ReplicaName:      "local",
			LagMB:            stats.LocalLagMB,
			State:            "streaming",
			SyncState:        "unknown",
			ReplayLagSec:     stats.MaxLagMB, // Using MaxLagMB which holds local lag for standbys
		})
	}

	if len(hotRows) == 0 {
		return
	}
	if err := c.tsLogger.LogPostgresReplicationLagDetail(ctx, hotRows); err != nil {
		slog.Error("[PgComprehensive] LogPostgresReplicationLagDetail error", "target", name, "err", err)
	}
}

func (c *PgComprehensiveCollector) collectSettingsSnapshot(ctx context.Context, name string, serverID uuid.UUID) {
	settings, err := c.pgRepo.GetSettingsSnapshot(ctx, name)
	if err != nil {
		slog.Error("[PgComprehensive] GetSettingsSnapshot error", "target", name, "err", err)
		return
	}
	var hotRows []hot.PostgresSettingSnapshotRow
	now := time.Now().UTC()
	for _, s := range settings {
		hotRows = append(hotRows, hot.PostgresSettingSnapshotRow{
			Timestamp:        now,
			ServerID:         serverID,
			Name:             s.Name,
			Setting:          s.Setting,
			Unit:             s.Unit,
			Source:           s.Source,
		})
	}
	if err := c.tsLogger.LogPostgresSettingsSnapshot(ctx, serverID, hotRows); err != nil {
		slog.Error("[PgComprehensive] LogPostgresSettingsSnapshot error", "target", name, "err", err)
	}
}

func (c *PgComprehensiveCollector) collectDeadlockDeltas(ctx context.Context, name string, serverID uuid.UUID) {
	rows, err := c.pgRepo.GetDeadlocksTotalByDB(ctx, name)
	if err != nil {
		slog.Error("[PgComprehensive] GetDeadlocksTotalByDB error", "target", name, "err", err)
		return
	}
	totals := make(map[string]int64, len(rows))
	for _, r := range rows {
		totals[r.DatabaseName] = r.DeadlocksTotal
	}
	if err := c.tsLogger.LogPostgresDeadlocksDelta(ctx, serverID, totals); err != nil {
		slog.Error("[PgComprehensive] LogPostgresDeadlocksDelta error", "target", name, "err", err)
	}
}
