// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
//
// Purpose: Implementations for legacy /api/postgres/* routes previously stubbed with empty JSON.

package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/rsharma155/sql_optima/internal/storage/hot"
)

func stubNotImplemented(w http.ResponseWriter, endpoint string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "not_implemented",
		"error":   endpoint + " is not implemented",
		"message": "This endpoint is reserved for a future release.",
	})
}

func (h *PostgresHandlers) Config(w http.ResponseWriter, r *http.Request) {
	sid, ok := h.parseID(r)
	if !ok {
		http.Error(w, "invalid instance", http.StatusBadRequest)
		return
	}
	instName := r.URL.Query().Get("instance")
	if instName == "" {
		for _, inst := range h.cfg.Instances {
			if inst.ServerID == sid {
				instName = inst.Name
				break
			}
		}
	}
	if h.metricsSvc == nil || h.metricsSvc.PgRepo == nil {
		stubNotImplemented(w, "/api/postgres/config")
		return
	}
	rows, err := h.metricsSvc.PgRepo.FetchPgConfigSettings(r.Context(), instName)
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	settings := make([]map[string]string, 0, len(rows))
	for _, row := range rows {
		settings = append(settings, map[string]string{
			"name":          row.Name,
			"value":         row.Setting,
			"unit":          row.Unit,
			"default_value": row.DefaultValue,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"settings": settings})
}

func (h *PostgresHandlers) DatabaseSize(w http.ResponseWriter, r *http.Request) {
	sid, ok := h.parseID(r)
	if !ok {
		http.Error(w, "invalid instance", http.StatusBadRequest)
		return
	}
	instName := r.URL.Query().Get("instance")
	if instName == "" {
		for _, inst := range h.cfg.Instances {
			if inst.ServerID == sid {
				instName = inst.Name
				break
			}
		}
	}
	if h.metricsSvc == nil || h.metricsSvc.PgRepo == nil {
		stubNotImplemented(w, "/api/postgres/database-size")
		return
	}
	stats := h.metricsSvc.PgRepo.GetDatabaseSizeStats(r.Context(), instName)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(stats)
}

func (h *PostgresHandlers) SessionStateHistory(w http.ResponseWriter, r *http.Request) {
	sid, ok := h.parseID(r)
	if !ok {
		http.Error(w, "invalid instance", http.StatusBadRequest)
		return
	}
	limit := 180
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}
	rows, err := h.metricsSvc.GetPostgresSessionStateCountsHistory(r.Context(), sid, limit)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	labels := make([]string, 0, len(rows))
	active := make([]int, 0, len(rows))
	idle := make([]int, 0, len(rows))
	idleInTxn := make([]int, 0, len(rows))
	waiting := make([]int, 0, len(rows))
	for i := len(rows) - 1; i >= 0; i-- {
		rw := rows[i]
		labels = append(labels, rw.CaptureTimestamp.UTC().Format(time.RFC3339))
		active = append(active, rw.ActiveCount)
		idle = append(idle, rw.IdleCount)
		idleInTxn = append(idleInTxn, rw.IdleInTxnCount)
		waiting = append(waiting, rw.WaitingCount)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"history": map[string]interface{}{
			"labels":      labels,
			"active":      active,
			"idle":        idle,
			"idle_in_txn": idleInTxn,
			"waiting":     waiting,
		},
	})
}

func (h *PostgresHandlers) TableMaintenanceLatest(w http.ResponseWriter, r *http.Request) {
	sid, ok := h.parseID(r)
	if !ok {
		http.Error(w, "invalid instance", http.StatusBadRequest)
		return
	}
	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}
	res, err := h.metricsSvc.GetLatestPostgresTableMaint(r.Context(), sid, limit)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if res == nil {
		res = []hot.PostgresTableMaintResponse{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"tables": res})
}

func (h *PostgresHandlers) PoolerLatest(w http.ResponseWriter, r *http.Request) {
	sid, ok := h.parseID(r)
	if !ok {
		http.Error(w, "invalid instance", http.StatusBadRequest)
		return
	}
	res, err := h.metricsSvc.GetLatestPostgresPoolerStats(r.Context(), sid)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (h *PostgresHandlers) PoolerHistory(w http.ResponseWriter, r *http.Request) {
	sid, ok := h.parseID(r)
	if !ok {
		http.Error(w, "invalid instance", http.StatusBadRequest)
		return
	}
	limit := 60
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}
	res, err := h.metricsSvc.GetPostgresPoolerStatsHistory(r.Context(), sid, limit)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if res == nil {
		res = []hot.PostgresPoolerStatRow{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"history": res})
}

func (h *PostgresHandlers) ConnectionUtilizationHistory(w http.ResponseWriter, r *http.Request) {
	sid, ok := h.parseID(r)
	if !ok {
		http.Error(w, "invalid instance", http.StatusBadRequest)
		return
	}
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	history, err := h.metricsSvc.GetPostgresControlCenterHistory(r.Context(), sid, from, to, 120)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"labels":          history.Labels,
		"utilization_pct": history.ConnectionsUsagePct,
	})
}

func (h *PostgresHandlers) BackupLatest(w http.ResponseWriter, r *http.Request) {
	sid, ok := h.parseID(r)
	if !ok {
		http.Error(w, "invalid instance", http.StatusBadRequest)
		return
	}
	res, err := h.metricsSvc.GetLatestPostgresBackupRun(r.Context(), sid)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (h *PostgresHandlers) WALArchiverRisk(w http.ResponseWriter, r *http.Request) {
	sid, ok := h.parseID(r)
	if !ok {
		http.Error(w, "invalid instance", http.StatusBadRequest)
		return
	}
	res, err := h.metricsSvc.GetLatestPostgresWALArchiverRisk(r.Context(), sid)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

// Dashboard is not implemented; returns HTTP 501.
func (h *PostgresHandlers) Dashboard(w http.ResponseWriter, r *http.Request) {
	stubNotImplemented(w, "/api/postgres/dashboard")
}
func (h *PostgresHandlers) DBObservation(w http.ResponseWriter, r *http.Request) {
	stubNotImplemented(w, "/api/postgres/db-observation")
}
func (h *PostgresHandlers) BlockingTree(w http.ResponseWriter, r *http.Request) {
	stubNotImplemented(w, "/api/postgres/blocking-tree")
}
func (h *PostgresHandlers) Disk(w http.ResponseWriter, r *http.Request) {
	stubNotImplemented(w, "/api/postgres/disk")
}
func (h *PostgresHandlers) BackupReport(w http.ResponseWriter, r *http.Request) {
	stubNotImplemented(w, "/api/postgres/backups/report")
}
func (h *PostgresHandlers) LogsReport(w http.ResponseWriter, r *http.Request) {
	stubNotImplemented(w, "/api/postgres/logs/report")
}
