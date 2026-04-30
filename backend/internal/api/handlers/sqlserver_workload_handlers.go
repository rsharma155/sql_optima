// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: API handlers for the SQL Server Workload Observability Dashboard.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/service"
)

type SqlServerWorkloadHandlers struct {
	metricsSvc *service.MetricsService
	cfg        *config.Config
}

func NewSqlServerWorkloadHandlers(metricsSvc *service.MetricsService, cfg *config.Config) *SqlServerWorkloadHandlers {
	return &SqlServerWorkloadHandlers{metricsSvc: metricsSvc, cfg: cfg}
}

func (h *SqlServerWorkloadHandlers) Summary(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")
	from, to, err := parseWorkloadTimeRange(fromStr, toStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	summary, err := h.metricsSvc.GetSqlServerWorkloadSummary(r.Context(), instance, from, to)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(summary)
}

func (h *SqlServerWorkloadHandlers) Trends(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")
	from, to, err := parseWorkloadTimeRange(fromStr, toStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	trends, err := h.metricsSvc.GetSqlServerWorkloadTrends(r.Context(), instance, from, to)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"trends": trends})
}

func (h *SqlServerWorkloadHandlers) TopOffenders(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")
	from, to, err := parseWorkloadTimeRange(fromStr, toStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	limit := 20
	if lStr := r.URL.Query().Get("limit"); lStr != "" {
		if l, err := strconv.Atoi(lStr); err == nil && l > 0 {
			limit = l
		}
	}

	offenders, err := h.metricsSvc.GetSqlServerWorkloadTopOffenders(r.Context(), instance, from, to, limit)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"top_offenders": offenders})
}

func (h *SqlServerWorkloadHandlers) AppLoad(w http.ResponseWriter, r *http.Request) {
	h.handleTimeline(w, r, h.metricsSvc.GetSqlServerWorkloadAppLoadTimeline, "app_load")
}

func (h *SqlServerWorkloadHandlers) LoginLoad(w http.ResponseWriter, r *http.Request) {
	h.handleTimeline(w, r, h.metricsSvc.GetSqlServerWorkloadLoginLoadTimeline, "login_load")
}

func (h *SqlServerWorkloadHandlers) TopApps(w http.ResponseWriter, r *http.Request) {
	h.handleTopN(w, r, h.metricsSvc.GetSqlServerWorkloadTopApps, "top_apps")
}

func (h *SqlServerWorkloadHandlers) TopLogins(w http.ResponseWriter, r *http.Request) {
	h.handleTopN(w, r, h.metricsSvc.GetSqlServerWorkloadTopLogins, "top_logins")
}

func (h *SqlServerWorkloadHandlers) handleTimeline(w http.ResponseWriter, r *http.Request, fn func(context.Context, string, time.Time, time.Time) ([]map[string]interface{}, error), key string) {
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")
	from, to, err := parseWorkloadTimeRange(fromStr, toStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	data, err := fn(r.Context(), instance, from, to)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{key: data})
}

func (h *SqlServerWorkloadHandlers) handleTopN(w http.ResponseWriter, r *http.Request, fn func(context.Context, string, time.Time, time.Time, int) ([]map[string]interface{}, error), key string) {
	instance := r.URL.Query().Get("instance")
	if err := validateInstanceName(instance); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")
	from, to, err := parseWorkloadTimeRange(fromStr, toStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	limit := 10
	if lStr := r.URL.Query().Get("limit"); lStr != "" {
		if l, err := strconv.Atoi(lStr); err == nil && l > 0 {
			limit = l
		}
	}

	data, err := fn(r.Context(), instance, from, to, limit)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{key: data})
}

func parseWorkloadTimeRange(fromStr, toStr string) (time.Time, time.Time, error) {
	var from, to time.Time
	var err error

	if fromStr == "" {
		from = time.Now().UTC().Add(-1 * time.Hour)
	} else {
		from, err = time.Parse(time.RFC3339, fromStr)
		if err != nil {
			// Try without Z or fractional seconds if needed, but browsers usually send correct RFC3339
			from, err = time.Parse("2006-01-02T15:04:05", strings.Split(fromStr, ".")[0])
			if err != nil {
				return time.Time{}, time.Time{}, fmt.Errorf("invalid from time: %w", err)
			}
		}
	}

	if toStr == "" {
		to = time.Now().UTC()
	} else {
		to, err = time.Parse(time.RFC3339, toStr)
		if err != nil {
			to, err = time.Parse("2006-01-02T15:04:05", strings.Split(toStr, ".")[0])
			if err != nil {
				return time.Time{}, time.Time{}, fmt.Errorf("invalid to time: %w", err)
			}
		}
	}

	return from, to, nil
}
