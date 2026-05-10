// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL incident feed collector — captures blocking events, long-running
//          queries, and deadlock spikes per collection cycle into TimescaleDB.
//          Eliminates live ad-hoc queries from the dashboard.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package service

import (
	"context"
	"log"
	"time"

	"github.com/rsharma155/sql_optima/internal/models"
	"github.com/rsharma155/sql_optima/internal/repository"
	"github.com/rsharma155/sql_optima/internal/storage/hot"
)

// CollectPgIncidents performs a sweep of the monitored instance for operational incidents.
func (s *MetricsService) CollectPgIncidents(instanceName string) {
	if s.tsLogger == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	now := time.Now().UTC()

	// 1. Fetch data
	locks, _ := s.PgRepo.FetchDetailedLocks(instanceName)
	activeQueries, _ := s.PgRepo.GetActiveQueries(instanceName)
	dlTotal, _ := s.PgRepo.FetchDeadlocksTotalAllDBs(instanceName)
	
	dlRate := 0.0
	if s.tsLogger != nil {
		if r, ok := s.tsLogger.ComputePgDeadlockRate(instanceName, dlTotal, 60.0); ok {
			dlRate = r
		}
	}

	// 2. Build rows
	incidents := buildIncidentRows(instanceName, now, locks, activeQueries, dlRate)

	// 3. Persist
	if len(incidents) > 0 {
		if err := s.tsLogger.LogPgIncidentFeed(ctx, incidents); err != nil {
			log.Printf("[IncidentCollector] ERROR: Failed to log incidents for %s: %v", instanceName, err)
		}
	}
}

func buildIncidentRows(instanceName string, now time.Time, locks []repository.PGTimescaleLockInternal, queries []models.PgSession, deadlockRate float64) []hot.PgIncidentFeedRow {
	var incidents []hot.PgIncidentFeedRow

	// 1. Check for blocking incidents
	for _, l := range locks {
		if l.WaitDurationMs > 0 && l.BlockedBy != 0 {
			severity := "warning"
			// In a more advanced implementation, we'd count how many are blocked by this PID.
			
			incidents = append(incidents, hot.PgIncidentFeedRow{
				Ts:           now,
				InstanceID:   instanceName,
				IncidentType: "blocking",
				Severity:     severity,
				PID:          l.PID,
				DurationMs:   l.WaitDurationMs,
				Datname:      l.DatabaseName,
				QuerySnippet: l.QueryText,
			})
		}
	}

	// 2. Check for long running queries
	// threshold: 5 seconds (5000ms)
	longQueryThresholdMs := 5000.0
	for _, q := range queries {
		if q.DurationMs > longQueryThresholdMs {
			severity := "warning"
			if q.DurationMs > 300000 { // 5 minutes
				severity = "critical"
			}

			incidents = append(incidents, hot.PgIncidentFeedRow{
				Ts:           now,
				InstanceID:   instanceName,
				IncidentType: "long_query",
				Severity:     severity,
				PID:          q.PID,
				DurationMs:   q.DurationMs,
				Usename:      q.UserName,
				Datname:      q.Database,
				QuerySnippet: q.Query,
			})
		}
	}

	// 3. Check for deadlocks
	if deadlockRate > 0 {
		incidents = append(incidents, hot.PgIncidentFeedRow{
			Ts:           now,
			InstanceID:   instanceName,
			IncidentType: "deadlock",
			Severity:     "critical",
			DurationMs:   deadlockRate,
		})
	}

	return incidents
}

