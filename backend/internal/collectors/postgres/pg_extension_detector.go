// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Detect available PostgreSQL monitoring extensions and cache the result.
//
// Metadata:
//   Type: Collector Utility
//   Package: postgres
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package postgres

import (
	"log/slog"
	"context"
	"database/sql"
	"sync"
	"time"
)

type PgQueryStatsSource string

const (
	PgStatMonitor    PgQueryStatsSource = "pg_stat_monitor"
	PgStatStatements PgQueryStatsSource = "pg_stat_statements"
	PgQueryStatsNone PgQueryStatsSource = "none"
)

type ExtensionDetector struct {
	mu           sync.RWMutex
	source       PgQueryStatsSource
	lastChecked  time.Time
	cacheTimeout time.Duration
}

func NewExtensionDetector() *ExtensionDetector {
	return &ExtensionDetector{
		source:       PgQueryStatsNone,
		cacheTimeout: 5 * time.Minute,
	}
}

func (d *ExtensionDetector) GetSource(ctx context.Context, db *sql.DB) PgQueryStatsSource {
	d.mu.RLock()
	if time.Since(d.lastChecked) < d.cacheTimeout && d.source != PgQueryStatsNone {
		source := d.source
		d.mu.RUnlock()
		return source
	}
	d.mu.RUnlock()

	d.mu.Lock()
	defer d.mu.Unlock()

	// Re-check cache in case another goroutine updated it while we were waiting for the lock
	if time.Since(d.lastChecked) < d.cacheTimeout && d.source != PgQueryStatsNone {
		return d.source
	}

	d.source = d.detectSource(ctx, db)
	d.lastChecked = time.Now()

	return d.source
}

func (d *ExtensionDetector) detectSource(ctx context.Context, db *sql.DB) PgQueryStatsSource {
	// First, check what extensions are installed
	query := `
		SELECT extname 
		FROM pg_extension 
		WHERE extname IN ('pg_stat_monitor', 'pg_stat_statements')
		ORDER BY CASE WHEN extname = 'pg_stat_monitor' THEN 1 ELSE 2 END;
	`
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		slog.Error("[PgExtensionDetector] ERROR: Failed to query pg_extension", "err", err)
		return PgQueryStatsNone
	}
	defer rows.Close()

	var installed []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err == nil {
			installed = append(installed, name)
		}
	}

	for _, ext := range installed {
		if ext == "pg_stat_monitor" {
			// Verify if pg_stat_monitor is actually LOADED and functional.
			// The extension might be created but not in shared_preload_libraries.
			var dummy int
			err := db.QueryRowContext(ctx, "SELECT 1 FROM pg_stat_monitor LIMIT 1").Scan(&dummy)
			if err == nil {
				slog.Info("[PgExtensionDetector] Detected functional pg_stat_monitor.")
				return PgStatMonitor
			}
			slog.Error("[PgExtensionDetector] WARN: pg_stat_monitor is installed but NOT functional (likely missing from shared_preload_libraries)", "err", err)
			// Continue to check if pg_stat_statements is available
		}
		if ext == "pg_stat_statements" {
			// Verify pg_stat_statements
			var dummy int
			err := db.QueryRowContext(ctx, "SELECT 1 FROM pg_stat_statements LIMIT 1").Scan(&dummy)
			if err == nil {
				slog.Info("[PgExtensionDetector] Detected functional pg_stat_statements.")
				return PgStatStatements
			}
			slog.Warn("[PgExtensionDetector] WARN: pg_stat_statements is installed but NOT functional", "err", err)
		}
	}

	slog.Info("[PgExtensionDetector] INFO: No functional query monitoring extension found.")
	return PgQueryStatsNone
}
