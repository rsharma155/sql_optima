// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: HTTP handlers for PostgreSQL configuration, best practices, and settings drift.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/rsharma155/sql_optima/internal/storage/hot"
)


func (h *PostgresHandlers) SettingsDrift(w http.ResponseWriter, r *http.Request) {
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
	if !h.metricsSvc.IsTimescaleConnected() {
		json.NewEncoder(w).Encode(map[string]interface{}{"instance": instance, "changes": []interface{}{}, "source": "none"})
		return
	}

	latestTs, prevTs, latest, prev, err := h.metricsSvc.GetPostgresSettingsSnapshotLatestTwo(r.Context(), instance)
	if err != nil {
		log.Printf("[API] PG settings/drift error for %s: %v", instance, err)
		json.NewEncoder(w).Encode(map[string]interface{}{"instance": instance, "changes": []interface{}{}, "source": "timescale"})
		return
	}

	prevMap := map[string]hot.PostgresSettingSnapshotRow{}
	for _, r := range prev {
		prevMap[r.Name] = r
	}
	type change struct {
		Name      string `json:"name"`
		OldValue  string `json:"old_value"`
		NewValue  string `json:"new_value"`
		Unit      string `json:"unit"`
		OldSource string `json:"old_source"`
		NewSource string `json:"new_source"`
	}
	var changes []change
	for _, r := range latest {
		p, ok := prevMap[r.Name]
		if !ok {
			continue
		}
		if p.Setting != r.Setting || p.Source != r.Source || p.Unit != r.Unit {
			changes = append(changes, change{
				Name:      r.Name,
				OldValue:  p.Setting,
				NewValue:  r.Setting,
				Unit:      r.Unit,
				OldSource: p.Source,
				NewSource: r.Source,
			})
		}
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance":           instance,
		"latest_timestamp":   latestTs,
		"previous_timestamp": prevTs,
		"changes":            changes,
		"source":             "timescale",
	})
}

func (h *PostgresHandlers) Config(w http.ResponseWriter, r *http.Request) {
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

	settings, err := h.metricsSvc.PgRepo.GetConfig(instance)
	if err != nil {
		log.Printf("[API] PG config error for %s: %v", instance, err)
		json.NewEncoder(w).Encode(map[string]interface{}{"settings": []map[string]interface{}{}})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"settings": settings,
	})
}

func (h *PostgresHandlers) BestPractices(w http.ResponseWriter, r *http.Request) {
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

	result := h.metricsSvc.FetchPgBestPracticesWithTimescale(r.Context(), instance)
	if result.DataSource != "" {
		w.Header().Set("X-Data-Source", result.DataSource)
	}
	json.NewEncoder(w).Encode(result)
}
