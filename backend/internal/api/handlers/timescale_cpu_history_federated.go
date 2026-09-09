// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Hot+cold federation for SQL Server CPU history dashboard reads.
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

func (h *TimescaleHandlers) SqlServerCPUHistory(w http.ResponseWriter, r *http.Request) {
	logger := h.metricsSvc.GetTimescaleDBLogger()
	if logger == nil {
		http.Error(w, "timescale unavailable", http.StatusServiceUnavailable)
		return
	}
	h.serveFederatedSeries(w, r, "sqlserver_cpu_history", "capture_timestamp_ms",
		func(ctx context.Context, id uuid.UUID, from, to string) ([]map[string]interface{}, error) {
			return logger.GetSQLServerCPUHistory(ctx, id, from, to, 500)
		},
		func(result *coldQueryResponse) []map[string]interface{} {
			return mapColdRowsGeneric(result, func(get func(...string) interface{}, ts time.Time) map[string]interface{} {
				return map[string]interface{}{
					"timestamp":     ts,
					"sql_process":   toFloat(get("sql_cpu_utilization", "sql_process")),
					"system_idle":   toFloat(get("idle_cpu", "system_idle")),
					"other_process": toFloat(get("system_cpu_utilization", "other_process")),
				}
			})
		},
		"failed to load cpu history",
	)
}
