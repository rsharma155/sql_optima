// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: HTTP handlers for PostgreSQL locks, deadlocks, and blocking analysis.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rsharma155/sql_optima/internal/storage/hot"
)


func (h *PostgresHandlers) DeadlocksHistory(w http.ResponseWriter, r *http.Request) {
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
	limit := 180
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 && n <= 5000 {
			limit = n
		}
	}
	minutes := 180
	if s := r.URL.Query().Get("window_minutes"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 && n <= 10080 {
			minutes = n
		}
	}
	if !h.metricsSvc.IsTimescaleConnected() {
		json.NewEncoder(w).Encode(map[string]interface{}{"instance": instance, "history": map[string]interface{}{}, "source": "none"})
		return
	}
	rows, err := h.metricsSvc.GetPostgresDeadlocksHistory(r.Context(), instance, minutes, limit)
	if err != nil {
		rows = []hot.PostgresDeadlockStatRow{}
	}
	// Aggregate deltas across DBs per timestamp.
	byTs := map[string]int64{}
	for _, r := range rows {
		k := r.CaptureTimestamp.UTC().Format(time.RFC3339)
		byTs[k] += r.DeadlocksDelta
	}
	// Build ascending series.
	type kv struct {
		ts string
		v  int64
	}
	var xs []kv
	for ts, v := range byTs {
		xs = append(xs, kv{ts: ts, v: v})
	}
	sort.Slice(xs, func(i, j int) bool { return xs[i].ts < xs[j].ts })
	labels := make([]string, 0, len(xs))
	deltas := make([]int64, 0, len(xs))
	for _, x := range xs {
		labels = append(labels, x.ts)
		deltas = append(deltas, x.v)
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"history": map[string]interface{}{
			"labels":          labels,
			"deadlocks_delta": deltas,
		},
		"source": "timescale",
	})
}

func (h *PostgresHandlers) LockWaitHistory(w http.ResponseWriter, r *http.Request) {
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
	limit := 400
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 && n <= 5000 {
			limit = n
		}
	}
	minutes := 180
	if s := r.URL.Query().Get("window_minutes"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 && n <= 10080 {
			minutes = n
		}
	}
	if !h.metricsSvc.IsTimescaleConnected() {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"instance": instance,
			"history":  map[string]interface{}{},
			"source":   "none",
		})
		return
	}
	labels, counts, err := h.metricsSvc.GetPostgresLockWaitHistory(r.Context(), instance, minutes, limit)
	if err != nil {
		log.Printf("[API] PG lock-wait history error for %s: %v", instance, err)
		labels, counts = nil, nil
	}
	if labels == nil {
		labels = []string{}
	}
	if counts == nil {
		counts = []int{}
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"history": map[string]interface{}{
			"labels":                labels,
			"lock_waiting_sessions": counts,
		},
		"source": "timescale",
	})
}

func (h *PostgresHandlers) LocksBlockingKPIs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if !instanceExists(r.Context(), h.cfg, h.metricsSvc, instance) || !instanceTypeFromDB(r.Context(), h.cfg, h.metricsSvc, instance, "postgres") {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}
	if !h.metricsSvc.IsTimescaleConnected() {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"instance": instance,
			"kpis":     map[string]interface{}{},
			"source":   "none",
		})
		return
	}
	k, err := h.metricsSvc.GetPgLocksBlockingKpis(r.Context(), instance)
	if err != nil || k == nil {
		log.Printf("[API] PG locks-blocking kpis error for %s: %v", instance, err)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"instance": instance,
			"kpis":     map[string]interface{}{},
			"source":   "timescale",
		})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"kpis":     k,
		"source":   "timescale",
	})
}

func (h *PostgresHandlers) LocksBlockingTimeline(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if !instanceExists(r.Context(), h.cfg, h.metricsSvc, instance) || !instanceTypeFromDB(r.Context(), h.cfg, h.metricsSvc, instance, "postgres") {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}
	if !h.metricsSvc.IsTimescaleConnected() {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"instance":  instance,
			"timeline":  []interface{}{},
			"incidents": []interface{}{},
			"source":    "none",
		})
		return
	}

	q := r.URL.Query()
	fromStr := strings.TrimSpace(q.Get("from"))
	toStr := strings.TrimSpace(q.Get("to"))
	var points []hot.PgBlockingTimelinePoint
	var incs []hot.PgBlockingIncident
	var err error
	// If from/to provided (RFC3339), prefer range mode. Otherwise fall back to window_hours.
	if fromStr != "" && toStr != "" {
		fromT, err := time.Parse(time.RFC3339, fromStr)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid from (use RFC3339)"})
			return
		}
		toT, err := time.Parse(time.RFC3339, toStr)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid to (use RFC3339)"})
			return
		}
		points, err = h.metricsSvc.GetPgBlockingTimelineRange(r.Context(), instance, fromT, toT)
		if err != nil {
			log.Printf("[API] PG locks-blocking timeline range error for %s: %v", instance, err)
			points = []hot.PgBlockingTimelinePoint{}
		}
		incs, err = h.metricsSvc.GetPgBlockingIncidentsRange(r.Context(), instance, fromT, toT)
		if err != nil {
			log.Printf("[API] PG locks-blocking incidents range error for %s: %v", instance, err)
			incs = []hot.PgBlockingIncident{}
		}
	} else {
		windowHours := 24
		if s := q.Get("window_hours"); s != "" {
			if n, err := strconv.Atoi(s); err == nil && n > 0 && n <= 168 {
				windowHours = n
			}
		}
		window := time.Duration(windowHours) * time.Hour
		points, err = h.metricsSvc.GetPgBlockingTimeline(r.Context(), instance, window)
		if err != nil {
			log.Printf("[API] PG locks-blocking timeline error for %s: %v", instance, err)
			points = []hot.PgBlockingTimelinePoint{}
		}
		incs, err = h.metricsSvc.GetPgBlockingIncidentsInWindow(r.Context(), instance, window)
		if err != nil {
			log.Printf("[API] PG locks-blocking incidents error for %s: %v", instance, err)
			incs = []hot.PgBlockingIncident{}
		}
	}
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"instance":  instance,
		"timeline":  points,
		"incidents": incs,
		"source":    "timescale",
	})
}

func (h *PostgresHandlers) LocksBlockingTopLockedTables(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if !instanceExists(r.Context(), h.cfg, h.metricsSvc, instance) || !instanceTypeFromDB(r.Context(), h.cfg, h.metricsSvc, instance, "postgres") {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}
	limit := 10
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}
	lookbackMin := 10
	if s := r.URL.Query().Get("lookback_minutes"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n >= 1 && n <= 180 {
			lookbackMin = n
		}
	}
	if !h.metricsSvc.IsTimescaleConnected() {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"instance": instance,
			"tables":   []interface{}{},
			"source":   "none",
		})
		return
	}

	q := r.URL.Query()
	fromStr := strings.TrimSpace(q.Get("from"))
	toStr := strings.TrimSpace(q.Get("to"))
	var rows []hot.PgTopLockedTable
	if fromStr != "" && toStr != "" {
		fromT, err := time.Parse(time.RFC3339, fromStr)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid from (use RFC3339)"})
			return
		}
		toT, err := time.Parse(time.RFC3339, toStr)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid to (use RFC3339)"})
			return
		}
		rows, err = h.metricsSvc.GetPgTopLockedTablesRange(r.Context(), instance, fromT, toT, limit)
		if err != nil {
			log.Printf("[API] PG top locked tables range error for %s: %v", instance, err)
			rows = []hot.PgTopLockedTable{}
		}
	} else {
		var err error
		rows, err = h.metricsSvc.GetPgTopLockedTables(r.Context(), instance, time.Duration(lookbackMin)*time.Minute, limit)
		if err != nil {
			log.Printf("[API] PG top locked tables error for %s: %v", instance, err)
			rows = []hot.PgTopLockedTable{}
		}
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"tables":   rows,
		"source":   "timescale",
	})
}

func (h *PostgresHandlers) LocksBlockingDetails(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if !instanceExists(r.Context(), h.cfg, h.metricsSvc, instance) || !instanceTypeFromDB(r.Context(), h.cfg, h.metricsSvc, instance, "postgres") {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}
	if !h.metricsSvc.IsTimescaleConnected() {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"instance":      instance,
			"collected_at":  "",
			"blocking_tree": []interface{}{},
			"source":        "none",
		})
		return
	}

	q := r.URL.Query()
	fromStr := strings.TrimSpace(q.Get("from"))
	toStr := strings.TrimSpace(q.Get("to"))
	if fromStr == "" || toStr == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "from and to are required (RFC3339)"})
		return
	}
	fromT, err := time.Parse(time.RFC3339, fromStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid from (use RFC3339)"})
		return
	}
	toT, err := time.Parse(time.RFC3339, toStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid to (use RFC3339)"})
		return
	}

	resp, err := h.metricsSvc.GetPgBlockingDetailsInRange(r.Context(), instance, fromT, toT)
	if err != nil || resp == nil {
		log.Printf("[API] PG locks-blocking details error for %s: %v", instance, err)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"instance":      instance,
			"collected_at":  "",
			"blocking_tree": []interface{}{},
			"source":        "timescale",
		})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"instance":      instance,
		"collected_at":  resp.CollectedAt,
		"blocking_tree": resp.BlockingTree,
		"source":        "timescale",
	})
}

func (h *PostgresHandlers) Locks(w http.ResponseWriter, r *http.Request) {
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

	locks, err := h.metricsSvc.GetLatestPostgresLocks(r.Context(), instance)
	if err != nil {
		log.Printf("[API] PG locks (timescale) error for %s: %v", instance, err)
		json.NewEncoder(w).Encode(map[string]interface{}{"locks": []interface{}{}, "error": err.Error()})
		return
	}

	w.Header().Set("X-Data-Source", "timescale")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance": instance,
		"locks":    locks,
	})
}


func (h *PostgresHandlers) BlockingTree(w http.ResponseWriter, r *http.Request) {
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

	tree, err := h.metricsSvc.GetLatestPostgresBlockingTree(r.Context(), instance)
	if err != nil {
		log.Printf("[API] PG blocking-tree (timescale) error for %s: %v", instance, err)
		json.NewEncoder(w).Encode(map[string]interface{}{"blocking_tree": []interface{}{}, "error": err.Error()})
		return
	}

	w.Header().Set("X-Data-Source", "timescale")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance":      instance,
		"blocking_tree": tree,
	})
}

