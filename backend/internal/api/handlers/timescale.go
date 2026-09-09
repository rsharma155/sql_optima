// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: TimescaleDB-specific metrics retrieval API handlers.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"log/slog"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/rsharma155/sql_optima/internal/domain"
	"github.com/rsharma155/sql_optima/internal/service"
)

type TimescaleHandlers struct {
	metricsSvc *service.MetricsService
	coldQuery  *ColdQueryHandlers
}

func NewTimescaleHandlers(svc *service.MetricsService, coldQuery ...*ColdQueryHandlers) *TimescaleHandlers {
	h := &TimescaleHandlers{metricsSvc: svc}
	if len(coldQuery) > 0 {
		h.coldQuery = coldQuery[0]
	}
	return h
}

func (h *TimescaleHandlers) Status(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(map[string]bool{"connected": h.metricsSvc.IsTimescaleConnected()})
}

func (h *TimescaleHandlers) GetSQLServerMetrics(w http.ResponseWriter, r *http.Request) {
	idStr := mux.Vars(r)["id"]
	id, err := uuid.Parse(idStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")
	if fromStr != "" && toStr != "" {
		from, _ := time.Parse(time.RFC3339, fromStr)
		to, _ := time.Parse(time.RFC3339, toStr)
		res, err := h.metricsSvc.GetTimescaleSQLServerMetricsRange(r.Context(), id, from, to)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(res)
		return
	}

	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if val, err := strconv.Atoi(l); err == nil {
			limit = val
		}
	}

	res, err := h.metricsSvc.GetTimescaleSQLServerMetrics(r.Context(), id, limit)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (h *TimescaleHandlers) SqlServerMemoryDrilldown(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	empty := map[string]interface{}{
		"memory_metrics":    []interface{}{},
		"scheduler_memory":  []interface{}{},
		"buffer_pool_by_db": []interface{}{},
		"memory_clerks":     []interface{}{},
	}

	serverID, ok := ParseServerID(r, h.metricsSvc.Config)
	if !ok {
		_ = json.NewEncoder(w).Encode(empty)
		return
	}

	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")
	fromT, toT, _ := parseTimeRange(fromStr, toStr)

	metrics, _ := h.metricsSvc.GetSqlServerMemoryDrilldown(r.Context(), serverID, fromStr, toStr)
	if metrics == nil {
		metrics = []map[string]interface{}{}
	}

	clerks, _ := h.metricsSvc.GetSqlServerMemoryClerksTimeSeries(r.Context(), serverID, fromT, toT)
	bpdb, _ := h.metricsSvc.GetSQLServerBufferPoolByDB(r.Context(), serverID, fromStr, toStr, 200)
	if bpdb == nil {
		bpdb = []map[string]interface{}{}
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"memory_metrics":    metrics,
		"scheduler_memory":  []interface{}{},
		"buffer_pool_by_db": bpdb,
		"memory_clerks":     clerks,
	})
}

func (h *TimescaleHandlers) SqlServerTopQueries(w http.ResponseWriter, r *http.Request) {
	// Implementation placeholder
	_ = json.NewEncoder(w).Encode([]interface{}{})
}

func (h *TimescaleHandlers) SqlServerQueryStatsDashboard(w http.ResponseWriter, r *http.Request) {
	// Implementation placeholder
	_ = json.NewEncoder(w).Encode(map[string]interface{}{})
}

func (h *TimescaleHandlers) SqlServerQueryStatsTimeSeries(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	serverID, ok := ParseServerID(r, h.metricsSvc.Config)
	if !ok {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "instance name or server_id required"})
		return
	}

	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	metric := r.URL.Query().Get("metric") // cpu, duration, reads, executions
	dbName := strings.TrimSpace(r.URL.Query().Get("database"))
	if strings.EqualFold(dbName, "all") {
		dbName = ""
	}
	excludeSystem := true
	if es := r.URL.Query().Get("exclude_system"); es == "false" {
		excludeSystem = false
	}

	tFrom, tTo, _ := parseTimeRange(from, to)
	if dbName == "" {
		qa := &SqlServerQueryAnalysisHandlers{metricsSvc: h.metricsSvc, cfg: h.metricsSvc.Config}
		partial := domain.WorkloadQueryFilter{
			ExcludeSystem:    excludeSystem,
			MonitoringLogins: sqlServerExcludeLoginsForInstance(h.metricsSvc.Config, strings.TrimSpace(r.URL.Query().Get("instance"))),
		}
		if resolved, err := qa.resolveAnalysisDatabase(r.Context(), serverID, tFrom, tTo, "", true, partial); err == nil {
			dbName = resolved
		}
	}
	logins := sqlServerExcludeLoginsForInstance(h.metricsSvc.Config, strings.TrimSpace(r.URL.Query().Get("instance")))

	series, err := h.metricsSvc.GetSqlServerQueryStatsTimeSeries(r.Context(), serverID, metric, from, to, dbName, excludeSystem, logins)
	if err != nil {
		slog.Error("[Timescale] GetSqlServerQueryStatsTimeSeries error", "err", err)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"series": []interface{}{}, "error": "failed to load query stats series"})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"series":   series,
		"database": dbName,
	})
}

func (h *TimescaleHandlers) SqlServerQueryStatsTrend(w http.ResponseWriter, r *http.Request) {
	idStr := mux.Vars(r)["id"]
	id, err := uuid.Parse(idStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	sqlHash := r.URL.Query().Get("sql_hash")
	from, to, _ := parseTimeRange(r.URL.Query().Get("from"), r.URL.Query().Get("to"))

	res, err := h.metricsSvc.GetSQLServerQueryTrend(r.Context(), id, sqlHash, from, to)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(res)
}

func (h *TimescaleHandlers) SqlServerLongRunningQueries(w http.ResponseWriter, r *http.Request) {
	// Implementation placeholder
	_ = json.NewEncoder(w).Encode([]interface{}{})
}

func (h *TimescaleHandlers) GetPostgresThroughput(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode([]interface{}{})
}

func (h *TimescaleHandlers) PostgresConnections(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode([]interface{}{})
}
