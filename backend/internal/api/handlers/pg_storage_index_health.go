// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL Storage and Index Health API handlers.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/rsharma155/sql_optima/internal/apiresponse"
	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/service"
	"github.com/rsharma155/sql_optima/internal/storage/hot"
)

type PgStorageIndexHealthHandlers struct {
	metricsSvc *service.MetricsService
	cfg        *config.Config
}

func NewPgStorageIndexHealthHandlers(svc *service.MetricsService, cfg *config.Config) *PgStorageIndexHealthHandlers {
	return &PgStorageIndexHealthHandlers{metricsSvc: svc, cfg: cfg}
}

func (h *PgStorageIndexHealthHandlers) parseID(r *http.Request) (uuid.UUID, bool) {
	return ParseServerID(r, h.cfg)
}

func (h *PgStorageIndexHealthHandlers) GetIndexUsage(w http.ResponseWriter, r *http.Request) {
	id, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance name or server_id required"})
		return
	}

	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	db := r.URL.Query().Get("db")
	schema := r.URL.Query().Get("schema")
	table := r.URL.Query().Get("table")

	res, err := h.metricsSvc.TimescaleStorageIndexHealthIndexUsage(r.Context(), id, from, to, db, schema, table)
	if err != nil {
		apiresponse.WriteJSONError(w, http.StatusInternalServerError, "failed to load storage index health", err, "handler", "pg_storage_index")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (h *PgStorageIndexHealthHandlers) GetTableUsage(w http.ResponseWriter, r *http.Request) {
	id, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance name or server_id required"})
		return
	}

	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")

	res, err := h.metricsSvc.TimescaleStorageIndexHealthTableUsage(r.Context(), id, from, to)
	if err != nil {
		apiresponse.WriteJSONError(w, http.StatusInternalServerError, "failed to load storage index health", err, "handler", "pg_storage_index")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (h *PgStorageIndexHealthHandlers) GetGrowth(w http.ResponseWriter, r *http.Request) {
	id, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance name or server_id required"})
		return
	}

	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")

	res, err := h.metricsSvc.TimescaleStorageIndexHealthGrowth(r.Context(), id, from, to)
	if err != nil {
		apiresponse.WriteJSONError(w, http.StatusInternalServerError, "failed to load storage index health", err, "handler", "pg_storage_index")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (h *PgStorageIndexHealthHandlers) GetDashboard(w http.ResponseWriter, r *http.Request) {
	id, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance name or server_id required"})
		return
	}

	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	filters := hot.SIHFilters{
		DBNames:     r.URL.Query()["db"],
		SchemaNames: r.URL.Query()["schema"],
		TableLike:   r.URL.Query().Get("table"),
	}

	res, err := h.metricsSvc.TimescaleStorageIndexHealthDashboard(r.Context(), id, from, to, filters)
	if err != nil {
		apiresponse.WriteJSONError(w, http.StatusInternalServerError, "failed to load storage index health", err, "handler", "pg_storage_index")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (h *PgStorageIndexHealthHandlers) GetFilterOptions(w http.ResponseWriter, r *http.Request) {
	id, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance name or server_id required"})
		return
	}

	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	db := r.URL.Query().Get("db")
	schema := r.URL.Query().Get("schema")

	res, err := h.metricsSvc.TimescaleStorageIndexHealthFilterOptions(r.Context(), id, from, to, db, schema)
	if err != nil {
		apiresponse.WriteJSONError(w, http.StatusInternalServerError, "failed to load storage index health", err, "handler", "pg_storage_index")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (h *PgStorageIndexHealthHandlers) GetIndexDefinition(w http.ResponseWriter, r *http.Request) {
	id, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "instance name or server_id required"})
		return
	}

	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")

	res, err := h.metricsSvc.TimescaleStorageIndexDefinition(r.Context(), id, from, to)
	if err != nil {
		apiresponse.WriteJSONError(w, http.StatusInternalServerError, "failed to load storage index health", err, "handler", "pg_storage_index")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}
