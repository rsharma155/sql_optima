// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: CSV export handler for the PostgreSQL Best Practices dashboard.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"encoding/json"
	"net/http"
)

// ExportBestPracticesCSV streams the PostgreSQL Best Practices audit as a downloadable CSV.
func (h *PostgresHandlers) ExportBestPracticesCSV(w http.ResponseWriter, r *http.Request) {
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

	data := h.metricsSvc.FetchPgBestPracticesWithTimescale(r.Context(), instance)

	cw := newCSVResponse(w, "pg_best_practices", instance)

	cw.WriteSection("", []string{
		"Category", "Parameter", "Current Value", "Default Value", "Status", "Guidance", "Remediation SQL",
	})

	for _, check := range data.ServerConfig {
		cw.WriteRow([]string{
			check.Category,
			check.ConfigurationName,
			check.CurrentValue,
			check.DefaultValue,
			check.Status,
			check.Message,
			check.RemediationSQL,
		})
	}

	cw.Flush()
}
