// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL CPU performance and system stats API handlers.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/rsharma155/sql_optima/internal/service"
)

type PgCpuHandlers struct {
	metricsSvc *service.MetricsService
}

func NewPgCpuHandlers(svc *service.MetricsService) *PgCpuHandlers {
	return &PgCpuHandlers{metricsSvc: svc}
}

func (h *PgCpuHandlers) GetCpuHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode([]interface{}{})
}

func (h *PgCpuHandlers) GetCpuSaturation(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{})
}
