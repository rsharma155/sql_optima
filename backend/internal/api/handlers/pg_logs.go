// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: HTTP handlers for PostgreSQL logs analysis and summaries.
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


func (h *PostgresHandlers) LogsReport(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
		return
	}
	if !h.metricsSvc.IsTimescaleConnected() {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "timescale not connected"})
		return
	}

	var req pgLogReportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid json body"})
		return
	}
	instance := strings.TrimSpace(req.Instance)
	if instance == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance is required"})
		return
	}
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

	rows := make([]hot.PostgresLogEventRow, 0, len(req.Events))
	for _, e := range req.Events {
		if strings.TrimSpace(e.Message) == "" {
			continue
		}
		ts := time.Now().UTC()
		if e.CaptureTimestamp != nil {
			ts = e.CaptureTimestamp.UTC()
		}
		rows = append(rows, hot.PostgresLogEventRow{
			CaptureTimestamp:   ts,
			ServerInstanceName: instance,
			Severity:           e.Severity,
			SQLState:           e.SQLState,
			Message:            e.Message,
			UserName:           e.UserName,
			DatabaseName:       e.DatabaseName,
			ApplicationName:    e.ApplicationName,
			ClientAddr:         e.ClientAddr,
			PID:                e.PID,
			Context:            e.Context,
			Detail:             e.Detail,
			Hint:               e.Hint,
			Raw:                e.Raw,
		})
	}
	if len(rows) == 0 {
		json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "inserted": 0})
		return
	}

	if err := h.metricsSvc.LogPostgresLogEvents(r.Context(), instance, rows); err != nil {
		log.Printf("[API] PG logs report insert error for %s: %v", instance, err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to store log events"})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "inserted": len(rows)})
}

func (h *PostgresHandlers) LogsSummary(w http.ResponseWriter, r *http.Request) {
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
	windowMin := 60
	if wq := r.URL.Query().Get("window_minutes"); wq != "" {
		if n, err := strconv.Atoi(wq); err == nil && n > 0 && n <= 1440 {
			windowMin = n
		}
	}
	if !h.metricsSvc.IsTimescaleConnected() {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"instance": instance,
			"summary":  (*hot.PostgresLogSummary)(nil),
			"source":   "none",
		})
		return
	}
	s, err := h.metricsSvc.GetPostgresLogSummary(r.Context(), instance, windowMin)
	if err != nil {
		log.Printf("[API] PG logs summary error for %s: %v", instance, err)
		s = &hot.PostgresLogSummary{WindowMinutes: windowMin}
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"summary":  s,
		"source":   "timescale",
	})
}

func (h *PostgresHandlers) LogsRecent(w http.ResponseWriter, r *http.Request) {
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
	limit := 200
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 2000 {
			limit = n
		}
	}
	severity := r.URL.Query().Get("severity")
	if !h.metricsSvc.IsTimescaleConnected() {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"instance": instance,
			"events":   []hot.PostgresLogEventRow{},
			"source":   "none",
		})
		return
	}
	rows, err := h.metricsSvc.GetPostgresLogEvents(r.Context(), instance, limit, severity)
	if err != nil {
		rows = []hot.PostgresLogEventRow{}
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"events":   rows,
		"source":   "timescale",
	})
}
