// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: SQL Server workload and throughput API handlers.
//          All reads are from TimescaleDB (collector snapshots); no live DMV queries.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/domain"
	"github.com/rsharma155/sql_optima/internal/service"
)

type SqlServerWorkloadHandlers struct {
	metricsSvc *service.MetricsService
	cfg        *config.Config
}

func NewSqlServerWorkloadHandlers(svc *service.MetricsService, cfg *config.Config) *SqlServerWorkloadHandlers {
	return &SqlServerWorkloadHandlers{metricsSvc: svc, cfg: cfg}
}

func (h *SqlServerWorkloadHandlers) parseID(r *http.Request) (uuid.UUID, bool) {
	id, _, outcome := resolveInstanceParamWithRepo(r.Context(), r, h.cfg, h.metricsSvc)
	switch outcome {
	case lookupFound:
		return id, true
	default:
		return uuid.Nil, false
	}
}

func parseWorkloadFilterParams(r *http.Request) (database string, autoPickDatabase bool, excludeSystem bool) {
	database, autoPickDatabase = ParseSQLServerDatabaseScope(r)
	excludeSystem = true
	if es := r.URL.Query().Get("exclude_system"); es == "false" {
		excludeSystem = false
	}
	return database, autoPickDatabase, excludeSystem
}

func (h *SqlServerWorkloadHandlers) resolveWorkloadDatabase(ctx context.Context, serverID uuid.UUID, from, to time.Time, database string, autoPick bool, partial domain.WorkloadQueryFilter) (string, error) {
	database = strings.TrimSpace(database)
	if !autoPick {
		return database, nil
	}
	if h.metricsSvc != nil {
		if db, err := h.metricsSvc.GetSqlServerPrimaryWorkloadDatabase(ctx, serverID, from, to, partial); err == nil && db != "" {
			return db, nil
		}
	}
	return "", nil
}

func (h *SqlServerWorkloadHandlers) instanceNameFromRequest(r *http.Request) string {
	return strings.TrimSpace(r.URL.Query().Get("instance"))
}

func (h *SqlServerWorkloadHandlers) workloadFilter(ctx context.Context, r *http.Request, serverID uuid.UUID, from, to time.Time) (domain.WorkloadQueryFilter, error) {
	database, autoPick, excludeSystem := parseWorkloadFilterParams(r)
	partial := domain.WorkloadQueryFilter{
		ExcludeSystem:    excludeSystem,
		MonitoringLogins: sqlServerExcludeLoginsForInstance(h.cfg, h.instanceNameFromRequest(r)),
	}
	db, err := h.resolveWorkloadDatabase(ctx, serverID, from, to, database, autoPick, partial)
	if err != nil {
		return domain.WorkloadQueryFilter{}, err
	}
	partial.Database = db
	return partial, nil
}

func (h *SqlServerWorkloadHandlers) GetDatabases(w http.ResponseWriter, r *http.Request) {
	id, ok := h.parseID(r)
	if !ok {
		writeWorkloadTimescaleJSON(w, http.StatusBadRequest, map[string]string{"error": "instance name or server_id required"})
		return
	}
	from, to := ParseTimeRange(r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	selectedDB, _, excludeSystem := parseWorkloadFilterParams(r)
	partial := domain.WorkloadQueryFilter{
		ExcludeSystem:    excludeSystem,
		MonitoringLogins: sqlServerExcludeLoginsForInstance(h.cfg, h.instanceNameFromRequest(r)),
	}
	list, err := h.metricsSvc.GetSqlServerDatabasesInRange(r.Context(), id, from, to, partial)
	if err != nil {
		writeWorkloadTimescaleJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list databases"})
		return
	}
	scope := selectedDB
	if scope == "" {
		scope = "all"
	}
	writeWorkloadTimescaleJSON(w, http.StatusOK, map[string]interface{}{
		"server_id":      id.String(),
		"selected_scope": scope,
		"exclude_system": excludeSystem,
		"databases":      list,
	})
}

func (h *SqlServerWorkloadHandlers) GetDefaultDatabase(w http.ResponseWriter, r *http.Request) {
	id, ok := h.parseID(r)
	if !ok {
		writeWorkloadTimescaleJSON(w, http.StatusBadRequest, map[string]string{"error": "instance name or server_id required"})
		return
	}
	from, to := ParseTimeRange(r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	_, _, excludeSystem := parseWorkloadFilterParams(r)
	partial := domain.WorkloadQueryFilter{
		ExcludeSystem:    excludeSystem,
		MonitoringLogins: sqlServerExcludeLoginsForInstance(h.cfg, h.instanceNameFromRequest(r)),
	}
	db, err := h.resolveWorkloadDatabase(r.Context(), id, from, to, "", true, partial)
	if err != nil {
		writeWorkloadTimescaleJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to resolve default database"})
		return
	}
	writeWorkloadTimescaleJSON(w, http.StatusOK, map[string]string{"database": db})
}

func writeWorkloadTimescaleJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Data-Source", "timescale")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (h *SqlServerWorkloadHandlers) GetSummary(w http.ResponseWriter, r *http.Request) {
	id, ok := h.parseID(r)
	if !ok {
		writeWorkloadTimescaleJSON(w, http.StatusBadRequest, map[string]string{"error": "instance name or server_id required"})
		return
	}

	from, to := ParseTimeRange(r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	filter, err := h.workloadFilter(r.Context(), r, id, from, to)
	if err != nil {
		writeWorkloadTimescaleJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to resolve database scope"})
		return
	}

	res, err := h.metricsSvc.GetSqlServerWorkloadSummary(r.Context(), id, from, to, filter)
	if err != nil {
		writeWorkloadTimescaleJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load workload summary"})
		return
	}
	writeWorkloadTimescaleJSON(w, http.StatusOK, res)
}

func (h *SqlServerWorkloadHandlers) GetTrends(w http.ResponseWriter, r *http.Request) {
	id, ok := h.parseID(r)
	if !ok {
		writeWorkloadTimescaleJSON(w, http.StatusBadRequest, map[string]string{"error": "instance name or server_id required"})
		return
	}

	from, to := ParseTimeRange(r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	filter, err := h.workloadFilter(r.Context(), r, id, from, to)
	if err != nil {
		writeWorkloadTimescaleJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to resolve database scope"})
		return
	}

	res, err := h.metricsSvc.GetSqlServerWorkloadTrends(r.Context(), id, from, to, filter)
	if err != nil {
		writeWorkloadTimescaleJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load workload trends"})
		return
	}
	writeWorkloadTimescaleJSON(w, http.StatusOK, map[string]interface{}{"trends": res})
}

func (h *SqlServerWorkloadHandlers) GetTopOffenders(w http.ResponseWriter, r *http.Request) {
	id, ok := h.parseID(r)
	if !ok {
		writeWorkloadTimescaleJSON(w, http.StatusBadRequest, map[string]string{"error": "instance name or server_id required"})
		return
	}

	from, to := ParseTimeRange(r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	filter, err := h.workloadFilter(r.Context(), r, id, from, to)
	if err != nil {
		writeWorkloadTimescaleJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to resolve database scope"})
		return
	}
	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if val, err := strconv.Atoi(l); err == nil && val > 0 {
			limit = val
		}
	}

	res, err := h.metricsSvc.GetSqlServerWorkloadTopOffenders(r.Context(), id, from, to, limit, filter)
	if err != nil {
		slog.Error("GetSqlServerWorkloadTopOffenders", "instance", id, "database", filter.Database, "err", err)
		writeWorkloadTimescaleJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to load top queries",
		})
		return
	}
	writeWorkloadTimescaleJSON(w, http.StatusOK, map[string]interface{}{"top_offenders": res})
}

func (h *SqlServerWorkloadHandlers) GetAppLoadTimeline(w http.ResponseWriter, r *http.Request) {
	h.handleTimeline(w, r, "app_load", h.metricsSvc.GetSqlServerWorkloadAppLoadTimeline)
}

func (h *SqlServerWorkloadHandlers) GetLoginLoadTimeline(w http.ResponseWriter, r *http.Request) {
	h.handleTimeline(w, r, "login_load", h.metricsSvc.GetSqlServerWorkloadLoginLoadTimeline)
}

func (h *SqlServerWorkloadHandlers) GetTopApps(w http.ResponseWriter, r *http.Request) {
	h.handleTopN(w, r, "top_apps", h.metricsSvc.GetSqlServerWorkloadTopApps)
}

func (h *SqlServerWorkloadHandlers) GetTopLogins(w http.ResponseWriter, r *http.Request) {
	h.handleTopN(w, r, "top_logins", h.metricsSvc.GetSqlServerWorkloadTopLogins)
}

type workloadTimelineFn func(context.Context, uuid.UUID, time.Time, time.Time, domain.WorkloadQueryFilter) (interface{}, error)

func (h *SqlServerWorkloadHandlers) handleTimeline(w http.ResponseWriter, r *http.Request, wrapKey string, fn workloadTimelineFn) {
	id, ok := h.parseID(r)
	if !ok {
		writeWorkloadTimescaleJSON(w, http.StatusBadRequest, map[string]string{"error": "instance name or server_id required"})
		return
	}

	from, to := ParseTimeRange(r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	filter, err := h.workloadFilter(r.Context(), r, id, from, to)
	if err != nil {
		writeWorkloadTimescaleJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to resolve database scope"})
		return
	}

	res, err := fn(r.Context(), id, from, to, filter)
	if err != nil {
		writeWorkloadTimescaleJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load timeline"})
		return
	}
	writeWorkloadTimescaleJSON(w, http.StatusOK, map[string]interface{}{wrapKey: res})
}

type workloadTopNFn func(context.Context, uuid.UUID, time.Time, time.Time, int, domain.WorkloadQueryFilter) (interface{}, error)

func (h *SqlServerWorkloadHandlers) handleTopN(w http.ResponseWriter, r *http.Request, wrapKey string, fn workloadTopNFn) {
	id, ok := h.parseID(r)
	if !ok {
		writeWorkloadTimescaleJSON(w, http.StatusBadRequest, map[string]string{"error": "instance name or server_id required"})
		return
	}

	from, to := ParseTimeRange(r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	filter, err := h.workloadFilter(r.Context(), r, id, from, to)
	if err != nil {
		writeWorkloadTimescaleJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to resolve database scope"})
		return
	}
	limit := 10
	if l := r.URL.Query().Get("limit"); l != "" {
		if val, err := strconv.Atoi(l); err == nil {
			limit = val
		}
	}

	res, err := fn(r.Context(), id, from, to, limit, filter)
	if err != nil {
		writeWorkloadTimescaleJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load top N"})
		return
	}
	writeWorkloadTimescaleJSON(w, http.StatusOK, map[string]interface{}{wrapKey: res})
}
