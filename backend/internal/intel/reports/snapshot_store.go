// Package intel provides the SQL Optima autonomous health intelligence engine.
// This file persists computed risk scores and report artifacts to intelreport.intel_snapshots.
//
// Design context:
//   - DEFECT-3 fix: persisting the computed overall_risk (and dimension scores) allows the
//     historical trend chart to read from stored values rather than recomputing from raw
//     metrics — ensuring trend and live score use identical formulas.
//   - Only IntelligenceReportService writes to intelreport.intel_snapshots; collectors do not.
//   - The full report_html and report_json are optional: stored when format=html is generated.
//   - Failures are non-fatal: if the snapshot write fails the caller still returns the report.
//
// SQL Optima — https://github.com/rsharma155/sql_optima
// Copyright (c) 2026 Ravi Sharma. SPDX-License-Identifier: MIT

package reports

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rsharma155/sql_optima/internal/models"
)

// SnapshotWriter is the minimal DB interface needed to persist intel snapshots.
type SnapshotWriter interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// SnapshotStore persists IntelligenceReportResponse to intelreport.intel_snapshots.
type SnapshotStore struct {
	pool SnapshotWriter
}

func NewSnapshotStore(pool SnapshotWriter) *SnapshotStore {
	return &SnapshotStore{pool: pool}
}

// Save writes a snapshot row for the given server and report. Returns the generated run_id.
// Non-fatal: if the INSERT fails, the error is returned but the caller should continue.
func (s *SnapshotStore) Save(ctx context.Context, serverID uuid.UUID, report *models.IntelligenceReportResponse, reportHTML string) (string, error) {
	if s.pool == nil || report == nil {
		return "", nil
	}

	reportJSON, _ := json.Marshal(report)

	var utilizationClass string
	if report.UtilizationProfile != nil {
		utilizationClass = report.UtilizationProfile.Class
	}

	var cpuP50, cpuP95, cpuAvg, memP95 float64
	if report.UtilizationProfile != nil {
		cpuP50 = report.UtilizationProfile.CPUP50
		cpuP95 = report.UtilizationProfile.CPUP95
		cpuAvg = report.UtilizationProfile.CPUAvg
		memP95 = report.UtilizationProfile.MemP95
	}

	var diskUsedPct float64
	var diskDaysRemaining int
	if report.CapacityForecastV2 != nil {
		diskUsedPct = report.CapacityForecastV2.UsedPct
		diskDaysRemaining = report.CapacityForecastV2.DaysUntilBreach
		if diskDaysRemaining < 0 {
			diskDaysRemaining = 0
		}
	}

	criticalCount, highCount := 0, 0
	for _, r := range report.TriggeredRules {
		switch r.Severity {
		case "critical":
			criticalCount++
		case "high":
			highCount++
		}
	}

	// Compute data completeness as fraction of key metrics present in the report.
	dataCoverage := report.ConfidenceScore
	if dataCoverage > 1.0 {
		dataCoverage = 1.0
	}

	captureTimestamp := time.Now().UTC()
	if report.GeneratedAt != "" {
		if t, err := time.Parse(time.RFC3339, report.GeneratedAt); err == nil {
			captureTimestamp = t
		}
	}

	var runID string
	err := s.pool.QueryRow(ctx, `
		INSERT INTO intelreport.intel_snapshots (
			server_id, capture_timestamp,
			overall_risk, performance_risk, capacity_risk,
			availability_risk, replication_risk, maintenance_risk, query_risk,
			utilization_class,
			cpu_p50, cpu_p95, cpu_avg, mem_p95,
			ple_current, disk_used_pct, disk_days_remaining,
			data_completeness, data_coverage_days,
			rule_count_critical, rule_count_high,
			report_json, report_html
		) VALUES (
			$1, $2,
			$3, $4, $5,
			$6, $7, $8, $9,
			$10,
			$11, $12, $13, $14,
			$15, $16, $17,
			$18, $19,
			$20, $21,
			$22, $23
		)
		RETURNING run_id::text
	`,
		serverID, captureTimestamp,
		report.OverallRisk.OverallScore, report.OverallRisk.PerformanceRisk, report.OverallRisk.CapacityRisk,
		report.OverallRisk.AvailabilityRisk, report.OverallRisk.ReplicationRisk, report.OverallRisk.MaintenanceRisk, report.OverallRisk.QueryRisk,
		utilizationClass,
		cpuP50, cpuP95, cpuAvg, memP95,
		0.0, // ple_current — populated from raw data if available
		diskUsedPct, diskDaysRemaining,
		dataCoverage, 14, // data_coverage_days defaulted to 14-day window
		criticalCount, highCount,
		reportJSON, reportHTML,
	).Scan(&runID)

	return runID, err
}

// LoadHistoryTrend reads the last 30 days of intel snapshots for a server.
func (s *SnapshotStore) LoadHistoryTrend(ctx context.Context, serverID uuid.UUID) ([]models.IntelSnapshotEntry, error) {
	if s.pool == nil {
		return nil, nil
	}

	rows, err := s.pool.Query(ctx, `
		SELECT
			capture_timestamp::text,
			overall_risk, performance_risk, capacity_risk, availability_risk,
			replication_risk, maintenance_risk, query_risk,
			utilization_class, cpu_p95, ple_current, disk_used_pct,
			rule_count_critical, rule_count_high
		FROM intelreport.intel_snapshots
		WHERE server_id = $1
		  AND capture_timestamp > NOW() - INTERVAL '30 days'
		ORDER BY capture_timestamp ASC
		LIMIT 60
	`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []models.IntelSnapshotEntry
	for rows.Next() {
		var e models.IntelSnapshotEntry
		if scanErr := rows.Scan(
			&e.Timestamp,
			&e.OverallRisk, &e.PerformanceRisk, &e.CapacityRisk, &e.AvailabilityRisk,
			&e.ReplicationRisk, &e.MaintenanceRisk, &e.QueryRisk,
			&e.UtilizationClass, &e.CPUP95, &e.PLECurrent, &e.DiskUsedPct,
			&e.CriticalCount, &e.HighCount,
		); scanErr == nil {
			entries = append(entries, e)
		}
	}
	return entries, rows.Err()
}

// LatestSnapshot holds the most recent persisted intelligence report for a server.
type LatestSnapshot struct {
	RunID        string
	GeneratedAt  time.Time
	ReportHTML   string
	OverallScore float64
	Category     string
	DataStatus   string
}

// LoadLatest returns the newest snapshot within maxAge, if any.
func (s *SnapshotStore) LoadLatest(ctx context.Context, serverID uuid.UUID, maxAge time.Duration) (*LatestSnapshot, error) {
	if s.pool == nil {
		return nil, nil
	}
	if maxAge <= 0 {
		maxAge = 24 * time.Hour
	}

	var snap LatestSnapshot
	var reportJSON []byte
	cutoff := time.Now().UTC().Add(-maxAge)
	err := s.pool.QueryRow(ctx, `
		SELECT run_id::text, capture_timestamp, COALESCE(report_html, ''),
		       overall_risk, report_json
		FROM intelreport.intel_snapshots
		WHERE server_id = $1
		  AND capture_timestamp >= $2
		  AND report_html IS NOT NULL
		  AND length(report_html) > 100
		ORDER BY capture_timestamp DESC
		LIMIT 1
	`, serverID, cutoff).Scan(
		&snap.RunID, &snap.GeneratedAt, &snap.ReportHTML,
		&snap.OverallScore, &reportJSON,
	)
	if err != nil {
		return nil, err
	}

	if len(reportJSON) > 0 {
		var report models.IntelligenceReportResponse
		if json.Unmarshal(reportJSON, &report) == nil {
			snap.Category = report.OverallRisk.Category
			snap.DataStatus = report.DataStatus
			if snap.DataStatus == "" {
				snap.DataStatus = "Full"
			}
		}
	}
	return &snap, nil
}
