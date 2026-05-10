// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: HTTP handlers for the SQL Server Watched Query Analyzer — CRUD,
//
//	time-series snapshots, plan comparison, wait stats, and event markers.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/models"
	"github.com/rsharma155/sql_optima/internal/service"
)

// SqlServerWatchedQueryHandlers groups HTTP handlers for the Watched Query Analyzer.
type SqlServerWatchedQueryHandlers struct {
	metricsSvc *service.MetricsService
	cfg        *config.Config
}

// NewSqlServerWatchedQueryHandlers constructs a new handler set.
func NewSqlServerWatchedQueryHandlers(metricsSvc *service.MetricsService, cfg *config.Config) *SqlServerWatchedQueryHandlers {
	return &SqlServerWatchedQueryHandlers{metricsSvc: metricsSvc, cfg: cfg}
}

// List returns all watched queries for an instance.
func (h *SqlServerWatchedQueryHandlers) List(w http.ResponseWriter, r *http.Request) {
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

	wqs, err := h.metricsSvc.ListSqlServerWatchedQueries(r.Context(), instance)
	if err != nil {
		slog.Error("sqlserver_watched_queries_list", "instance", instance, "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance":        instance,
		"watched_queries": wqs,
	})
}

// Add adds a query to the watch list (POST body: {query_hash, name, query_text}).
func (h *SqlServerWatchedQueryHandlers) Add(w http.ResponseWriter, r *http.Request) {
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

	var body struct {
		QueryHash    string `json:"query_hash"`
		Name         string `json:"name"`
		QueryText    string `json:"query_text"`
		DatabaseName string `json:"database_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
		return
	}
	if body.Name == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "name is required"})
		return
	}
	if body.QueryHash == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "query_hash is required"})
		return
	}

	id, err := h.metricsSvc.AddSqlServerWatchedQuery(r.Context(), instance, models.SqlServerWatchedQuery{
		QueryHash:    body.QueryHash,
		Name:         body.Name,
		QueryText:    body.QueryText,
		DatabaseName: body.DatabaseName,
	})
	if err != nil {
		slog.Error("sqlserver_watched_query_add", "instance", instance, "error", err)

		errMsg := err.Error()
		if strings.Contains(errMsg, "maximum") {
			w.WriteHeader(http.StatusForbidden)
		} else if strings.Contains(errMsg, "23505") || strings.Contains(errMsg, "unique constraint") {
			w.WriteHeader(http.StatusConflict)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}

		json.NewEncoder(w).Encode(map[string]string{"error": errMsg})
		return
	}
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":       id,
		"instance": instance,
	})
}

// Delete removes a watched query by ID (?id=N).
func (h *SqlServerWatchedQueryHandlers) Delete(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	idStr := r.URL.Query().Get("id")
	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "valid id is required"})
		return
	}

	if err := h.metricsSvc.DeleteSqlServerWatchedQuery(r.Context(), id); err != nil {
		slog.Error("sqlserver_watched_query_delete", "id", id, "error", err)
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
}

// Detail returns a watched query with snapshots, events, plan info, and wait stats.
func (h *SqlServerWatchedQueryHandlers) Detail(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	idStr := r.URL.Query().Get("id")
	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "valid id is required"})
		return
	}

	from, to := parseTimeRange(r)

	wq, snaps, events, err := h.metricsSvc.GetSqlServerWatchedQueryDetail(r.Context(), id, from, to)
	if err != nil {
		slog.Error("sqlserver_watched_query_detail", "id", id, "error", err)
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"watched_query": wq,
		"snapshots":     snaps,
		"events":        events,
	})
}

// AddEvent records an optimization event marker (POST body: {event_type, notes}).
func (h *SqlServerWatchedQueryHandlers) AddEvent(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	idStr := r.URL.Query().Get("id")
	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "valid id is required"})
		return
	}

	var body struct {
		EventType string `json:"event_type"`
		Notes     string `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
		return
	}
	if body.EventType == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "event_type is required"})
		return
	}

	if err := h.metricsSvc.AddSqlServerWatchedQueryEvent(r.Context(), id, body.EventType, body.Notes); err != nil {
		slog.Error("sqlserver_watched_query_add_event", "id", id, "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"status": "created"})
}
