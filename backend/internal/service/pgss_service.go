// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Service-layer methods for the enhanced pg_stat_statements dashboard,
//
//	orchestrating calls between the repository, TimescaleDB storage, and API handlers.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package service

import (
	"context"
	"strings"
	"time"

	"github.com/rsharma155/sql_optima/internal/models"
)

// GetPgssStatus checks whether query monitoring extensions (pgss or pgsm) are active.
func (s *MetricsService) GetPgssStatus(ctx context.Context, instanceName string) models.PgssStatusResponse {
	resp := models.PgssStatusResponse{Instance: instanceName}

	if s.PgRepo == nil {
		resp.Message = "instance connection not available"
		return resp
	}

	db, ok := s.PgRepo.GetConn(instanceName)
	if !ok || db == nil {
		resp.Message = "instance connection not available"
		return resp
	}

	// Check shared_preload_libraries
	var libs string
	if err := db.QueryRowContext(ctx, "SHOW shared_preload_libraries").Scan(&libs); err == nil {
		resp.SharedPreloadActive = strings.Contains(libs, "pg_stat_statements") || strings.Contains(libs, "pg_stat_monitor")
	}

	// Check extension installed (statements or monitor)
	var extName string
	err := db.QueryRowContext(ctx, "SELECT extname FROM pg_extension WHERE extname IN ('pg_stat_monitor', 'pg_stat_statements') ORDER BY CASE WHEN extname = 'pg_stat_monitor' THEN 1 ELSE 2 END LIMIT 1").Scan(&extName)
	if err == nil {
		resp.ExtensionInstalled = true
		if extName == "pg_stat_monitor" {
			resp.Message = "Enhanced monitoring active (pg_stat_monitor)."
		} else {
			resp.Message = "Standard monitoring active (pg_stat_statements). pg_stat_monitor is optional but recommended."
		}
	} else {
		resp.ExtensionInstalled = false
	}

	resp.Ready = resp.SharedPreloadActive && resp.ExtensionInstalled
	if !resp.Ready {
		if !resp.SharedPreloadActive {
			resp.Message = "Neither pg_stat_statements nor pg_stat_monitor found in shared_preload_libraries (requires restart)."
		} else {
			resp.Message = "Query stats extension not created. Run 'CREATE EXTENSION pg_stat_statements' (minimum) or 'CREATE EXTENSION pg_stat_monitor' (optional/enhanced)."
		}
	} else if s.tsLogger == nil {
		resp.Ready = false 
		resp.Message = "TimescaleDB not connected. Query performance charts require the metrics repository."
	}
	return resp
}

// GetPgssWorkload returns per-minute workload time-series from the pgss_delta_1m table.
func (s *MetricsService) GetPgssWorkload(ctx context.Context, instanceName string, from, to time.Time) ([]models.PgssWorkloadPoint, error) {
	if s.tsLogger == nil {
		return nil, nil
	}
	hotPts, err := s.tsLogger.GetPgssWorkloadTimeSeries(ctx, instanceName, from, to)
	if err != nil {
		return nil, err
	}
	pts := make([]models.PgssWorkloadPoint, len(hotPts))
	for i, h := range hotPts {
		pts[i] = models.PgssWorkloadPoint{
			Timestamp: h.Timestamp, QueryLoadMsSec: h.QueryLoadMsSec,
			QPS: h.QPS, RowsSec: h.RowsSec, CacheHitRatio: h.CacheHitRatio,
			WalBytesSec: h.WalBytesSec, PlanningMsSec: h.PlanningMsSec,
			ExecPct: h.ExecPct, PlanPct: h.PlanPct,
			RowsPerQuery: h.RowsPerQuery, TempMbSec: h.TempMbSec,
			BlksReadSec: h.BlksReadSec, TempBlksWrittenSec: h.TempBlksWrittenSec,
			CpuSaturationMsSec: h.CpuSaturationMsSec,
		}
	}
	return pts, nil
}

// GetPgssLatency returns latency percentile approximations from pgss_delta_1m.
func (s *MetricsService) GetPgssLatency(ctx context.Context, instanceName string, from, to time.Time) ([]models.PgssLatencyPoint, error) {
	if s.tsLogger == nil {
		return nil, nil
	}
	hotPts, err := s.tsLogger.GetPgssLatencyTimeSeries(ctx, instanceName, from, to)
	if err != nil {
		return nil, err
	}
	pts := make([]models.PgssLatencyPoint, len(hotPts))
	for i, h := range hotPts {
		pts[i] = models.PgssLatencyPoint{
			Timestamp: h.Timestamp, P50: h.P50, P95: h.P95, P99: h.P99, MaxExec: h.MaxExec,
		}
	}
	return pts, nil
}

// GetPgssTopQueries returns top queries from pgss_delta_1m for a time window.
func (s *MetricsService) GetPgssTopQueries(ctx context.Context, instanceName string, from, to time.Time, sortBy string, limit int) ([]models.PgssTopQuery, error) {
	if s.tsLogger == nil {
		return nil, nil
	}
	hotQ, err := s.tsLogger.GetPgssTopQueries(ctx, instanceName, from, to, sortBy, limit)
	if err != nil {
		return nil, err
	}
	q := make([]models.PgssTopQuery, len(hotQ))
	for i, h := range hotQ {
		q[i] = models.PgssTopQuery{
			QueryID: h.QueryID, Query: h.Query, TotalTime: h.TotalTime, PctDBTime: h.PctDBTime,
			Calls: h.Calls, AvgMs: h.AvgMs, RowsPerCall: h.RowsPerCall,
			HitPct: h.HitPct, TempMB: h.TempMB, WalMB: h.WalMB,
			ReadsPerCall: h.ReadsPerCall, PlanRatio: h.PlanRatio, Flags: h.Flags,
		}
	}
	return q, nil
}

// GetPgssRegressions returns queries that degraded between the last 30m and previous 30m.
func (s *MetricsService) GetPgssRegressions(ctx context.Context, instanceName string) ([]models.PgssRegression, error) {
	if s.tsLogger == nil {
		return nil, nil
	}
	hotR, err := s.tsLogger.GetPgssRegressions(ctx, instanceName)
	if err != nil {
		return nil, err
	}
	r := make([]models.PgssRegression, len(hotR))
	for i, h := range hotR {
		r[i] = models.PgssRegression{
			QueryID: h.QueryID, Query: h.Query, PrevAvgMs: h.PrevAvgMs,
			CurrAvgMs: h.CurrAvgMs, ChangePct: h.ChangePct, Status: h.Status,
		}
	}
	return r, nil
}

// GetPgssSummary returns aggregate KPI metrics for the incident summary strip.
func (s *MetricsService) GetPgssSummary(ctx context.Context, instanceName string, from, to time.Time) (*models.PgssSummaryResponse, error) {
	if s.tsLogger == nil {
		return nil, nil
	}
	hotS, err := s.tsLogger.GetPgssSummary(ctx, instanceName, from, to)
	if err != nil {
		return nil, err
	}
	return &models.PgssSummaryResponse{
		Instance:           instanceName,
		QueryLoadMsSec:     hotS.QueryLoadMsSec,
		QPS:                hotS.QPS,
		P99Ms:              hotS.P99Ms,
		CacheHitPct:        hotS.CacheHitPct,
		TempMbSec:          hotS.TempMbSec,
		WalMbSec:           hotS.WalMbSec,
		CpuSaturationMsSec: hotS.CpuSaturationMsSec,
	}, nil
}
