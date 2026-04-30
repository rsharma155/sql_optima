// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: HTTP handlers for PostgreSQL backup reports and history.
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

	"github.com/jackc/pgx/v5"
	"github.com/rsharma155/sql_optima/internal/storage/hot"
)


func (h *PostgresHandlers) BackupReport(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
		return
	}

	var req backupReportRequest
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
	if !h.metricsSvc.IsTimescaleConnected() {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "timescale not connected"})
		return
	}

	row := hot.PostgresBackupRunRow{
		ServerInstanceName: instance,
		Tool:               req.Tool,
		BackupType:         req.BackupType,
		Status:             req.Status,
		StartedAt:          req.StartedAt,
		FinishedAt:         req.FinishedAt,
		DurationSeconds:    req.DurationSeconds,
		WalArchivedUntil:   req.WalArchivedUntil,
		Repo:               req.Repo,
		SizeBytes:          req.SizeBytes,
		ErrorMessage:       req.ErrorMessage,
		Metadata:           req.Metadata,
	}
	if err := h.metricsSvc.LogPostgresBackupRun(r.Context(), row); err != nil {
		log.Printf("[API] PG backup report insert error for %s: %v", instance, err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to store backup report"})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}

func (h *PostgresHandlers) BackupLatest(w http.ResponseWriter, r *http.Request) {
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
	row, err := h.metricsSvc.GetLatestPostgresBackupRun(r.Context(), instance)
	if err != nil {
		if err != pgx.ErrNoRows {
			log.Printf("[API] PG backup latest error for %s: %v", instance, err)
		}
		row = nil
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"latest":   row,
	})
}

func (h *PostgresHandlers) BackupHistory(w http.ResponseWriter, r *http.Request) {
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
	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}
	rows, err := h.metricsSvc.GetPostgresBackupRunHistory(r.Context(), instance, limit)
	if err != nil {
		log.Printf("[API] PG backup history error for %s: %v", instance, err)
		rows = []hot.PostgresBackupRunRow{}
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"history":  rows,
	})
}

type pgLogReportRequest struct {
	Instance string `json:"instance"`
	Events   []struct {
		CaptureTimestamp *time.Time             `json:"capture_timestamp,omitempty"`
		Severity         string                 `json:"severity"`
		SQLState         string                 `json:"sqlstate,omitempty"`
		Message          string                 `json:"message"`
		UserName         string                 `json:"user_name,omitempty"`
		DatabaseName     string                 `json:"database_name,omitempty"`
		ApplicationName  string                 `json:"application_name,omitempty"`
		ClientAddr       string                 `json:"client_addr,omitempty"`
		PID              int64                  `json:"pid,omitempty"`
		Context          string                 `json:"context,omitempty"`
		Detail           string                 `json:"detail,omitempty"`
		Hint             string                 `json:"hint,omitempty"`
		Raw              map[string]interface{} `json:"raw,omitempty"`
	} `json:"events"`
}
