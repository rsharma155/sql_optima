// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Service-layer methods for the SQL Server Query Analysis Dashboard,
//
//	orchestrating between the SQLSERVER repository, TimescaleDB storage,
//	and HTTP handlers.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/rsharma155/sql_optima/internal/models"
	"github.com/rsharma155/sql_optima/internal/storage/hot"
)

// ────────────────────────────────────────────────
// Query Analysis Summary
// ────────────────────────────────────────────────

// GetSqlServerQueryAnalysisSummary returns KPI card data for the query analysis dashboard.
func (s *MetricsService) GetSqlServerQueryAnalysisSummary(ctx context.Context, instance string, hours int, excludeSystem bool) (*models.SqlServerQueryAnalysisSummary, error) {
	if s.tsLogger == nil {
		return &models.SqlServerQueryAnalysisSummary{}, nil
	}
	row, err := s.tsLogger.GetSqlServerQueryAnalysisSummary(ctx, instance, hours, excludeSystem)
	if err != nil {
		return nil, err
	}
	return &models.SqlServerQueryAnalysisSummary{
		TotalExecutions:        row.TotalExecutions,
		AvgDuration:            row.AvgDuration,
		AvgCPU:                 row.AvgCPU,
		AvgReads:               row.AvgReads,
		Regressions24h:         row.Regressions24h,
		PlanChanges24h:         row.PlanChanges24h,
		Top10CpuSharePct:       row.Top10CpuSharePct,
		TotalQueriesInQS:       row.TotalQueriesInQS,
		QueriesExecutedInRange: row.QueriesExecutedInRange,
		QueriesWithMultiPlans:  row.QueriesWithMultiPlans,
		QueriesSingleExecution: row.QueriesSingleExecution,
	}, nil
}

// ────────────────────────────────────────────────
// Regressions
// ────────────────────────────────────────────────

// GetSqlServerQueryRegressions returns recent regression rows from TimescaleDB.
func (s *MetricsService) GetSqlServerQueryRegressions(ctx context.Context, instance string, limit int) ([]models.SqlServerQueryRegression, error) {
	if s.tsLogger == nil {
		return nil, nil
	}
	rows, err := s.tsLogger.GetSqlServerQueryRegressions(ctx, instance, limit)
	if err != nil {
		return nil, err
	}
	out := make([]models.SqlServerQueryRegression, len(rows))
	for i, r := range rows {
		out[i] = models.SqlServerQueryRegression{
			CaptureTime:    r.CaptureTime,
			InstanceName:   r.InstanceName,
			DatabaseName:   r.DatabaseName,
			QueryHash:      r.QueryHash,
			QueryText:      r.QueryText,
			RegressionType: r.RegressionType,
			PreviousAvg:    r.PreviousAvg,
			CurrentAvg:     r.CurrentAvg,
			PercentChange:  r.PercentChange,
			PlanChanged:    r.PlanChanged,
		}
	}
	return out, nil
}

// ────────────────────────────────────────────────
// Plan Instability
// ────────────────────────────────────────────────

// GetSqlServerPlanInstability returns recent plan instability rows from TimescaleDB.
func (s *MetricsService) GetSqlServerPlanInstability(ctx context.Context, instance string, limit int) ([]models.SqlServerPlanInstability, error) {
	if s.tsLogger == nil {
		return nil, nil
	}
	rows, err := s.tsLogger.GetSqlServerPlanInstability(ctx, instance, limit)
	if err != nil {
		return nil, err
	}
	out := make([]models.SqlServerPlanInstability, len(rows))
	for i, r := range rows {
		out[i] = models.SqlServerPlanInstability{
			CaptureTime:       r.CaptureTime,
			InstanceName:      r.InstanceName,
			DatabaseName:      r.DatabaseName,
			QueryHash:         r.QueryHash,
			QueryText:         r.QueryText,
			PlanCount:         r.PlanCount,
			LastExecutionTime: r.LastExecutionTime,
		}
	}
	return out, nil
}

// ────────────────────────────────────────────────
// Top Queries (from existing interval table)
// ────────────────────────────────────────────────

// GetSqlServerTopQueriesAnalysis returns top queries from the existing interval table.
func (s *MetricsService) GetSqlServerTopQueriesAnalysis(ctx context.Context, instance, sortBy string, limit, hours int, excludeSystem bool) ([]models.SqlServerTopQueryRow, error) {
	if s.tsLogger == nil {
		return nil, nil
	}
	rows, err := s.tsLogger.GetSqlServerTopQueriesFromInterval(ctx, instance, sortBy, limit, hours, excludeSystem)
	if err != nil {
		return nil, err
	}
	out := make([]models.SqlServerTopQueryRow, len(rows))
	for i, r := range rows {
		row := models.SqlServerTopQueryRow{
			QueryHash:         r.QueryHash,
			QueryText:         r.QueryText,
			DatabaseName:      r.DatabaseName,
			LoginName:         r.LoginName,
			ApplicationName:   r.ApplicationName,
			Executions:        r.Executions,
			AvgCpuMs:          r.AvgCpuMs,
			AvgDurationMs:     r.AvgDurationMs,
			AvgReads:          r.AvgReads,
			TotalCpuMs:        r.TotalCpuMs,
		}
		if r.LastExecutionTime != nil {
			row.LastExecutionTime = *r.LastExecutionTime
		}
		out[i] = row
	}
	return out, nil
}

// ────────────────────────────────────────────────
// Watched Queries CRUD
// ────────────────────────────────────────────────

// ListSqlServerWatchedQueries returns all watched queries for an instance.
func (s *MetricsService) ListSqlServerWatchedQueries(ctx context.Context, instance string) ([]models.SqlServerWatchedQuery, error) {
	if s.tsLogger == nil {
		return nil, nil
	}
	rows, err := s.tsLogger.ListSqlServerWatchedQueries(ctx, instance)
	if err != nil {
		return nil, err
	}
	out := make([]models.SqlServerWatchedQuery, len(rows))
	for i, r := range rows {
		out[i] = models.SqlServerWatchedQuery{
			ID: r.ID, InstanceName: r.InstanceName, DatabaseName: r.DatabaseName,
			QueryHash: r.QueryHash, Name: r.Name, QueryText: r.QueryText,
			CreatedAt: r.CreatedAt, LastExecutedAt: r.LastExecutedAt.Time,
		}
		if r.ObjectID.Valid {
			out[i].ObjectID = int(r.ObjectID.Int32)
		}
	}
	return out, nil
}

// AddSqlServerWatchedQuery adds a query to the watch list (max 10 per instance, enforced here).
func (s *MetricsService) AddSqlServerWatchedQuery(ctx context.Context, instance string, wq models.SqlServerWatchedQuery) (int, error) {
	if s.tsLogger == nil {
		return 0, nil
	}
	count, err := s.tsLogger.CountSqlServerWatchedQueries(ctx, instance)
	if err != nil {
		return 0, err
	}
	if count >= 10 {
		return 0, fmt.Errorf("maximum of 10 watched queries per instance reached")
	}
	row := hot.SqlServerWatchedQueryRow{
		InstanceName: instance,
		DatabaseName: wq.DatabaseName,
		QueryHash:    wq.QueryHash,
		Name:         wq.Name,
		QueryText:    wq.QueryText,
	}
	id, err := s.tsLogger.InsertSqlServerWatchedQuery(ctx, row)
	if err != nil {
		return 0, err
	}

	// Capture initial plan/stats immediately so they are available without waiting for the next collector run
	go func() {
		// Create a background context for this async capture
		bgCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		snap, err := s.MsRepo.FetchWatchedQueryStats(bgCtx, instance, wq.DatabaseName, wq.QueryHash)
		if err != nil {
			log.Printf("[SqlServerQueryAnalysis] Immediate capture failed for %s: %v", wq.QueryHash, err)
			return
		}
		if snap == nil {
			log.Printf("[SqlServerQueryAnalysis] Immediate capture: no stats found in Query Store for %s", wq.QueryHash)
			return
		}

		// Update query text if it was missing
		if wq.QueryText == "" && snap.QueryText != "" {
			_ = s.tsLogger.UpdateSqlServerWatchedQueryText(bgCtx, id, snap.QueryText)
		}

		// Fetch wait stats
		waitStats, _ := s.MsRepo.FetchQueryWaitStats(bgCtx, instance, wq.DatabaseName, wq.QueryHash)

		// Log snapshot
		snapRows := []hot.SqlServerWatchedSnapshotRow{{
			SnapshotTime:      time.Now().UTC(),
			WatchedID:         id,
			InstanceName:      instance,
			Executions:        snap.Executions,
			AvgDurationMs:     snap.AvgDurationMs,
			AvgCpuMs:          snap.AvgCpuMs,
			AvgReads:          snap.AvgReads,
			TotalDurationMs:   snap.TotalDurationMs,
			TotalCpuMs:        snap.TotalCpuMs,
			PlanCount:         snap.PlanCount,
			LastExecutionTime: snap.LastExecutionTime,
			QueryPlan:         snap.QueryPlan,
			WaitStats:         waitStats,
		}}
		if err := s.tsLogger.LogSqlServerWatchedQuerySnapshot(bgCtx, snapRows); err != nil {
			log.Printf("[SqlServerQueryAnalysis] Immediate LogSqlServerWatchedQuerySnapshot failed: %v", err)
		} else {
			log.Printf("[SqlServerQueryAnalysis] Successfully captured initial plan and stats for watched query ID %d", id)
		}
	}()

	return id, nil
}

// DeleteSqlServerWatchedQuery removes a watched query by ID.
func (s *MetricsService) DeleteSqlServerWatchedQuery(ctx context.Context, id int) error {
	if s.tsLogger == nil {
		return nil
	}
	return s.tsLogger.DeleteSqlServerWatchedQuery(ctx, id)
}

// GetSqlServerWatchedQueryDetail returns a single watched query with its time-series snapshots and events.
func (s *MetricsService) GetSqlServerWatchedQueryDetail(ctx context.Context, id int, from, to time.Time) (*models.SqlServerWatchedQuery, []models.SqlServerWatchedQuerySnapshot, []models.SqlServerWatchedQueryEvent, error) {
	if s.tsLogger == nil {
		return nil, nil, nil, nil
	}
	wqRow, err := s.tsLogger.GetSqlServerWatchedQuery(ctx, id)
	if err != nil {
		return nil, nil, nil, err
	}

	wq := &models.SqlServerWatchedQuery{
		ID: wqRow.ID, InstanceName: wqRow.InstanceName, DatabaseName: wqRow.DatabaseName,
		QueryHash: wqRow.QueryHash, Name: wqRow.Name, QueryText: wqRow.QueryText,
		CreatedAt: wqRow.CreatedAt, LastExecutedAt: wqRow.LastExecutedAt.Time,
	}
	if wqRow.ObjectID.Valid {
		wq.ObjectID = int(wqRow.ObjectID.Int32)
	}

	snapRows, err := s.tsLogger.GetSqlServerWatchedQuerySnapshots(ctx, id, from, to)
	if err != nil {
		log.Printf("[SqlServerQueryAnalysis] GetSqlServerWatchedQuerySnapshots(%d): %v", id, err)
		snapRows = nil
	}
	snaps := make([]models.SqlServerWatchedQuerySnapshot, len(snapRows))
	for i, r := range snapRows {
		s := models.SqlServerWatchedQuerySnapshot{
			SnapshotTime: r.SnapshotTime, WatchedID: r.WatchedID,
			Executions: r.Executions, AvgDurationMs: r.AvgDurationMs,
			AvgCpuMs: r.AvgCpuMs, AvgReads: r.AvgReads,
			TotalDurationMs: r.TotalDurationMs, TotalCpuMs: r.TotalCpuMs,
			PlanCount: r.PlanCount, LastExecutionTime: r.LastExecutionTime,
			QueryPlan: r.QueryPlan,
		}

		if r.WaitStats != nil {
			if ws, ok := r.WaitStats.([]models.SqlServerQueryWaitStat); ok {
				s.WaitStats = ws
			} else if data, err := json.Marshal(r.WaitStats); err == nil {
				// Fallback: if scanned as raw map/slice, try to unmarshal into model
				var wsArr []models.SqlServerQueryWaitStat
				if err := json.Unmarshal(data, &wsArr); err == nil {
					s.WaitStats = wsArr
				}
			}
		}
		snaps[i] = s
	}

	evtRows, err := s.tsLogger.GetSqlServerWatchedQueryEvents(ctx, id)
	if err != nil {
		log.Printf("[SqlServerQueryAnalysis] GetSqlServerWatchedQueryEvents(%d): %v", id, err)
		evtRows = nil
	}
	evts := make([]models.SqlServerWatchedQueryEvent, len(evtRows))
	for i, r := range evtRows {
		evts[i] = models.SqlServerWatchedQueryEvent{
			ID: r.ID, WatchedID: r.WatchedID, EventTime: r.EventTime,
			EventType: r.EventType, Notes: r.Notes,
		}
	}

	return wq, snaps, evts, nil
}

// AddSqlServerWatchedQueryEvent adds an optimization event marker.
func (s *MetricsService) AddSqlServerWatchedQueryEvent(ctx context.Context, watchedID int, eventType, notes string) error {
	if s.tsLogger == nil {
		return nil
	}
	return s.tsLogger.InsertSqlServerWatchedQueryEvent(ctx, hot.SqlServerWatchedEventRow{
		WatchedID: watchedID,
		EventTime: time.Now().UTC(),
		EventType: eventType,
		Notes:     notes,
	})
}

// ────────────────────────────────────────────────
// Query Plans & Wait Stats (live DMV queries)
// ────────────────────────────────────────────────

// GetSqlServerQueryPlans returns plan metadata from Query Store for a specific query hash.
func (s *MetricsService) GetSqlServerQueryPlans(ctx context.Context, instance, dbName, queryHash string) ([]models.SqlServerQueryPlanInfo, error) {
	if s.MsRepo == nil {
		return nil, nil
	}
	return s.MsRepo.FetchQueryPlans(ctx, instance, dbName, queryHash)
}

// GetSqlServerQueryWaitStats returns wait stats from Query Store for a specific query hash.
func (s *MetricsService) GetSqlServerQueryWaitStats(ctx context.Context, instance, dbName, queryHash string) ([]models.SqlServerQueryWaitStat, error) {
	if s.MsRepo == nil {
		return nil, nil
	}
	return s.MsRepo.FetchQueryWaitStats(ctx, instance, dbName, queryHash)
}
