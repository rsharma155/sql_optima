/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: HTTP handlers for enhanced SQL Server Storage & Index Health diagnostics.
 *          Supports time-series trends and detailed table drill-downs.
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/rsharma155/sql_optima/internal/service"
)

type MssqlStorageIndexHandlers struct {
	metricsSvc *service.MetricsService
}

func NewMssqlStorageIndexHandlers(svc *service.MetricsService) *MssqlStorageIndexHandlers {
	return &MssqlStorageIndexHandlers{metricsSvc: svc}
}

// TableDrilldown returns a comprehensive analytical package for a specific table.
func (h *MssqlStorageIndexHandlers) TableDrilldown(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	engine := r.URL.Query().Get("engine")
	instance := r.URL.Query().Get("instance")
	db := r.URL.Query().Get("db")
	schema := r.URL.Query().Get("schema")
	table := r.URL.Query().Get("table")
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")

	if instance == "" || db == "" || table == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "instance, db, and table are required"})
		return
	}

	// Default to sqlserver if engine not provided
	if engine == "" {
		engine = "sqlserver"
	}

	// Default to last 24h if missing to avoid SQL cast errors
	if from == "" || to == "" {
		now := time.Now().UTC()
		if to == "" {
			to = now.Format(time.RFC3339)
		}
		if from == "" {
			from = now.Add(-24 * time.Hour).Format(time.RFC3339)
		}
	}

	// 1. Table Growth History
	growth, err := h.metricsSvc.GetTableSizeHistory(r.Context(), engine, instance, from, to, db, schema, table)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "growth: " + err.Error()})
		return
	}

	// 2. Index Usage History
	indices, _ := h.metricsSvc.GetIndexUsageHistory(r.Context(), engine, instance, from, to, db, schema, table)

	// 3. Fragmentation History
	frag, _ := h.metricsSvc.GetIndexFragmentationHistory(r.Context(), engine, instance, from, to, db, schema, table)

	// 4. Table Structure (latest)
	// (Logic can be added here to fetch from snapshot.mssql_table_structure_history)

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"instance":      instance,
		"database":      db,
		"table":         table,
		"growth_series": growth,
		"index_usage":   indices,
		"fragmentation": frag,
	})
}
