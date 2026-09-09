// Package handlers provides HTTP handlers for the Wait Statistics Dashboard V2.
// Metadata:
//   - Feature: SQL Server Wait Stats V2
//   - Layer: Application (API)
package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/rsharma155/sql_optima/internal/apiresponse"
	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/service"
)

type SqlServerWaitStatsHandlers struct {
	metricsSvc *service.MetricsService
	cfg        *config.Config
}

func NewSqlServerWaitStatsHandlers(svc *service.MetricsService, cfg *config.Config) *SqlServerWaitStatsHandlers {
	return &SqlServerWaitStatsHandlers{metricsSvc: svc, cfg: cfg}
}

// GetKPIs returns the primary wait signals for the KPI strip.
func (h *SqlServerWaitStatsHandlers) GetKPIs(w http.ResponseWriter, r *http.Request) {
	serverID, ok := ParseServerID(r, h.cfg)
	if !ok {
		http.Error(w, "server_id or instance name required", http.StatusBadRequest)
		return
	}

	from, to := ParseTimeRange(r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	ctx := r.Context()

	if h.metricsSvc.GetTimescaleDBLogger() == nil {
		http.Error(w, "TimescaleDB not connected", http.StatusServiceUnavailable)
		return
	}

	kpis, err := h.metricsSvc.GetTimescaleDBLogger().GetWaitKPIsV2(ctx, serverID, from, to)
	if err != nil {
		apiresponse.WritePlainError(w, http.StatusInternalServerError, "failed to load wait stats", err, "handler", "sqlserver_wait_stats")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(kpis)
}

// GetTrends returns category-level wait time time-series.
func (h *SqlServerWaitStatsHandlers) GetTrends(w http.ResponseWriter, r *http.Request) {
	serverID, ok := ParseServerID(r, h.cfg)
	if !ok {
		http.Error(w, "server_id required", http.StatusBadRequest)
		return
	}

	from, to := ParseTimeRange(r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	ctx := r.Context()

	trends, err := h.metricsSvc.GetTimescaleDBLogger().GetWaitTrendsV2(ctx, serverID, from, to)
	if err != nil {
		apiresponse.WritePlainError(w, http.StatusInternalServerError, "failed to load wait stats", err, "handler", "sqlserver_wait_stats")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(trends)
}

// GetTopWaitTypes returns the most expensive wait types in the window.
func (h *SqlServerWaitStatsHandlers) GetTopWaitTypes(w http.ResponseWriter, r *http.Request) {
	serverID, ok := ParseServerID(r, h.cfg)
	if !ok {
		http.Error(w, "server_id required", http.StatusBadRequest)
		return
	}

	from, to := ParseTimeRange(r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	ctx := r.Context()

	top, err := h.metricsSvc.GetTimescaleDBLogger().GetTopWaitTypesV2(ctx, serverID, from, to, 10)
	if err != nil {
		apiresponse.WritePlainError(w, http.StatusInternalServerError, "failed to load wait stats", err, "handler", "sqlserver_wait_stats")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(top)
}

// GetCPUPressure returns signal wait vs scheduler yield trend.
func (h *SqlServerWaitStatsHandlers) GetCPUPressure(w http.ResponseWriter, r *http.Request) {
	serverID, ok := ParseServerID(r, h.cfg)
	if !ok {
		http.Error(w, "server_id required", http.StatusBadRequest)
		return
	}

	from, to := ParseTimeRange(r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	ctx := r.Context()

	cpu, err := h.metricsSvc.GetTimescaleDBLogger().GetCPUPressureTrend(ctx, serverID, from, to)
	if err != nil {
		apiresponse.WritePlainError(w, http.StatusInternalServerError, "failed to load wait stats", err, "handler", "sqlserver_wait_stats")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cpu)
}

// GetActiveSessions returns sessions from the most-recent snapshot in the selected window.
func (h *SqlServerWaitStatsHandlers) GetActiveSessions(w http.ResponseWriter, r *http.Request) {
	serverID, ok := ParseServerID(r, h.cfg)
	if !ok {
		http.Error(w, "server_id required", http.StatusBadRequest)
		return
	}

	from, to := ParseTimeRange(r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	ctx := r.Context()
	sessions, err := h.metricsSvc.GetTimescaleDBLogger().GetActiveWaitSessionsV2(ctx, serverID, from, to)
	if err != nil {
		apiresponse.WritePlainError(w, http.StatusInternalServerError, "failed to load wait stats", err, "handler", "sqlserver_wait_stats")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sessions)
}

// GetBlockingTree returns the blocking hierarchy at the most-recent snapshot in the time range.
func (h *SqlServerWaitStatsHandlers) GetBlockingTree(w http.ResponseWriter, r *http.Request) {
	serverID, ok := ParseServerID(r, h.cfg)
	if !ok {
		http.Error(w, "server_id required", http.StatusBadRequest)
		return
	}

	from, to := ParseTimeRange(r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	ctx := r.Context()
	tree, err := h.metricsSvc.GetTimescaleDBLogger().GetBlockingTree(ctx, serverID, from, to)
	if err != nil {
		apiresponse.WritePlainError(w, http.StatusInternalServerError, "failed to load wait stats", err, "handler", "sqlserver_wait_stats")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tree)
}

// GetDatabaseImpact returns wait time per database in the selected window.
func (h *SqlServerWaitStatsHandlers) GetDatabaseImpact(w http.ResponseWriter, r *http.Request) {
	serverID, ok := ParseServerID(r, h.cfg)
	if !ok {
		http.Error(w, "server_id required", http.StatusBadRequest)
		return
	}

	from, to := ParseTimeRange(r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	ctx := r.Context()
	impact, err := h.metricsSvc.GetTimescaleDBLogger().GetDatabaseWaitImpact(ctx, serverID, from, to)
	if err != nil {
		apiresponse.WritePlainError(w, http.StatusInternalServerError, "failed to load wait stats", err, "handler", "sqlserver_wait_stats")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(impact)
}

// GetWaitTypeHelp returns DBA guidance for a specific wait type.
func (h *SqlServerWaitStatsHandlers) GetWaitTypeHelp(w http.ResponseWriter, r *http.Request) {
	waitType := r.URL.Query().Get("wait_type")
	if waitType == "" {
		http.Error(w, "wait_type required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	help, err := h.metricsSvc.GetTimescaleDBLogger().GetWaitTypeHelp(ctx, waitType)
	if err != nil {
		apiresponse.WritePlainError(w, http.StatusInternalServerError, "failed to load wait stats", err, "handler", "sqlserver_wait_stats")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(help)
}
