// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: SQL Server user-watched queries API handlers.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/service"
	"github.com/rsharma155/sql_optima/internal/storage/hot"
	"github.com/rsharma155/sqlplan-analyzer"
)

type WatchedQueryHandlers struct {
	metricsSvc *service.MetricsService
	cfg        *config.Config
}

func NewWatchedQueryHandlers(svc *service.MetricsService, cfg *config.Config) *WatchedQueryHandlers {
	return &WatchedQueryHandlers{metricsSvc: svc, cfg: cfg}
}

func (h *WatchedQueryHandlers) parseID(r *http.Request) (uuid.UUID, bool) {
	id, _, outcome := resolveInstanceParam(r, h.cfg)
	if outcome != lookupFound {
		return uuid.Nil, false
	}
	return id, true
}

func (h *WatchedQueryHandlers) resolve(w http.ResponseWriter, r *http.Request) (uuid.UUID, string, bool) {
	id, name, outcome := resolveInstanceParam(r, h.cfg)
	switch outcome {
	case lookupMissing:
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "instance name or server_id required"})
		return uuid.Nil, "", false
	case lookupNotFound:
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return uuid.Nil, "", false
	}
	return id, name, true
}

func (h *WatchedQueryHandlers) ListQueries(w http.ResponseWriter, r *http.Request) {
	id, instanceName, ok := h.resolve(w, r)
	if !ok {
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if h.metricsSvc.GetTimescaleDBLogger() == nil {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"instance":        instanceName,
			"watched_queries": []hot.SqlServerWatchedQueryRow{},
		})
		return
	}

	res, err := h.metricsSvc.ListSqlServerWatchedQueries(r.Context(), id)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if res == nil {
		res = []hot.SqlServerWatchedQueryRow{}
	}
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"instance":        instanceName,
		"watched_queries": res,
	})
}

func (h *WatchedQueryHandlers) GetDetail(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.URL.Query().Get("id"))
	serverID, ok := h.parseID(r)
	if !ok || id == 0 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	wq, err := h.metricsSvc.GetSqlServerWatchedQuery(r.Context(), id)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	snapshots, _ := h.metricsSvc.ListWatchedQuerySnapshots(r.Context(), serverID, id)
	events, _ := h.metricsSvc.ListWatchedQueryEvents(r.Context(), id)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"watched_query": wq,
		"snapshots":     snapshots,
		"events":        events,
	})
}

func (h *WatchedQueryHandlers) GetPlanAnalysis(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.URL.Query().Get("id"))
	snapTime := r.URL.Query().Get("snapshot_time")

	snap, err := h.metricsSvc.GetWatchedQuerySnapshot(r.Context(), id, snapTime)
	if err != nil || snap.QueryPlan == "" {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "plan XML not found for this snapshot"})
		return
	}

	analysis, err := sqlplan.AnalyzeBytes([]byte(snap.QueryPlan))
	if err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		return
	}
	htmlReport, _ := sqlplan.GenerateHTML(analysis)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":      true,
		"html_report":  htmlReport,
		"health_score": analysis.HealthScore.OverallScore,
	})
}

func (h *WatchedQueryHandlers) AddQuery(w http.ResponseWriter, r *http.Request) {
	id, _, ok := h.resolve(w, r)
	if !ok {
		return
	}

	var req hot.SqlServerWatchedQueryRow
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if req.QueryHash == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "query_hash required"})
		return
	}
	req.ServerID = id

	newID, err := h.metricsSvc.AddSqlServerWatchedQuery(r.Context(), id, req)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]int{"id": newID})
}

func (h *WatchedQueryHandlers) DeleteQuery(w http.ResponseWriter, r *http.Request) {
	qidStr := r.URL.Query().Get("id")
	qid, err := strconv.Atoi(qidStr)
	if err != nil || qid == 0 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "valid id required"})
		return
	}

	if err := h.metricsSvc.DeleteSqlServerWatchedQuery(r.Context(), qid); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func parseTimeRange(fromStr, toStr string) (time.Time, time.Time, error) {
	from, _ := time.Parse(time.RFC3339, fromStr)
	to, _ := time.Parse(time.RFC3339, toStr)
	if to.IsZero() {
		to = time.Now().UTC()
	}
	if from.IsZero() {
		from = to.Add(-24 * time.Hour)
	}
	return from, to, nil
}
