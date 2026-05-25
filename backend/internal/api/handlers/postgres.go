// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Entry point for PostgreSQL handlers, defining the PostgresHandlers struct and constructor.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"context"
	"log/slog"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/models"
	"github.com/rsharma155/sql_optima/internal/service"
)

type PostgresHandlers struct {
	metricsSvc *service.MetricsService
	cfg        *config.Config
}

func NewPostgresHandlers(metricsSvc *service.MetricsService, cfg *config.Config) *PostgresHandlers {
	return &PostgresHandlers{metricsSvc: metricsSvc, cfg: cfg}
}

func (h *PostgresHandlers) parseID(r *http.Request) (uuid.UUID, bool) {
	return ParseServerID(r, h.cfg)
}

func (h *PostgresHandlers) Databases(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	dbs, err := h.metricsSvc.PgRepo.GetDatabases(r.Context(), instance)
	if err != nil {
		slog.Error("[API] PG databases error", "target", instance, "err", err)
		json.NewEncoder(w).Encode(map[string]interface{}{"databases": []string{}})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance":  instance,
		"databases": dbs,
	})
}

func (h *PostgresHandlers) CheckpointHealthHistory(w http.ResponseWriter, r *http.Request) {
	sid, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if h.metricsSvc.GetTimescaleDBLogger() == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "TimescaleDB logger not initialized"})
		return
	}
	from, to := ParseTimeRange(r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	limit := 100
	if lStr := r.URL.Query().Get("limit"); lStr != "" {
		if val, err := strconv.Atoi(lStr); err == nil {
			limit = val
		}
	}

	data, err := h.metricsSvc.GetPostgresCheckpointSummary(r.Context(), sid, from, to, limit)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if data == nil {
		_ = json.NewEncoder(w).Encode([]interface{}{})
	} else {
		_ = json.NewEncoder(w).Encode(data)
	}
}

func (h *PostgresHandlers) BGWriter(w http.ResponseWriter, r *http.Request) {
	sid, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	from, to := ParseTimeRange(r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	limit := 100
	if lStr := r.URL.Query().Get("limit"); lStr != "" {
		limit, _ = strconv.Atoi(lStr)
	}
	data, err := h.metricsSvc.GetPostgresBGWriterHistory(r.Context(), sid, from, to, limit)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"stats": data})
}

func (h *PostgresHandlers) Archiver(w http.ResponseWriter, r *http.Request) {
	sid, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	from, to := ParseTimeRange(r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	limit := 100
	if lStr := r.URL.Query().Get("limit"); lStr != "" {
		limit, _ = strconv.Atoi(lStr)
	}
	data, err := h.metricsSvc.GetPostgresArchiverHistory(r.Context(), sid, from, to, limit)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"stats": data})
}

func (h *PostgresHandlers) DbIOHistory(w http.ResponseWriter, r *http.Request) {
	sid, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	from, to := ParseTimeRange(r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	limit := 100
	if lStr := r.URL.Query().Get("limit"); lStr != "" {
		limit, _ = strconv.Atoi(lStr)
	}
	data, err := h.metricsSvc.GetPostgresDbIOHistory(r.Context(), sid, from, to, limit)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"rows": data})
}

func (h *PostgresHandlers) LocksBlockingTimeline(w http.ResponseWriter, r *http.Request) {
	sid, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	from, to := ParseTimeRange(r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	data, err := h.metricsSvc.GetPgLocksBlockingTimeline(r.Context(), sid, from, to)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(data)
}

func (h *PostgresHandlers) LocksBlockingTopLockedTables(w http.ResponseWriter, r *http.Request) {
	sid, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	from, to := ParseTimeRange(r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	limit := 10
	if lStr := r.URL.Query().Get("limit"); lStr != "" {
		limit, _ = strconv.Atoi(lStr)
	}
	data, err := h.metricsSvc.GetPgLocksBlockingTopLockedTables(r.Context(), sid, from, to, limit)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"tables": data})
}

func (h *PostgresHandlers) LocksBlockingDetails(w http.ResponseWriter, r *http.Request) {
	sid, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	from, to := ParseTimeRange(r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	data, err := h.metricsSvc.GetPgLocksBlockingDetails(r.Context(), sid, from, to)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(data)
}

func (h *PostgresHandlers) IncidentList(w http.ResponseWriter, r *http.Request) {
	sid, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")

	// Derive time range from window_hours param, or from/to if provided.
	var from, to time.Time
	if f := r.URL.Query().Get("from"); f != "" {
		from, to = ParseTimeRange(f, r.URL.Query().Get("to"))
	} else {
		wh, _ := strconv.Atoi(r.URL.Query().Get("window_hours"))
		if wh <= 0 {
			wh = 24
		}
		to = time.Now().UTC()
		from = to.Add(-time.Duration(wh) * time.Hour)
	}

	incidents, err := h.metricsSvc.GetPgBlockingIncidentsList(r.Context(), sid, from, to)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "failed to load incidents"})
		return
	}

	// Build response with duration_seconds computed field.
	type incRow struct {
		IncidentID          int64      `json:"incident_id"`
		StartedAt           time.Time  `json:"started_at"`
		EndedAt             *time.Time `json:"ended_at,omitempty"`
		DurationSeconds     float64    `json:"duration_seconds"`
		PeakBlockedSessions int        `json:"peak_blocked_sessions"`
		RootBlockerPID      *int       `json:"root_blocker_pid,omitempty"`
		RootBlockerQuery    string     `json:"root_blocker_query,omitempty"`
		Status              string     `json:"status"`
	}
	now := time.Now().UTC()
	rows := make([]incRow, 0, len(incidents))
	for _, inc := range incidents {
		end := now
		if inc.EndedAt != nil {
			end = *inc.EndedAt
		}
		rows = append(rows, incRow{
			IncidentID:          inc.IncidentID,
			StartedAt:           inc.StartedAt,
			EndedAt:             inc.EndedAt,
			DurationSeconds:     end.Sub(inc.StartedAt).Seconds(),
			PeakBlockedSessions: inc.PeakBlockedSessions,
			RootBlockerPID:      inc.RootBlockerPID,
			RootBlockerQuery:    inc.RootBlockerQuery,
			Status:              inc.Status,
		})
	}
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"incidents": rows})
}

func (h *PostgresHandlers) ChronicBlockers(w http.ResponseWriter, r *http.Request) {
	sid, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	from, to := ParseTimeRange(r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	data, err := h.metricsSvc.GetPgChronicBlockers(r.Context(), sid, from, to)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"blockers": data})
}

func (h *PostgresHandlers) DeadlocksHistory(w http.ResponseWriter, r *http.Request) {
	sid, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	minutes := 180
	if mStr := r.URL.Query().Get("window_minutes"); mStr != "" {
		minutes, _ = strconv.Atoi(mStr)
	}
	limit := 100
	if lStr := r.URL.Query().Get("limit"); lStr != "" {
		limit, _ = strconv.Atoi(lStr)
	}
	data, err := h.metricsSvc.GetPostgresDeadlockHistory(r.Context(), sid, minutes, limit)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"history": data})
}

func (h *PostgresHandlers) TableMaintenanceHistory(w http.ResponseWriter, r *http.Request) {
	sid, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	db := r.URL.Query().Get("db")
	sch := r.URL.Query().Get("schema")
	tab := r.URL.Query().Get("table")
	limit := 100
	if lStr := r.URL.Query().Get("limit"); lStr != "" {
		limit, _ = strconv.Atoi(lStr)
	}
	data, err := h.metricsSvc.GetPostgresTableMaintHistory(r.Context(), sid, db, sch, tab, limit)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"history": data})
}

func (h *PostgresHandlers) VacuumProgressHistory(w http.ResponseWriter, r *http.Request) {
	sid, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	limit := 100
	if lStr := r.URL.Query().Get("limit"); lStr != "" {
		limit, _ = strconv.Atoi(lStr)
	}
	data, err := h.metricsSvc.GetPostgresVacuumHistory(r.Context(), sid, limit)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"history": data})
}

func (h *PostgresHandlers) BackupHistory(w http.ResponseWriter, r *http.Request) {
	sid, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	limit := 100
	if lStr := r.URL.Query().Get("limit"); lStr != "" {
		limit, _ = strconv.Atoi(lStr)
	}
	data, err := h.metricsSvc.GetPostgresBackupHistory(r.Context(), sid, limit)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"history": data})
}

func (h *PostgresHandlers) LogsSummary(w http.ResponseWriter, r *http.Request) {
	sid, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	window := 60
	if wStr := r.URL.Query().Get("window_minutes"); wStr != "" {
		window, _ = strconv.Atoi(wStr)
	}
	data, err := h.metricsSvc.GetPostgresLogSummary(r.Context(), sid, window)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(data)
}

func (h *PostgresHandlers) LogsRecent(w http.ResponseWriter, r *http.Request) {
	sid, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	severity := r.URL.Query().Get("severity")
	limit := 100
	if lStr := r.URL.Query().Get("limit"); lStr != "" {
		limit, _ = strconv.Atoi(lStr)
	}
	data, err := h.metricsSvc.GetPostgresLogEvents(r.Context(), sid, limit, severity)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"events": data})
}

func (h *PostgresHandlers) SystemStats(w http.ResponseWriter, r *http.Request) {
	sid, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	data, err := h.metricsSvc.GetLatestPostgresControlCenterStats(r.Context(), sid)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(data)
}

func (h *PostgresHandlers) SystemStatsHistory(w http.ResponseWriter, r *http.Request) {
	sid, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	from, to := ParseTimeRange(r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	limit := 100
	data, err := h.metricsSvc.GetPostgresControlCenterHistory(r.Context(), sid, from.Format(time.RFC3339), to.Format(time.RFC3339), limit)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(data)
}

func (h *PostgresHandlers) ServerInfo(w http.ResponseWriter, r *http.Request) {
	sid, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	// Use overview or specific method
	res := h.metricsSvc.GetPostgresOverview(sid)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (h *PostgresHandlers) IncidentFeed(w http.ResponseWriter, r *http.Request) {
	sid, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if h.metricsSvc.GetTimescaleDBLogger() == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "TimescaleDB logger not initialized"})
		return
	}
	severity := 0
	if sStr := r.URL.Query().Get("severity"); sStr != "" {
		severity, _ = strconv.Atoi(sStr)
	}

	limit := 30
	if lStr := r.URL.Query().Get("limit"); lStr != "" {
		limit, _ = strconv.Atoi(lStr)
	}

	data, err := h.metricsSvc.GetPgIncidentFeed(r.Context(), sid, severity, limit)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if data == nil {
		_ = json.NewEncoder(w).Encode([]interface{}{})
	} else {
		_ = json.NewEncoder(w).Encode(data)
	}
}

func (h *PostgresHandlers) Overview(w http.ResponseWriter, r *http.Request) {
	sid, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	res := h.metricsSvc.GetPostgresOverview(sid)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (h *PostgresHandlers) ControlCenter(w http.ResponseWriter, r *http.Request) {
	sid, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if h.metricsSvc.GetTimescaleDBLogger() == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "TimescaleDB logger not initialized"})
		return
	}

	res, err := h.metricsSvc.GetLatestPostgresControlCenterStats(r.Context(), sid)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if res == nil {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"stats": map[string]interface{}{}})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"stats": res})
}

func (h *PostgresHandlers) DashboardV2(w http.ResponseWriter, r *http.Request) {
	sid, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	res, err := h.metricsSvc.GetPgDashboardV2(r.Context(), sid)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (h *PostgresHandlers) ControlCenterHistory(w http.ResponseWriter, r *http.Request) {
	sid, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if h.metricsSvc.GetTimescaleDBLogger() == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "TimescaleDB logger not initialized"})
		return
	}

	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	limit := 100
	if lStr := r.URL.Query().Get("limit"); lStr != "" {
		limit, _ = strconv.Atoi(lStr)
	}

	data, err := h.metricsSvc.GetPostgresControlCenterHistory(r.Context(), sid, from, to, limit)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if data == nil {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"labels": []string{}})
	} else {
		_ = json.NewEncoder(w).Encode(data)
	}
}

func (h *PostgresHandlers) Storage(w http.ResponseWriter, r *http.Request) {
	sid, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	data, err := h.metricsSvc.GetPostgresStorageMetrics(r.Context(), sid)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(data)
}

func (h *PostgresHandlers) VacuumProgress(w http.ResponseWriter, r *http.Request) {
	sid, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if h.metricsSvc.GetTimescaleDBLogger() == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "TimescaleDB logger not initialized"})
		return
	}
	data, err := h.metricsSvc.GetPostgresVacuumProgress(r.Context(), sid)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(data)
}

func (h *PostgresHandlers) BloatEstimates(w http.ResponseWriter, r *http.Request) {
	sid, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	limit := 100
	if lStr := r.URL.Query().Get("limit"); lStr != "" {
		limit, _ = strconv.Atoi(lStr)
	}

	data, err := h.metricsSvc.GetPostgresBloatEstimates(r.Context(), sid, limit)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(data)
}

func (h *PostgresHandlers) XIDWraparoundRisk(w http.ResponseWriter, r *http.Request) {
	sid, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	data, err := h.metricsSvc.GetPostgresXIDWraparoundRisk(r.Context(), sid)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(data)
}

func (h *PostgresHandlers) LongRunningTransactions(w http.ResponseWriter, r *http.Request) {
	sid, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	data, err := h.metricsSvc.GetPostgresLongRunningTransactions(r.Context(), sid)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(data)
}

func (h *PostgresHandlers) CPUHistory(w http.ResponseWriter, r *http.Request) {
	sid, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if h.metricsSvc.GetTimescaleDBLogger() == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "TimescaleDB logger not initialized"})
		return
	}

	from, to := ParseTimeRange(r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	limit := 100
	if lStr := r.URL.Query().Get("limit"); lStr != "" {
		limit, _ = strconv.Atoi(lStr)
	}

	data, err := h.metricsSvc.GetPostgresCPUHistory(r.Context(), sid, from, to, limit)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(data)
}

func (h *PostgresHandlers) CPUSaturation(w http.ResponseWriter, r *http.Request) {
	sid, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if h.metricsSvc.GetTimescaleDBLogger() == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "TimescaleDB logger not initialized"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	data, err := h.metricsSvc.GetTimescaleDBLogger().GetLatestOSCPUSaturation(r.Context(), sid)
	if err != nil {
		// Return empty map instead of 500 if no data found
		_ = json.NewEncoder(w).Encode(map[string]interface{}{})
		return
	}
	_ = json.NewEncoder(w).Encode(data)
}

func (h *PostgresHandlers) PgMemoryIntelligence(w http.ResponseWriter, r *http.Request) {
	sid, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if h.metricsSvc.GetTimescaleDBLogger() == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "TimescaleDB logger not initialized"})
		return
	}

	from, to := ParseTimeRange(r.URL.Query().Get("from"), r.URL.Query().Get("to"))

	data, err := h.metricsSvc.GetPostgresMemoryHistory(r.Context(), sid, from, to)
	if err != nil {
		slog.Error("[API] PgMemoryIntelligence error", "server_id", sid, "err", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error":                   "failed to load memory intelligence",
			"time_series":             []interface{}{},
			"components":              map[string]interface{}{},
			"os_collector_configured": false,
		})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(data)
}

func (h *PostgresHandlers) IndexBloat(w http.ResponseWriter, r *http.Request) {
	sid, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if h.metricsSvc.GetTimescaleDBLogger() == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "TimescaleDB logger not initialized"})
		return
	}
	data, err := h.metricsSvc.GetPostgresIndexBloat(r.Context(), sid)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(data)
}

func (h *PostgresHandlers) WaitEventsHistory(w http.ResponseWriter, r *http.Request) {
	sid, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	from, to := ParseTimeRange(r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	limit := 180
	if lStr := r.URL.Query().Get("limit"); lStr != "" {
		limit, _ = strconv.Atoi(lStr)
	}

	data, err := h.metricsSvc.GetPostgresWaitEventsHistory(r.Context(), sid, from, to, limit)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"rows": data})
}

func (h *PostgresHandlers) Sessions(w http.ResponseWriter, r *http.Request) {
	sid, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	data, err := h.metricsSvc.GetLatestPostgresSessions(r.Context(), sid)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(data)
}

func (h *PostgresHandlers) Locks(w http.ResponseWriter, r *http.Request) {
	sid, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	data, err := h.metricsSvc.GetLatestPostgresLocks(r.Context(), sid)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"locks": data})
}

func (h *PostgresHandlers) LocksBlockingKPIs(w http.ResponseWriter, r *http.Request) {
	sid, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	data, err := h.metricsSvc.GetPgLocksBlockingKpis(r.Context(), sid)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"kpis": data})
}

func (h *PostgresHandlers) LockWaitHistory(w http.ResponseWriter, r *http.Request) {
	sid, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	data, err := h.metricsSvc.GetPostgresLockWaitTrend(r.Context(), sid, 180)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(data)
}

func (h *PostgresHandlers) Replication(w http.ResponseWriter, r *http.Request) {
	sid, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	data, err := h.metricsSvc.GetPostgresReplicationLagTrend(r.Context(), sid, from, to, 100)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Fetch LIVE replication stats for the standby table
	stats, _ := h.metricsSvc.GetPostgresReplicationStatus(r.Context(), sid)

	// Satisfy frontend sidebar detection (router.js) and Replication page (replication.js)
	// which both expect a { stats: { is_primary: bool, standbys: [] } } structure.
	resp := map[string]interface{}{
		"series": data,
		"stats":  stats,
	}
	if stats == nil {
		resp["stats"] = map[string]interface{}{
			"is_primary": len(data) > 0,
			"standbys":   []interface{}{},
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *PostgresHandlers) ReplicationLagHistory(w http.ResponseWriter, r *http.Request) {
	sid, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	limit := 100
	if lStr := r.URL.Query().Get("limit"); lStr != "" {
		if val, err := strconv.Atoi(lStr); err == nil {
			limit = val
		}
	}

	data, err := h.metricsSvc.GetPostgresReplicationLagHistory(r.Context(), sid, from, to, limit)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"series": data,
	})
}

func (h *PostgresHandlers) ReplicationSlots(w http.ResponseWriter, r *http.Request) {
	sid, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	data, err := h.metricsSvc.GetTimescalePostgresReplicationSlots(sid, 100)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(data)
}

func (h *PostgresHandlers) BestPractices(w http.ResponseWriter, r *http.Request) {
	sid, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	data, err := h.metricsSvc.FetchPgBestPracticesWithTimescale(r.Context(), sid)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(data)
}

func (h *PostgresHandlers) ExportBestPracticesCSV(w http.ResponseWriter, r *http.Request) {
	instance := strings.TrimSpace(r.URL.Query().Get("instance"))
	if err := validateInstanceName(instance); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	sid, ok := h.parseID(r)
	if !ok {
		http.Error(w, "invalid instance", http.StatusBadRequest)
		return
	}
	h.runPgBestPracticesEvaluation(r.Context(), sid)
	raw, err := h.metricsSvc.FetchPgBestPracticesWithTimescale(r.Context(), sid)
	if err != nil {
		http.Error(w, "failed to load best practices", http.StatusInternalServerError)
		return
	}
	if m, ok := raw.(map[string]interface{}); ok {
		writePgRuleChecksCSV(w, csvFilename("postgres_best_practices", instance), m)
		return
	}
	result, ok := bestPracticesFromAny(raw)
	if !ok {
		http.Error(w, "unexpected response type", http.StatusInternalServerError)
		return
	}
	writeBestPracticesCSV(w, csvFilename("postgres_best_practices", instance), result)
}

func (h *PostgresHandlers) runPgBestPracticesEvaluation(ctx context.Context, serverID uuid.UUID) {
	pool := h.metricsSvc.GetTimescaleDBPool()
	if pool == nil || h.cfg == nil || serverID == uuid.Nil {
		return
	}
	rh := NewRulesHandler(pool, h.cfg, h.metricsSvc.IsOSMetricsIngestEnabled, h.metricsSvc.PgRepo)
	evalCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), ruleEvalRunTimeout)
	defer cancel()
	if err := rh.EvaluateServer(evalCtx, serverID, "postgres"); err != nil {
		slog.Warn("[ExportBestPracticesCSV] rule evaluation failed", "server_id", serverID, "err", err)
	}
}

func (h *PostgresHandlers) Queries(w http.ResponseWriter, r *http.Request) {
	sid, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	from, to := ParseTimeRange(r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	w.Header().Set("Content-Type", "application/json")

	instName := r.URL.Query().Get("instance")
	if h.metricsSvc.PgRepo != nil && !h.metricsSvc.PgRepo.GetPgssSupported(instName) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"queries":                    []interface{}{},
			"pg_stat_statements_enabled": false,
			"stats_source":               "timescale_delta",
		})
		return
	}

	pool := h.metricsSvc.GetTimescaleDBPool()
	if pool == nil {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"queries": []interface{}{}, "stats_source": "timescale_delta",
		})
		return
	}
	queries, err := queryPgssTopQueries(r.Context(), pool, sid, from, to)
	if err != nil {
		slog.Error("[Queries] pgss query error", "err", err)
		queries = []map[string]interface{}{}
	}
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"queries":                    queries,
		"pg_stat_statements_enabled": true,
		"stats_source":               "timescale_delta",
		"stats_note":                 "Aggregated from pg_stat_statements deltas (pgss_delta_1m).",
		"window_from":                from.Format(time.RFC3339),
		"window_to":                  to.Format(time.RFC3339),
	})
}

func (h *PostgresHandlers) Alerts(w http.ResponseWriter, r *http.Request) {
	sid, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	data, err := h.metricsSvc.GetPgIncidentFeed(r.Context(), sid, 0, 100)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(data)
}

// pgssResolve resolves the instance param distinguishing missing (→400) from not-found (→404)
// and wrong type (→400).
func (h *PostgresHandlers) pgssResolve(w http.ResponseWriter, r *http.Request) (id uuid.UUID, name string, ok bool) {
	var outcome instanceLookup
	id, name, outcome = resolveInstanceParam(r, h.cfg)
	w.Header().Set("Content-Type", "application/json")
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
	if h.cfg != nil {
		up := strings.ToUpper(name)
		for _, inst := range h.cfg.Instances {
			if strings.ToUpper(inst.Name) == up && strings.ToLower(inst.Type) != "postgres" {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "instance is not postgres"})
				return uuid.Nil, "", false
			}
		}
	}
	return id, name, true
}

func (h *PostgresHandlers) PgssStatus(w http.ResponseWriter, r *http.Request) {
	id, _, ok := h.pgssResolve(w, r)
	if !ok {
		return
	}
	res, err := h.metricsSvc.GetPgssStatus(r.Context(), id)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (h *PostgresHandlers) PgssWorkload(w http.ResponseWriter, r *http.Request) {
	id, name, ok := h.pgssResolve(w, r)
	if !ok {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if h.metricsSvc.GetTimescaleDBLogger() == nil {
		_ = json.NewEncoder(w).Encode(models.PgssWorkloadResponse{Instance: name, Points: []models.PgssWorkloadPoint{}})
		return
	}
	from, to := ParseTimeRange(r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	pts, err := h.metricsSvc.GetPgssWorkloadTrend(r.Context(), id, from, to)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	resp := models.PgssWorkloadResponse{Instance: name, Points: make([]models.PgssWorkloadPoint, 0, len(pts))}
	for _, p := range pts {
		resp.Points = append(resp.Points, models.PgssWorkloadPoint{
			Timestamp:          p.Timestamp,
			QueryLoadMsSec:     p.QueryLoadMsSec,
			QPS:                p.QPS,
			RowsSec:            p.RowsSec,
			CacheHitRatio:      p.CacheHitRatio,
			WalBytesSec:        p.WalBytesSec,
			ExecPct:            p.ExecPct,
			PlanPct:            p.PlanPct,
			TempMbSec:          p.TempMbSec,
			BlksReadSec:        p.BlksReadSec,
			RowsPerQuery:       p.RowsPerQuery,
			TempBlksWrittenSec: p.TempBlksWrittenSec,
		})
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *PostgresHandlers) PgssLatency(w http.ResponseWriter, r *http.Request) {
	id, name, ok := h.pgssResolve(w, r)
	if !ok {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if h.metricsSvc.GetTimescaleDBLogger() == nil {
		_ = json.NewEncoder(w).Encode(models.PgssLatencyResponse{Instance: name, Points: []models.PgssLatencyPoint{}})
		return
	}
	from, to := ParseTimeRange(r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	pts, err := h.metricsSvc.GetPgssLatencyTrend(r.Context(), id, from, to)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	resp := models.PgssLatencyResponse{Instance: name, Points: make([]models.PgssLatencyPoint, 0, len(pts))}
	for _, p := range pts {
		resp.Points = append(resp.Points, models.PgssLatencyPoint{
			Timestamp: p.Timestamp, P50: p.P50, P95: p.P95, P99: p.P99, MaxExec: p.MaxExec,
		})
	}
	_ = json.NewEncoder(w).Encode(resp)
}

var pgssAllowedSorts = map[string]bool{
	"total_time": true, "mean_time": true, "calls": true, "io": true,
	"temp": true, "wal": true, "planning": true,
}

func (h *PostgresHandlers) PgssTop(w http.ResponseWriter, r *http.Request) {
	id, name, ok := h.pgssResolve(w, r)
	if !ok {
		return
	}
	sortBy := r.URL.Query().Get("sort")
	if !pgssAllowedSorts[sortBy] {
		sortBy = "total_time"
	}
	dbName := sanitizeFilterParam(r.URL.Query().Get("db_name"))
	userName := sanitizeFilterParam(r.URL.Query().Get("username"))
	appName := sanitizeFilterParam(r.URL.Query().Get("app_name"))
	qType := sanitizeQueryType(r.URL.Query().Get("query_type"))
	hideSys := r.URL.Query().Get("hide_system") == "true"
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 50
	}
	w.Header().Set("Content-Type", "application/json")
	if h.metricsSvc.GetTimescaleDBLogger() == nil {
		_ = json.NewEncoder(w).Encode(models.PgssTopQueriesResponse{Instance: name, SortBy: sortBy, Queries: []models.PgssTopQuery{}})
		return
	}
	from, to := ParseTimeRange(r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	excludeUsers := pgssExcludeUsersForInstance(h.cfg, name)
	rows, err := h.metricsSvc.GetPgssTopQueries(r.Context(), id, from, to, sortBy, limit, dbName, userName, appName, qType, hideSys, excludeUsers)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	resp := models.PgssTopQueriesResponse{Instance: name, SortBy: sortBy, Queries: make([]models.PgssTopQuery, 0, len(rows))}
	for _, q := range rows {
		resp.Queries = append(resp.Queries, models.PgssTopQuery{
			QueryID: q.QueryID, Query: q.Query, DbName: q.DbName, UserName: q.UserName,
			AppName: q.AppName, QueryType: q.QueryType, TotalTime: q.TotalTime,
			PctDBTime: q.PctDBTime, Calls: q.Calls, AvgMs: q.AvgMs,
			RowsPerCall: q.RowsPerCall, HitPct: q.HitPct, TempMB: q.TempMB,
			WalMB: q.WalMB, ReadsPerCall: q.ReadsPerCall, PlanRatio: q.PlanRatio,
			Flags: q.Flags, CapturedAt: q.CapturedAt,
		})
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *PostgresHandlers) PgssRegressions(w http.ResponseWriter, r *http.Request) {
	id, name, ok := h.pgssResolve(w, r)
	if !ok {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if h.metricsSvc.GetTimescaleDBLogger() == nil {
		_ = json.NewEncoder(w).Encode(models.PgssRegressionsResponse{Instance: name, Regressions: []models.PgssRegression{}})
		return
	}
	rows, err := h.metricsSvc.GetPgssRegressions(r.Context(), id)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	resp := models.PgssRegressionsResponse{Instance: name, Regressions: make([]models.PgssRegression, 0, len(rows))}
	for _, row := range rows {
		resp.Regressions = append(resp.Regressions, models.PgssRegression{
			QueryID: row.QueryID, Query: row.Query, PrevAvgMs: row.PrevAvgMs,
			CurrAvgMs: row.CurrAvgMs, ChangePct: row.ChangePct, Status: row.Status,
		})
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *PostgresHandlers) PgssSummary(w http.ResponseWriter, r *http.Request) {
	id, name, ok := h.pgssResolve(w, r)
	if !ok {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	from, to := ParseTimeRange(r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	summary, err := h.metricsSvc.GetPgssSummary(r.Context(), id, from, to)
	if err != nil || summary == nil {
		_ = json.NewEncoder(w).Encode(models.PgssSummaryResponse{Instance: name})
		return
	}
	_ = json.NewEncoder(w).Encode(models.PgssSummaryResponse{
		Instance: name, QueryLoadMsSec: summary.QueryLoadMsSec, QPS: summary.QPS,
		P99Ms: summary.P99Ms, CacheHitPct: summary.CacheHitPct,
		TempMbSec: summary.TempMbSec, WalMbSec: summary.WalMbSec,
		UniqueQueryCount: summary.UniqueQueryCount,
	})
}

func (h *PostgresHandlers) PgssFilters(w http.ResponseWriter, r *http.Request) {
	id, name, ok := h.pgssResolve(w, r)
	if !ok {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if h.metricsSvc.GetTimescaleDBLogger() == nil {
		_ = json.NewEncoder(w).Encode(models.PgssFilterOptions{Instance: name})
		return
	}
	from, to := ParseTimeRange(r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	opts, err := h.metricsSvc.GetPgssFilterOptions(r.Context(), id, from, to)
	if err != nil || opts == nil {
		_ = json.NewEncoder(w).Encode(models.PgssFilterOptions{Instance: name})
		return
	}
	_ = json.NewEncoder(w).Encode(models.PgssFilterOptions{
		Instance: name, Databases: opts.Databases, Users: opts.Users, AppNames: opts.AppNames,
	})
}

func (h *PostgresHandlers) PgssDbBreakdown(w http.ResponseWriter, r *http.Request) {
	id, name, ok := h.pgssResolve(w, r)
	if !ok {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if h.metricsSvc.GetTimescaleDBLogger() == nil {
		_ = json.NewEncoder(w).Encode(models.PgssDbBreakdownResponse{Instance: name, Rows: []models.PgssDbBreakdown{}})
		return
	}
	from, to := ParseTimeRange(r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	rows, err := h.metricsSvc.GetPgssDbBreakdown(r.Context(), id, from, to)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	resp := models.PgssDbBreakdownResponse{Instance: name, Rows: make([]models.PgssDbBreakdown, 0, len(rows))}
	for _, row := range rows {
		resp.Rows = append(resp.Rows, models.PgssDbBreakdown{
			DbName: row.DbName, TotalExecMs: row.TotalExecMs, PctOfServer: row.PctOfServer,
			TotalCalls: row.TotalCalls, AvgMs: row.AvgMs, CacheHitPct: row.CacheHitPct,
			UniqueQueryIDs: row.UniqueQueryIDs,
		})
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *PostgresHandlers) PgssUserBreakdown(w http.ResponseWriter, r *http.Request) {
	id, name, ok := h.pgssResolve(w, r)
	if !ok {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if h.metricsSvc.GetTimescaleDBLogger() == nil {
		_ = json.NewEncoder(w).Encode(models.PgssUserBreakdownResponse{Instance: name, Rows: []models.PgssUserBreakdown{}})
		return
	}
	from, to := ParseTimeRange(r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	rows, err := h.metricsSvc.GetPgssUserBreakdown(r.Context(), id, from, to)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	resp := models.PgssUserBreakdownResponse{Instance: name, Rows: make([]models.PgssUserBreakdown, 0, len(rows))}
	for _, row := range rows {
		resp.Rows = append(resp.Rows, models.PgssUserBreakdown{
			UserName: row.UserName, TotalExecMs: row.TotalExecMs, PctOfServer: row.PctOfServer,
			TotalCalls: row.TotalCalls, AvgMs: row.AvgMs, UniqueQueryIDs: row.UniqueQueryIDs,
		})
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *PostgresHandlers) WaitSummary(w http.ResponseWriter, r *http.Request) {
	sid, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	from, to := ParseTimeRange(r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	limit := 100
	rows, err := h.metricsSvc.GetTimescaleDBLogger().GetPostgresWaitEventsHistory(r.Context(), sid, from, to, limit)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"data": []interface{}{}})
		return
	}

	type summaryRow struct {
		WaitEventType string  `json:"wait_event_type"`
		WaitEvent     string  `json:"wait_event"`
		AvgSessions   float64 `json:"avg_sessions"`
	}

	detailed := make(map[string]float64)
	distinctTimestamps := make(map[time.Time]bool)
	for _, r := range rows {
		key := r.WaitEventType + ":" + r.WaitEvent
		detailed[key] += float64(r.SessionsCount)
		distinctTimestamps[r.Timestamp] = true
	}

	count := len(distinctTimestamps)
	var data []summaryRow
	if count > 0 {
		for k, v := range detailed {
			parts := strings.Split(k, ":")
			data = append(data, summaryRow{
				WaitEventType: parts[0],
				WaitEvent:     parts[1],
				AvgSessions:   v / float64(count),
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"data": data})
}
