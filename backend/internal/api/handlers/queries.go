// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: SQL Server query-related API handlers (active queries, Query Store).
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/rsharma155/sql_optima/internal/service"
)

type QueryHandlers struct {
	metricsSvc *service.MetricsService
}

func NewQueryHandlers(svc *service.MetricsService) *QueryHandlers {
	return &QueryHandlers{metricsSvc: svc}
}

func (h *QueryHandlers) GetActiveQueries(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"queries": []interface{}{}})
}

func (h *QueryHandlers) GetQueryBottlenecks(w http.ResponseWriter, r *http.Request) {
	idStr := mux.Vars(r)["id"]
	id, err := uuid.Parse(idStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	start := r.URL.Query().Get("from")
	end := r.URL.Query().Get("to")

	res, err := h.metricsSvc.GetQueryBottlenecksWithRange(r.Context(), id, start, end)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (h *QueryHandlers) GetSQLText(w http.ResponseWriter, r *http.Request) {
	idStr := mux.Vars(r)["id"]
	id, err := uuid.Parse(idStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	dbName := r.URL.Query().Get("database")
	sqlHash := r.URL.Query().Get("sql_hash")

	res, err := h.metricsSvc.GetSqlServerQueryStoreSQLText(r.Context(), id, dbName, sqlHash)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"sql_text": res})
}

func (h *QueryHandlers) GetProcedureStats(w http.ResponseWriter, r *http.Request) {
	idStr := mux.Vars(r)["id"]
	id, err := uuid.Parse(idStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	_ = id
	_ = json.NewEncoder(w).Encode([]interface{}{})
}
