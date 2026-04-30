// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: HTTP handlers for PostgreSQL maintenance tasks like vacuum progress and XID risk.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rsharma155/sql_optima/internal/storage/hot"
)



func (h *PostgresHandlers) VacuumProgress(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if !instanceExists(r.Context(), h.cfg, h.metricsSvc, instance) || !instanceTypeFromDB(r.Context(), h.cfg, h.metricsSvc, instance, "postgres") {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	// Fetch detailed progress from TimescaleDB (last 10 entries)
	rows, err := h.metricsSvc.GetTimescalePostgresVacuumProgress(instance, 10)
	if err != nil {
		log.Printf("[API] PG vacuum-progress (timescale) error for %s: %v", instance, err)
		json.NewEncoder(w).Encode(map[string]interface{}{"instance": instance, "progress": []interface{}{}})
		return
	}

	w.Header().Set("X-Data-Source", "timescale")
	
	// Map to expected UI fields
	type uiProgress struct {
		PID               int64     `json:"pid"`
		DatabaseName      string    `json:"database_name"`
		RelationName      string    `json:"relation_name"`
		Phase             string    `json:"phase"`
		ProgressPct       float64   `json:"progress_pct"`
		HeapBlksVacuumed  int64     `json:"heap_blks_vacuumed"`
	}
	var uiRows []uiProgress
	now := time.Now().UTC()
	for _, r := range rows {
		// Only show snapshots from last 10 minutes for "live" view
		if now.Sub(r.CaptureTimestamp) > 10*time.Minute {
			continue
		}
		pct := 0.0
		if r.HeapBlksTotal > 0 {
			pct = float64(r.HeapBlksScanned) / float64(r.HeapBlksTotal) * 100.0
		}
		uiRows = append(uiRows, uiProgress{
			PID:               r.PID,
			DatabaseName:      r.DatabaseName,
			RelationName:      r.RelationName,
			Phase:              r.Phase,
			ProgressPct:       pct,
			HeapBlksVacuumed:  r.HeapBlksVacuumed,
		})
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"progress": uiRows,
		"source":   "timescale",
	})
}



func (h *PostgresHandlers) VacuumProgressHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if !instanceExists(r.Context(), h.cfg, h.metricsSvc, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}
	if !instanceTypeFromDB(r.Context(), h.cfg, h.metricsSvc, instance, "postgres") {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance is not postgres"})
		return
	}
	limit := 200
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 2000 {
			limit = n
		}
	}
	rows, err := h.metricsSvc.GetTimescalePostgresVacuumProgress(instance, limit)
	if err != nil {
		rows = []hot.PostgresVacuumProgressRow{}
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"history":  rows,
		"source":   "timescale",
	})
}

func (h *PostgresHandlers) TableMaintenanceHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if !instanceExists(r.Context(), h.cfg, h.metricsSvc, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}
	if !instanceTypeFromDB(r.Context(), h.cfg, h.metricsSvc, instance, "postgres") {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance is not postgres"})
		return
	}
	schema := r.URL.Query().Get("schema")
	table := r.URL.Query().Get("table")
	database := r.URL.Query().Get("database")
	if strings.TrimSpace(schema) == "" || strings.TrimSpace(table) == "" || strings.TrimSpace(database) == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "database, schema and table are required"})
		return
	}
	limit := 180
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 2000 {
			limit = n
		}
	}
	rows, err := h.metricsSvc.GetPostgresTableMaintenanceHistory(r.Context(), instance, database, schema, table, limit)
	if err != nil {
		rows = []hot.PostgresTableMaintRow{}
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"schema":   schema,
		"table":    table,
		"history":  rows,
		"source":   "timescale",
	})
}

func (h *PostgresHandlers) TableMaintenanceLatest(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if !instanceExists(r.Context(), h.cfg, h.metricsSvc, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}
	if !instanceTypeFromDB(r.Context(), h.cfg, h.metricsSvc, instance, "postgres") {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance is not postgres"})
		return
	}
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}
	if !h.metricsSvc.IsTimescaleConnected() {
		json.NewEncoder(w).Encode(map[string]interface{}{"instance": instance, "latest": []hot.PostgresTableMaintRow{}, "source": "none"})
		return
	}
	rows, err := h.metricsSvc.GetLatestPostgresTableMaintenance(r.Context(), instance, limit)
	if err != nil {
		rows = []hot.PostgresTableMaintRow{}
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"latest":   rows,
		"source":   "timescale",
	})
}

func (h *PostgresHandlers) XIDWraparoundRisk(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if !instanceExists(r.Context(), h.cfg, h.metricsSvc, instance) || !instanceTypeFromDB(r.Context(), h.cfg, h.metricsSvc, instance, "postgres") {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid instance"})
		return
	}
	
	snap, err := h.metricsSvc.GetLatestPostgresXIDRisk(r.Context(), instance)
	if err != nil {
		log.Printf("[API] PG xid-wraparound (timescale) error for %s: %v", instance, err)
		json.NewEncoder(w).Encode(map[string]interface{}{"instance": instance, "databases": []interface{}{}})
		return
	}

	w.Header().Set("X-Data-Source", "timescale")
	// Map snapshot fields to expected UI format (PgXIDWraparoundRisk structure)
	usedPct := float64(snap.MaxXIDAge) / 2000000000.0 * 100.0
	riskLevel := "low"
	if usedPct > 80 {
		riskLevel = "critical"
	} else if usedPct > 50 {
		riskLevel = "high"
	} else if usedPct > 20 {
		riskLevel = "medium"
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"databases": []map[string]interface{}{
			{
				"database_name": "ALL",
				"freeze_age":    snap.MaxXIDAge,
				"used_pct":      usedPct,
				"risk_level":    riskLevel,
			},
		},
	})
}


// WALArchiverRisk returns a combined WAL archiver health and slot retention risk summary.
func (h *PostgresHandlers) WALArchiverRisk(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if !instanceExists(r.Context(), h.cfg, h.metricsSvc, instance) || !instanceTypeFromDB(r.Context(), h.cfg, h.metricsSvc, instance, "postgres") {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid instance"})
		return
	}

	snap, err := h.metricsSvc.GetLatestPostgresWALArchiverRisk(r.Context(), instance)
	if err != nil {
		log.Printf("[API] PG wal-archiver-risk (timescale) error for %s: %v", instance, err)
		json.NewEncoder(w).Encode(map[string]interface{}{"instance": instance, "risk": nil})
		return
	}

	w.Header().Set("X-Data-Source", "timescale")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"risk": map[string]interface{}{
			"wal_mb_per_min":    snap.WALMBPerMin,
			"replication_slots": snap.ReplicationSlots,
			"replica_lag_sec":   snap.ReplicaLagSec,
		},
	})
}


// LongRunningTransactions returns active transactions running longer than 1 minute.
