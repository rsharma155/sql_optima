// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Query bottleneck analysis handlers providing query performance issues and recommendations.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/service"
)

type QueryHandlers struct {
	metricsSvc *service.MetricsService
	cfg        *config.Config
}

func NewQueryHandlers(metricsSvc *service.MetricsService, cfg *config.Config) *QueryHandlers {
	return &QueryHandlers{metricsSvc: metricsSvc, cfg: cfg}
}

func (h *QueryHandlers) Bottlenecks(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")
	timeRange := r.URL.Query().Get("time_range")
	limitStr := r.URL.Query().Get("limit")

	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}

	limit := 50
	if limitStr != "" {
		if l, err := parseInt(limitStr); err == nil && l > 0 && l <= 200 {
			limit = l
		}
	}

	if timeRange == "" {
		timeRange = "1h"
	}
	switch timeRange {
	case "15m", "1h", "6h", "24h", "7d":
		// ok
	default:
		timeRange = "1h"
	}

	database := strings.TrimSpace(r.URL.Query().Get("database"))

	bottlenecks, err := h.metricsSvc.GetQueryBottlenecksWithRange(instance, timeRange, limit, database)
	if err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "does not exist") || strings.Contains(errStr, "relation") {
			log.Printf("[API] Query store table not initialized for %s: %v", instance, err)
			bottlenecks = []map[string]interface{}{}
		} else {
			log.Printf("[API] Query bottlenecks error for %s: %v", instance, err)
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": errStr})
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if h.metricsSvc.IsTimescaleConnected() {
		w.Header().Set("X-Data-Source", "timescale")
	} else {
		w.Header().Set("X-Data-Source", "live_cache_fallback")
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"bottlenecks": bottlenecks,
		"count":       len(bottlenecks),
		"time_range":  timeRange,
	})
}

// QueryStoreSQLText returns the full query text for a Query Store row (drill-down).
func (h *QueryHandlers) QueryStoreSQLText(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")
	database := strings.TrimSpace(r.URL.Query().Get("database"))
	queryHash := strings.TrimSpace(r.URL.Query().Get("query_hash"))

	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if !instanceInConfig(h.cfg, instance) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance not found"})
		return
	}
	if database == "" || queryHash == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "database and query_hash are required"})
		return
	}

	txt, err := h.metricsSvc.GetMssqlQueryStoreSQLText(r.Context(), instance, database, queryHash)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"instance":    instance,
		"database":    database,
		"query_hash":  queryHash,
		"query_text":  txt,
		"text_length": len(txt),
	})
}

func parseInt(s string) (int, error) {
	result := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, &parseError{s}
		}
		result = result*10 + int(c-'0')
	}
	return result, nil
}

type parseError struct {
	s string
}

func (e *parseError) Error() string {
	return "invalid number: " + e.s
}
