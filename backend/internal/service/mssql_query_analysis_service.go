// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Service-layer methods for the SQL Server Query Analysis Dashboard,
//
//	orchestrating between the MSSQL repository, TimescaleDB storage,
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

// GetMssqlQueryAnalysisSummary returns KPI card data for the query analysis dashboard.
func (s *MetricsService) GetMssqlQueryAnalysisSummary(ctx context.Context, instance string, hours int) (*models.MssqlQueryAnalysisSummary, error) {
	if s.tsLogger == nil {
		return &models.MssqlQueryAnalysisSummary{}, nil
	}
	row, err := s.tsLogger.GetMssqlQueryAnalysisSummary(ctx, instance, hours)
	if err != nil {
		return nil, err
	}
	return &models.MssqlQueryAnalysisSummary{
		TotalExecutions: row.TotalExecutions,
		AvgDuration:     row.AvgDuration,
		AvgCPU:          row.AvgCPU,
		AvgReads:        row.AvgReads,
		Regressions24h:  row.Regressions24h,
		PlanChanges24h:  row.PlanChanges24h,
	}, nil
}

// ────────────────────────────────────────────────
// Regressions
// ────────────────────────────────────────────────

// GetMssqlQueryRegressions returns recent regression rows from TimescaleDB.
func (s *MetricsService) GetMssqlQueryRegressions(ctx context.Context, instance string, limit int) ([]models.MssqlQueryRegression, error) {
	if s.tsLogger == nil {
		return nil, nil
	}
	rows, err := s.tsLogger.GetMssqlQueryRegressions(ctx, instance, limit)
	if err != nil {
		return nil, err
	}
	out := make([]models.MssqlQueryRegression, len(rows))
	for i, r := range rows {
		out[i] = models.MssqlQueryRegression{
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

// GetMssqlPlanInstability returns recent plan instability rows from TimescaleDB.
func (s *MetricsService) GetMssqlPlanInstability(ctx context.Context, instance string, limit int) ([]models.MssqlPlanInstability, error) {
	if s.tsLogger == nil {
		return nil, nil
	}
	rows, err := s.tsLogger.GetMssqlPlanInstability(ctx, instance, limit)
	if err != nil {
		return nil, err
	}
	out := make([]models.MssqlPlanInstability, len(rows))
	for i, r := range rows {
		out[i] = models.MssqlPlanInstability{
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

// GetMssqlTopQueriesAnalysis returns top queries from the existing interval table.
func (s *MetricsService) GetMssqlTopQueriesAnalysis(ctx context.Context, instance, sortBy string, limit, hours int) ([]models.MssqlTopQueryRow, error) {
	if s.tsLogger == nil {
		return nil, nil
	}
	rows, err := s.tsLogger.GetMssqlTopQueriesFromInterval(ctx, instance, sortBy, limit, hours)
	if err != nil {
		return nil, err
	}
	out := make([]models.MssqlTopQueryRow, len(rows))
	for i, r := range rows {
		out[i] = models.MssqlTopQueryRow{
			QueryHash:     r.QueryHash,
			QueryText:     r.QueryText,
			DatabaseName:  r.DatabaseName,
			Executions:    r.Executions,
			AvgCpuMs:      r.AvgCpuMs,
			AvgDurationMs: r.AvgDurationMs,
			AvgReads:      r.AvgReads,
			TotalCpuMs:    r.TotalCpuMs,
		}
	}
	return out, nil
}

// ────────────────────────────────────────────────
// Watched Queries CRUD
// ────────────────────────────────────────────────

// ListMssqlWatchedQueries returns all watched queries for an instance.
func (s *MetricsService) ListMssqlWatchedQueries(ctx context.Context, instance string) ([]models.MssqlWatchedQuery, error) {
	if s.tsLogger == nil {
		return nil, nil
	}
	rows, err := s.tsLogger.ListMssqlWatchedQueries(ctx, instance)
	if err != nil {
		return nil, err
	}
	out := make([]models.MssqlWatchedQuery, len(rows))
	for i, r := range rows {
		out[i] = models.MssqlWatchedQuery{
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

// AddMssqlWatchedQuery adds a query to the watch list (max 10 per instance, enforced here).
func (s *MetricsService) AddMssqlWatchedQuery(ctx context.Context, instance string, wq models.MssqlWatchedQuery) (int, error) {
	if s.tsLogger == nil {
		return 0, nil
	}
	count, err := s.tsLogger.CountMssqlWatchedQueries(ctx, instance)
	if err != nil {
		return 0, err
	}
	if count >= 10 {
		return 0, fmt.Errorf("maximum of 10 watched queries per instance reached")
	}
	row := hot.MssqlWatchedQueryRow{
		InstanceName: instance,
		DatabaseName: wq.DatabaseName,
		QueryHash:    wq.QueryHash,
		Name:         wq.Name,
		QueryText:    wq.QueryText,
	}
	return s.tsLogger.InsertMssqlWatchedQuery(ctx, row)
}

// DeleteMssqlWatchedQuery removes a watched query by ID.
func (s *MetricsService) DeleteMssqlWatchedQuery(ctx context.Context, id int) error {
	if s.tsLogger == nil {
		return nil
	}
	return s.tsLogger.DeleteMssqlWatchedQuery(ctx, id)
}

// GetMssqlWatchedQueryDetail returns a single watched query with its time-series snapshots and events.
func (s *MetricsService) GetMssqlWatchedQueryDetail(ctx context.Context, id int, from, to time.Time) (*models.MssqlWatchedQuery, []models.MssqlWatchedQuerySnapshot, []models.MssqlWatchedQueryEvent, error) {
	if s.tsLogger == nil {
		return nil, nil, nil, nil
	}
	wqRow, err := s.tsLogger.GetMssqlWatchedQuery(ctx, id)
	if err != nil {
		return nil, nil, nil, err
	}

	wq := &models.MssqlWatchedQuery{
		ID: wqRow.ID, InstanceName: wqRow.InstanceName, DatabaseName: wqRow.DatabaseName,
		QueryHash: wqRow.QueryHash, Name: wqRow.Name, QueryText: wqRow.QueryText,
		CreatedAt: wqRow.CreatedAt, LastExecutedAt: wqRow.LastExecutedAt.Time,
	}
	if wqRow.ObjectID.Valid {
		wq.ObjectID = int(wqRow.ObjectID.Int32)
	}

	snapRows, err := s.tsLogger.GetMssqlWatchedQuerySnapshots(ctx, id, from, to)
	if err != nil {
		log.Printf("[MssqlQueryAnalysis] GetMssqlWatchedQuerySnapshots(%d): %v", id, err)
		snapRows = nil
	}
	snaps := make([]models.MssqlWatchedQuerySnapshot, len(snapRows))
	for i, r := range snapRows {
		s := models.MssqlWatchedQuerySnapshot{
			SnapshotTime: r.SnapshotTime, WatchedID: r.WatchedID,
			Executions: r.Executions, AvgDurationMs: r.AvgDurationMs,
			AvgCpuMs: r.AvgCpuMs, AvgReads: r.AvgReads,
			TotalDurationMs: r.TotalDurationMs, TotalCpuMs: r.TotalCpuMs,
			PlanCount: r.PlanCount, LastExecutionTime: r.LastExecutionTime,
			QueryPlan: r.QueryPlan,
		}

		if r.WaitStats != nil {
			if ws, ok := r.WaitStats.([]models.MssqlQueryWaitStat); ok {
				s.WaitStats = ws
			} else if data, err := json.Marshal(r.WaitStats); err == nil {
				// Fallback: if scanned as raw map/slice, try to unmarshal into model
				var wsArr []models.MssqlQueryWaitStat
				if err := json.Unmarshal(data, &wsArr); err == nil {
					s.WaitStats = wsArr
				}
			}
		}
		snaps[i] = s
	}

	evtRows, err := s.tsLogger.GetMssqlWatchedQueryEvents(ctx, id)
	if err != nil {
		log.Printf("[MssqlQueryAnalysis] GetMssqlWatchedQueryEvents(%d): %v", id, err)
		evtRows = nil
	}
	evts := make([]models.MssqlWatchedQueryEvent, len(evtRows))
	for i, r := range evtRows {
		evts[i] = models.MssqlWatchedQueryEvent{
			ID: r.ID, WatchedID: r.WatchedID, EventTime: r.EventTime,
			EventType: r.EventType, Notes: r.Notes,
		}
	}

	return wq, snaps, evts, nil
}

// AddMssqlWatchedQueryEvent adds an optimization event marker.
func (s *MetricsService) AddMssqlWatchedQueryEvent(ctx context.Context, watchedID int, eventType, notes string) error {
	if s.tsLogger == nil {
		return nil
	}
	return s.tsLogger.InsertMssqlWatchedQueryEvent(ctx, hot.MssqlWatchedEventRow{
		WatchedID: watchedID,
		EventTime: time.Now().UTC(),
		EventType: eventType,
		Notes:     notes,
	})
}

// ────────────────────────────────────────────────
// Query Plans & Wait Stats (live DMV queries)
// ────────────────────────────────────────────────

// GetMssqlQueryPlans returns plan metadata from Query Store for a specific query hash.
func (s *MetricsService) GetMssqlQueryPlans(ctx context.Context, instance, dbName, queryHash string) ([]models.MssqlQueryPlanInfo, error) {
	if s.MsRepo == nil {
		return nil, nil
	}
	return s.MsRepo.FetchQueryPlans(ctx, instance, dbName, queryHash)
}

// GetMssqlQueryWaitStats returns wait stats from Query Store for a specific query hash.
func (s *MetricsService) GetMssqlQueryWaitStats(ctx context.Context, instance, dbName, queryHash string) ([]models.MssqlQueryWaitStat, error) {
	if s.MsRepo == nil {
		return nil, nil
	}
	return s.MsRepo.FetchQueryWaitStats(ctx, instance, dbName, queryHash)
}
