// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL memory intelligence service layer.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package service

import (
	"context"
	"fmt"
	"log"
	"time"
)

// CollectAndLogPgMemory handles the end-to-end memory metrics collection and storage cycle.
func (s *MetricsService) CollectAndLogPgMemory(ctx context.Context, instanceName string) error {
	log.Printf("[PgMemoryService] Starting collection for %s", instanceName)
	if s.tsLogger == nil {
		return fmt.Errorf("tsLogger is nil")
	}

	// 1. Collect OS memory
	hostSnap, err := s.PgRepo.CollectPgHostMemory(instanceName)
	if err != nil {
		log.Printf("[PgMemoryService] Failed to collect host memory for %s: %v", instanceName, err)
	} else {
		if err := s.tsLogger.LogPgHostMemory(ctx, hostSnap); err != nil {
			log.Printf("[PgMemoryService] Failed to log host memory for %s: %v", instanceName, err)
		}
	}

	// 2. Collect PG memory stats
	pgSnap, err := s.PgRepo.CollectPgMemoryStats(ctx, instanceName)
	if err != nil {
		log.Printf("[PgMemoryService] Failed to collect PG memory stats for %s: %v", instanceName, err)
	} else {
		if err := s.tsLogger.LogPgMemoryStats(ctx, pgSnap); err != nil {
			log.Printf("[PgMemoryService] Failed to log PG memory stats for %s: %v", instanceName, err)
		} else {
			log.Printf("[PgMemoryService] Successfully persisted PG memory stats for %s at %v", instanceName, pgSnap.Timestamp)
		}
	}

	// 3. Collect configuration (less frequent - every 15 min)
	// We'll use a simple time-based check here or just do it every time for now (it's cheap)
	compSnap, err := s.PgRepo.CollectPgMemoryComponents(ctx, instanceName)
	if err == nil {
		if err := s.tsLogger.LogPgMemoryComponents(ctx, compSnap); err != nil {
			log.Printf("[PgMemoryService] Failed to log PG memory components for %s: %v", instanceName, err)
		}
	}

	// 4. Compute derived metrics
	if err := s.tsLogger.ComputeAndLogPgMemoryDerived(ctx, instanceName); err != nil {
		log.Printf("[PgMemoryService] Failed to compute derived metrics for %s: %v", instanceName, err)
	}

	return nil
}

// GetPgMemoryDashboardData returns all time-series data required for the memory dashboard.
func (s *MetricsService) GetPgMemoryDashboardData(ctx context.Context, instanceName string, from, to time.Time) (map[string]interface{}, error) {
	if s.tsLogger == nil {
		return nil, fmt.Errorf("tsLogger not available")
	}

	data, err := s.tsLogger.GetPgMemoryTimeSeries(ctx, instanceName, from, to)
	if err != nil {
		return nil, err
	}

	// On-demand collection if data is missing or very old
	series, ok := data["time_series"].([]map[string]interface{})
	if !ok || len(series) == 0 {
		log.Printf("[PgMemoryService] No time-series data for %s in range [%v - %v], triggering on-demand collection", instanceName, from, to)
		if cerr := s.CollectAndLogPgMemory(ctx, instanceName); cerr != nil {
			log.Printf("[PgMemoryService] On-demand collection failed for %s: %v", instanceName, cerr)
			return nil, fmt.Errorf("on-demand collection failed: %w", cerr)
		}
		
		// Re-fetch once after on-demand collection. 
		// Use a much wider window for the re-fetch to ensure that even if there's clock/timezone skew,
		// the user sees the data they just triggered.
		nowUTC := time.Now().UTC()
		wideTo := nowUTC.Add(5 * time.Minute)
		wideFrom := from.Add(-1 * time.Hour)
		
		// If the requested 'from' was in the future (due to timezone skew), 
		// ensure wideFrom covers 'now'.
		if wideFrom.After(nowUTC) {
			wideFrom = nowUTC.Add(-1 * time.Hour)
		}
		
		data, err = s.tsLogger.GetPgMemoryTimeSeries(ctx, instanceName, wideFrom, wideTo)
		if err != nil {
			return nil, err
		}
	}

	// Check if OS collector is configured
	osConfigured := false
	if s.ServerRepo != nil {
		srv, err := s.ServerRepo.GetByName(ctx, instanceName)
		if err == nil {
			status, _ := s.GetOSCollectorStatus(ctx, srv.Host)
			osConfigured = status
		}
	}

	data["os_collector_configured"] = osConfigured
	return data, nil
}
