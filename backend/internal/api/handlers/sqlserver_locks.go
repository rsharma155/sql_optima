// SQL Optima — https://github.com/rsharma155/sql_optima
//
// File: sqlserver_locks.go
// Purpose: HTTP handlers for SQL Server blocking and deadlock analysis endpoints.
// Metadata:
//   - Version: 1.1
//   - Package: handlers
//   - Targets: Blocking KPIs, Timeline, Details, Deadlocks
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"time"
)

// BlockingKPIs returns current blocking KPIs for SQL Server
func (h *SqlServerHandlers) BlockingKPIs(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")
	if instance == "" {
		http.Error(w, "instance required", http.StatusBadRequest)
		return
	}

	kpis, err := h.metricsSvc.GetSQLServerBlockingKPIs(r.Context(), instance)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(kpis)
}

// BlockingTimeline returns blocking timeline data
func (h *SqlServerHandlers) BlockingTimeline(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")
	if instance == "" {
		http.Error(w, "instance required", http.StatusBadRequest)
		return
	}

	from, to := h.parseTimeRange(r)
	timeline, err := h.metricsSvc.GetSQLServerBlockingTimeline(r.Context(), instance, from, to)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(timeline)
}

// BlockingDetails returns detailed blocking snapshots
func (h *SqlServerHandlers) BlockingDetails(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")
	if instance == "" {
		http.Error(w, "instance required", http.StatusBadRequest)
		return
	}

	from, to := h.parseTimeRange(r)
	details, err := h.metricsSvc.GetSQLServerBlockingDetails(r.Context(), instance, from, to)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(details)
}

// BlockingLocks returns detailed lock snapshots
func (h *SqlServerHandlers) BlockingLocks(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")
	if instance == "" {
		http.Error(w, "instance required", http.StatusBadRequest)
		return
	}

	from, to := h.parseTimeRange(r)
	locks, err := h.metricsSvc.GetSQLServerBlockingLocks(r.Context(), instance, from, to)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(locks)
}

// TopBlockingQueries returns top blocking queries
func (h *SqlServerHandlers) TopBlockingQueries(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")
	if instance == "" {
		http.Error(w, "instance required", http.StatusBadRequest)
		return
	}

	from, to := h.parseTimeRange(r)
	queries, err := h.metricsSvc.GetSQLServerTopBlockingQueries(r.Context(), instance, from, to)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(queries)
}

// MostBlockedDatabases returns databases with most blocking
func (h *SqlServerHandlers) MostBlockedDatabases(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")
	if instance == "" {
		http.Error(w, "instance required", http.StatusBadRequest)
		return
	}

	from, to := h.parseTimeRange(r)
	dbs, err := h.metricsSvc.GetSQLServerMostBlockedDatabases(r.Context(), instance, from, to)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(dbs)
}

// MostBlockedObjects returns objects with most blocking
func (h *SqlServerHandlers) MostBlockedObjects(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")
	if instance == "" {
		http.Error(w, "instance required", http.StatusBadRequest)
		return
	}

	from, to := h.parseTimeRange(r)
	objs, err := h.metricsSvc.GetSQLServerMostBlockedObjects(r.Context(), instance, from, to)
	if err != nil {
		log.Printf("[SqlServerHandlers] MostBlockedObjects error for %s: %v", instance, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(objs)
}

// DeadlockHistory returns deadlock history
func (h *SqlServerHandlers) DeadlockHistory(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")
	if instance == "" {
		http.Error(w, "instance required", http.StatusBadRequest)
		return
	}

	enabled, msg := h.metricsSvc.GetDeadlockXESessionStatus(instance)

	from, to := h.parseTimeRange(r)
	history, err := h.metricsSvc.GetSQLServerDeadlockHistory(r.Context(), instance, from, to)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"enabled": enabled,
		"message": msg,
		"events":  history,
	})
}

// BlockingRecurrence returns historical blocking for a SQL hash or login
func (h *SqlServerHandlers) BlockingRecurrence(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")
	sqlHash := r.URL.Query().Get("sql_hash")
	loginName := r.URL.Query().Get("login_name")

	if instance == "" {
		http.Error(w, "instance required", http.StatusBadRequest)
		return
	}

	recurrence, err := h.metricsSvc.GetSQLServerBlockingRecurrence(r.Context(), instance, sqlHash, loginName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(recurrence)
}

func (h *SqlServerHandlers) parseTimeRange(r *http.Request) (time.Time, time.Time) {
	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")
	
	now := time.Now().UTC()
	from := now.Add(-1 * time.Hour)
	to := now

	if fromStr != "" {
		if t, err := parseTimeFlexible(fromStr); err == nil {
			from = t.UTC()
		}
	}
	if toStr != "" {
		if t, err := parseTimeFlexible(toStr); err == nil {
			to = t.UTC()
		}
	}
	return from, to
}

func parseTimeFlexible(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339, s)
}
