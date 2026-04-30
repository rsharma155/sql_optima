// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: HTTP handlers for PostgreSQL storage, database sizes, and bloat analysis.
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
	"time"

	"github.com/rsharma155/sql_optima/internal/repository"
	"github.com/rsharma155/sql_optima/internal/storage/hot"
)




func (h *PostgresHandlers) Databases(w http.ResponseWriter, r *http.Request) {
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

	databases, err := h.metricsSvc.GetTimescalePostgresDatabases(r.Context(), instance)
	if err != nil {
		log.Printf("[API] PG databases (timescale) error for %s: %v", instance, err)
		json.NewEncoder(w).Encode(map[string]interface{}{"databases": []string{}})
		return
	}

	w.Header().Set("X-Data-Source", "timescale")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance":  instance,
		"databases": databases,
	})
}

// ControlCenter returns the latest derived DBA-first metrics snapshot from TimescaleDB.
func (h *PostgresHandlers) Disk(w http.ResponseWriter, r *http.Request) {
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

	rows, err := h.metricsSvc.GetTimescalePostgresDiskStats(instance, 200)
	if err != nil {
		log.Printf("[API] PG disk error for %s: %v", instance, err)
		rows = []hot.PostgresDiskStatRow{}
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"stats":    rows,
		"source":   "timescale",
	})
}

type backupReportRequest struct {
	Instance         string                 `json:"instance"`
	Tool             string                 `json:"tool"`
	BackupType       string                 `json:"backup_type"`
	Status           string                 `json:"status"`
	StartedAt        *time.Time             `json:"started_at,omitempty"`
	FinishedAt       *time.Time             `json:"finished_at,omitempty"`
	DurationSeconds  int64                  `json:"duration_seconds"`
	WalArchivedUntil *time.Time             `json:"wal_archived_until,omitempty"`
	Repo             string                 `json:"repo,omitempty"`
	SizeBytes        int64                  `json:"size_bytes"`
	ErrorMessage     string                 `json:"error_message,omitempty"`
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
}

func (h *PostgresHandlers) Storage(w http.ResponseWriter, r *http.Request) {
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

	limit := 100
	rows, err := h.metricsSvc.GetLatestPostgresTableMaintenance(r.Context(), instance, limit)
	if err != nil {
		log.Printf("[API] PG storage (timescale) error for %s: %v", instance, err)
		json.NewEncoder(w).Encode(map[string]interface{}{"instance": instance, "tables": []interface{}{}, "indexes": []interface{}{}})
		return
	}

	w.Header().Set("X-Data-Source", "timescale")

	// Map to expected UI format
	type uiTable struct {
		Schema              string     `json:"schema"`
		Table               string     `json:"table"`
		DatabaseName        string     `json:"database_name"`
		TotalBytes          int64      `json:"total_bytes"`
		DeadTuples          int64      `json:"dead_tuples"`
		DeadPct             float64    `json:"dead_pct"`
		LastAutovacuum      *time.Time `json:"last_autovacuum"`
		VacuumLagSeconds    float64    `json:"vacuum_lag_seconds"`
		EstimatedWasteMB    float64    `json:"estimated_waste_mb"`
		Recommendation      string     `json:"recommendation"`
	}

	var uiTables []uiTable
	now := time.Now().UTC()
	for _, r := range rows {
		lag := -1.0
		if r.LastAutovacuum != nil {
			lag = now.Sub(*r.LastAutovacuum).Seconds()
		}
		
		waste := float64(r.TotalBytes) * (r.DeadPct / 100.0) / 1024.0 / 1024.0
		rec := "Healthy"
		if r.DeadPct > 20 {
			rec = "Autovacuum Lagging / High Churn"
		}

		uiTables = append(uiTables, uiTable{
			Schema:           r.SchemaName,
			Table:            r.TableName,
			DatabaseName:     r.DatabaseName,
			TotalBytes:       r.TotalBytes,
			DeadTuples:       r.DeadTuples,
			DeadPct:          r.DeadPct,
			LastAutovacuum:   r.LastAutovacuum,
			VacuumLagSeconds: lag,
			EstimatedWasteMB: waste,
			Recommendation:   rec,
		})
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"tables":   uiRowsToMap(uiTables), // Converting to compatible map if needed or just slice
		"indexes":  []interface{}{},      // Index detail needs its own Timescale table
		"source":   "timescale",
	})
}

func uiRowsToMap(rows interface{}) interface{} {
    return rows
}


func (h *PostgresHandlers) DatabaseSize(w http.ResponseWriter, r *http.Request) {
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
	json.NewEncoder(w).Encode(h.metricsSvc.PgRepo.GetDatabaseSizeStats(instance))
}

func (h *PostgresHandlers) BloatEstimates(w http.ResponseWriter, r *http.Request) {
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
	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}
	rows, err := h.metricsSvc.PgRepo.GetBloatEstimates(instance, limit)
	if err != nil {
		log.Printf("[API] PG bloat error for %s: %v", instance, err)
		rows = []repository.PgBloatEstimate{}
	}
	if rows == nil {
		rows = []repository.PgBloatEstimate{}
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"tables":   rows,
	})
}

// IdleInTransaction returns sessions currently stuck in idle-in-transaction state.
func (h *PostgresHandlers) IndexBloat(w http.ResponseWriter, r *http.Request) {
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
	limitStr := r.URL.Query().Get("limit")
	limit := 100
	if v, err := strconv.Atoi(limitStr); err == nil && v > 0 {
		limit = v
	}
	indexes, err := h.metricsSvc.PgRepo.GetIndexBloat(instance, limit)
	if err != nil {
		log.Printf("[API] PG index-bloat error for %s: %v", instance, err)
		indexes = []repository.PgIndexBloat{}
	}
	if indexes == nil {
		indexes = []repository.PgIndexBloat{}
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"indexes":  indexes,
	})
}
