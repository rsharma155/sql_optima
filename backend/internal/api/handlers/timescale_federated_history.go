// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Hot+cold federated dashboard series for memory / wait / connection history.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"
)

func (h *TimescaleHandlers) SqlServerMemoryHistory(w http.ResponseWriter, r *http.Request) {
	logger := h.metricsSvc.GetTimescaleDBLogger()
	if logger == nil {
		http.Error(w, "timescale unavailable", http.StatusServiceUnavailable)
		return
	}
	h.serveFederatedSeries(w, r, "sqlserver_memory_history", "capture_timestamp_ms",
		func(ctx context.Context, id uuid.UUID, from, to string) ([]map[string]interface{}, error) {
			return logger.GetSQLServerMemoryHistory(ctx, id, from, to, 500)
		},
		func(result *coldQueryResponse) []map[string]interface{} {
			return mapColdRowsGeneric(result, func(get func(...string) interface{}, ts time.Time) map[string]interface{} {
				return map[string]interface{}{
					"timestamp":            ts,
					"page_life_expectancy": toFloat(get("page_life_expectancy")),
				}
			})
		},
		"failed to load memory history",
	)
}

func (h *TimescaleHandlers) SqlServerWaitHistory(w http.ResponseWriter, r *http.Request) {
	logger := h.metricsSvc.GetTimescaleDBLogger()
	if logger == nil {
		http.Error(w, "timescale unavailable", http.StatusServiceUnavailable)
		return
	}
	h.serveFederatedSeries(w, r, "sqlserver_wait_history", "capture_timestamp_ms",
		func(ctx context.Context, id uuid.UUID, from, to string) ([]map[string]interface{}, error) {
			return logger.GetSQLServerWaitHistoryRange(ctx, id, from, to, 500)
		},
		func(result *coldQueryResponse) []map[string]interface{} {
			return mapColdRowsGeneric(result, func(get func(...string) interface{}, ts time.Time) map[string]interface{} {
				return map[string]interface{}{
					"timestamp":              ts,
					"disk_read_ms_per_sec":   toFloat(get("disk_read_ms_per_sec")),
					"blocking_ms_per_sec":    toFloat(get("blocking_ms_per_sec")),
					"parallelism_ms_per_sec": toFloat(get("parallelism_ms_per_sec")),
					"other_ms_per_sec":       toFloat(get("other_ms_per_sec")),
				}
			})
		},
		"failed to load wait history",
	)
}

func (h *TimescaleHandlers) SqlServerConnectionHistory(w http.ResponseWriter, r *http.Request) {
	logger := h.metricsSvc.GetTimescaleDBLogger()
	if logger == nil {
		http.Error(w, "timescale unavailable", http.StatusServiceUnavailable)
		return
	}
	h.serveFederatedSeries(w, r, "sqlserver_connection_history", "capture_timestamp_ms",
		func(ctx context.Context, id uuid.UUID, from, to string) ([]map[string]interface{}, error) {
			return logger.GetSQLServerConnectionHistoryRange(ctx, id, from, to, 500)
		},
		func(result *coldQueryResponse) []map[string]interface{} {
			return mapColdRowsGeneric(result, func(get func(...string) interface{}, ts time.Time) map[string]interface{} {
				return map[string]interface{}{
					"timestamp":          ts,
					"login_name":         stringify(get("login_name")),
					"database_name":      stringify(get("database_name")),
					"active_connections": int(toFloat(get("active_connections"))),
					"active_requests":    int(toFloat(get("active_requests"))),
				}
			})
		},
		"failed to load connection history",
	)
}

func stringify(v interface{}) string {
	switch s := v.(type) {
	case string:
		return s
	case []byte:
		return string(s)
	default:
		return ""
	}
}
