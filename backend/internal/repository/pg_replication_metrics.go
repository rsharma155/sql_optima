// Package repository provides data access layer for database operations.
// It handles connections and queries for both PostgreSQL and SQL Server databases.
// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL replication lag and standby health monitoring.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"log/slog"
	"context"
	"fmt"
	"strings"

	"github.com/rsharma155/sql_optima/internal/models"
)

// GetReplicationLag returns replication lag in MB for standby servers.
// Returns status: "primary", "standby", or "unknown"
func (c *PgRepository) GetReplicationLag(ctx context.Context, instanceName string) (lagMB float64, status string, err error) {
	c.mutex.RLock()
	db, ok := c.conns[strings.ToUpper(instanceName)]
	c.mutex.RUnlock()

	if !ok || db == nil {
		return 0, "unknown", fmt.Errorf("connection not found")
	}

	// Check if this is a standby using pg_is_in_recovery()
	var isStandby bool
	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	err = db.QueryRowContext(ctx, "SELECT /* SQL_OPTIMA */   pg_is_in_recovery()").Scan(&isStandby)
	if err != nil {
		return 0, "unknown", err
	}

	if !isStandby {
		return 0, "primary", nil
	}

	// Calculate lag on standby using LSN difference
	query := `
		/* SQL_OPTIMA */ SELECT   
			CASE 
				WHEN pg_last_wal_replay_lsn() IS NOT NULL AND pg_last_wal_receive_lsn() IS NOT NULL 
				THEN EXTRACT(EPOCH FROM (pg_last_wal_receive_lsn() - pg_last_wal_replay_lsn())) / 1024 / 1024
				ELSE 0 
			END as lag_mb
	`
	ctx, cancel = WithQueryTimeout(ctx, 0)
	defer cancel()
	err = db.QueryRowContext(ctx, query).Scan(&lagMB)
	if err != nil {
		return 0, "standby", err
	}

	status = "standby"
	return
}

// GetReplicationStats returns detailed replication information including standby lag.
func (c *PgRepository) GetReplicationStats(ctx context.Context, instanceName string) (*models.PgReplicationStats, error) {
	c.mutex.RLock()
	db, ok := c.conns[strings.ToUpper(instanceName)]
	c.mutex.RUnlock()

	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}

	stats := &models.PgReplicationStats{}

	// Best-effort HA "cluster_name" signal (often set by operators).
	var clusterName string
	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	_ = db.QueryRowContext(ctx, "SELECT /* SQL_OPTIMA */   COALESCE(setting,'') FROM pg_settings WHERE name='cluster_name'").Scan(&clusterName)
	var appNames []string

	// Step 1: Determine Node Role
	var isStandby bool
	ctx, cancel = WithQueryTimeout(ctx, 0)
	defer cancel()
	err := db.QueryRowContext(ctx, "SELECT /* SQL_OPTIMA */   pg_is_in_recovery() AS is_standby").Scan(&isStandby)
	if err != nil {
		return nil, fmt.Errorf("failed to determine node role: %w", err)
	}
	stats.IsPrimary = !isStandby

	if stats.IsPrimary {
		// Step 2 (Primary): Fetch Connected CNPG Replicas from pg_stat_replication
		stats.ClusterState = "primary"

		ctx, cancel = WithQueryTimeout(ctx, 0)
		defer cancel()
		rows, err := db.QueryContext(ctx, `
			/* SQL_OPTIMA */
			SELECT
				application_name AS replica_pod_name,
				COALESCE(client_addr::text, '') AS pod_ip,
				COALESCE(state, '') AS state,
				COALESCE(sync_state, '') AS sync_state,
				COALESCE(sent_lsn::text, '') AS sent_lsn,
				COALESCE(pg_wal_lsn_diff(pg_current_wal_lsn(), replay_lsn) / 1024.0 / 1024.0, 0) AS replay_lag_mb,
				COALESCE(EXTRACT(EPOCH FROM write_lag), 0)  AS write_lag_sec,
				COALESCE(EXTRACT(EPOCH FROM flush_lag), 0)  AS flush_lag_sec,
				COALESCE(EXTRACT(EPOCH FROM replay_lag), 0) AS replay_lag_sec
			FROM pg_stat_replication
			ORDER BY application_name
		`)
		if err != nil {
			slog.Error("[POSTGRES] GetReplicationStats: pg_stat_replication query failed", "target", instanceName, "err", err)
			stats.Standbys = []models.PgReplicationStat{}
		} else {
			defer rows.Close()

			var maxLagMB float64
			for rows.Next() {
				var stat models.PgReplicationStat
				if err := rows.Scan(&stat.ReplicaPodName, &stat.PodIP, &stat.State, &stat.SyncState, &stat.SentLSN,
					&stat.ReplayLagMB, &stat.WriteLagSec, &stat.FlushLagSec, &stat.ReplayLagSec); err != nil {
					slog.Error("[POSTGRES] GetReplicationStats: scan error", "target", instanceName, "err", err)
					continue
				}
				appNames = append(appNames, stat.ReplicaPodName)
				stats.Standbys = append(stats.Standbys, stat)
				if stat.ReplayLagMB > maxLagMB {
					maxLagMB = stat.ReplayLagMB
				}
			}
			stats.MaxLagMB = maxLagMB
			if stats.Standbys == nil {
				stats.Standbys = []models.PgReplicationStat{}
			}
		}
	} else {
		// Step 3 (Standby): Fetch Local Replay Lag
		stats.ClusterState = "standby"

		var localLagMB float64
		ctx, cancel = WithQueryTimeout(ctx, 0)
		defer cancel()
		err = db.QueryRowContext(ctx, `
			/* SQL_OPTIMA */ SELECT   
				CASE 
					WHEN pg_last_wal_receive_lsn() = pg_last_wal_replay_lsn() THEN 0
					ELSE COALESCE(pg_wal_lsn_diff(pg_last_wal_receive_lsn(), pg_last_wal_replay_lsn()) / 1024.0 / 1024.0, 0)
				END AS local_replay_lag_mb
		`).Scan(&localLagMB)
		if err != nil {
			slog.Error("[POSTGRES] GetReplicationStats: local lag query failed", "target", instanceName, "err", err)
			stats.LocalLagMB = 0
		} else {
			stats.LocalLagMB = localLagMB
			stats.MaxLagMB = localLagMB
		}
		stats.Standbys = []models.PgReplicationStat{}
	}

	// clusterName/appNames are used by the API handler to determine HA provider.
	_ = clusterName
	_ = appNames

	// Get WAL generation rate (approximation)
	var walRate float64
	ctx, cancel = WithQueryTimeout(ctx, 0)
	defer cancel()
	err = db.QueryRowContext(ctx, `
		/* SQL_OPTIMA */ SELECT   
			CASE 
				WHEN pg_is_in_recovery() = false AND pg_current_wal_lsn() IS NOT NULL 
				THEN 0
				ELSE 0
			END
	`).Scan(&walRate)
	if err == nil {
		stats.WalGenRateMBps = walRate
	}

	// Get BGWriter efficiency
	var buffersBackend, maxwrittenClean int64
	var versionNum int
	ctx, cancel = WithQueryTimeout(ctx, 0)
	defer cancel()
	if err := db.QueryRowContext(ctx, "SELECT current_setting('server_version_num')::integer").Scan(&versionNum); err != nil {
		versionNum = 120000
	}

	var bgQuery string
	if versionNum >= 170000 {
		bgQuery = "SELECT /* SQL_OPTIMA */ 0, maxwritten_clean FROM pg_stat_bgwriter"
	} else {
		bgQuery = "SELECT /* SQL_OPTIMA */ buffers_backend, maxwritten_clean FROM pg_stat_bgwriter"
	}

	ctx, cancel = WithQueryTimeout(ctx, 0)
	defer cancel()
	err = db.QueryRowContext(ctx, bgQuery).Scan(&buffersBackend, &maxwrittenClean)
	if err == nil && (buffersBackend+maxwrittenClean) > 0 {
		stats.BgWriterEffPct = float64(buffersBackend) / float64(buffersBackend+maxwrittenClean) * 100
	}

	return stats, nil
}
