// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Collector logic for PostgreSQL incident feed.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package service

import (
	"log/slog"
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/rsharma155/sql_optima/internal/models"
	"github.com/rsharma155/sql_optima/internal/repository"
	"github.com/rsharma155/sql_optima/internal/storage/hot"
)

func (s *MetricsService) CollectPgIncidents(serverID uuid.UUID) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	instanceName := ""
	for _, inst := range s.Config.Instances {
		if inst.ServerID == serverID {
			instanceName = inst.Name
			break
		}
	}
	if instanceName == "" {
		return
	}

	db, ok := s.PgRepo.GetConn(instanceName)
	if !ok {
		return
	}

	now := time.Now().UTC()
	locks, _ := s.PgRepo.FetchDetailedLocks(ctx, instanceName, db)
	queries, _ := s.PgRepo.GetActiveQueries(instanceName)

	// Compute deadlock rate if possible (requires historical state)
	deadlockRate := 0.0

	rows := buildIncidentRows(serverID, now, locks, queries, deadlockRate)
	if len(rows) > 0 {
		if err := s.tsLogger.LogPgIncidentFeed(ctx, rows); err != nil {
			slog.Error("[Incidents] ERROR LogPgIncidentFeed", "target", serverID, "err", err)
		}
	}
}

func buildIncidentRows(serverID uuid.UUID, now time.Time, locks []repository.PGTimescaleLockInternal, queries []models.PgSession, deadlockRate float64) []hot.PgIncidentFeedRow {
	var out []hot.PgIncidentFeedRow

	// 1. Blocking: any lock with a blocker creates an incident
	blockerMap := make(map[int]int)
	for _, l := range locks {
		if l.BlockedBy > 0 {
			blockerMap[l.BlockedBy]++
		}
	}
	for pid, victims := range blockerMap {
		var blocker repository.PGTimescaleLockInternal
		for _, l := range locks {
			if l.PID == pid {
				blocker = l
				break
			}
		}
		severity := "warning"
		if victims >= 10 {
			severity = "critical"
		}
		out = append(out, hot.PgIncidentFeedRow{
			Ts:           now,
			ServerID:     serverID,
			IncidentType: "blocking",
			Severity:     severity,
			PID:          pid,
			BlockedCount: victims,
			Usename:      blocker.DatabaseName,
			Datname:      blocker.DatabaseName,
			QuerySnippet: blocker.QueryText,
		})
	}

	// 2. Long Running Queries (>= 5 seconds; critical at >= 5 minutes)
	const longQueryWarnMs = 5000.0
	const longQueryCritMs = 300000.0
	for _, q := range queries {
		if q.DurationMs < longQueryWarnMs {
			continue
		}
		severity := "warning"
		if q.DurationMs >= longQueryCritMs {
			severity = "critical"
		}
		out = append(out, hot.PgIncidentFeedRow{
			Ts:           now,
			ServerID:     serverID,
			IncidentType: "long_query",
			Severity:     severity,
			PID:          q.PID,
			DurationMs:   q.DurationMs,
			Usename:      q.UserName,
			Datname:      q.Database,
			QuerySnippet: q.Query,
		})
	}

	// 3. Deadlocks
	if deadlockRate > 0 {
		out = append(out, hot.PgIncidentFeedRow{
			Ts:           now,
			ServerID:     serverID,
			IncidentType: "deadlock",
			Severity:     "critical",
			DurationMs:   deadlockRate,
		})
	}

	return out
}
