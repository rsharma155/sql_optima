// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Engine-agnostic TimescaleDB storage index health API handlers.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/service"
	"github.com/rsharma155/sql_optima/internal/storage/hot"
)

type StorageIndexHealthTimescaleHandlers struct {
	metricsSvc *service.MetricsService
	cfg        *config.Config
}

func NewStorageIndexHealthTimescaleHandlers(svc *service.MetricsService, cfg *config.Config) *StorageIndexHealthTimescaleHandlers {
	return &StorageIndexHealthTimescaleHandlers{metricsSvc: svc, cfg: cfg}
}

func (h *StorageIndexHealthTimescaleHandlers) parseID(r *http.Request) (uuid.UUID, bool) {
	return ParseServerID(r, h.cfg)
}

func (h *StorageIndexHealthTimescaleHandlers) GetIndexUsage(w http.ResponseWriter, r *http.Request) {
	id, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")

	res, err := h.metricsSvc.TimescaleStorageIndexHealthIndexUsage(r.Context(), id, from, to)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (h *StorageIndexHealthTimescaleHandlers) GetTableUsage(w http.ResponseWriter, r *http.Request) {
	id, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")

	res, err := h.metricsSvc.TimescaleStorageIndexHealthTableUsage(r.Context(), id, from, to)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (h *StorageIndexHealthTimescaleHandlers) GetGrowth(w http.ResponseWriter, r *http.Request) {
	id, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")

	res, err := h.metricsSvc.TimescaleStorageIndexHealthGrowth(r.Context(), id, from, to)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (h *StorageIndexHealthTimescaleHandlers) GetDashboard(w http.ResponseWriter, r *http.Request) {
	id, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	
	// Optional filters
	filters := hot.SIHFilters{
		DBNames:     r.URL.Query()["db"],
		SchemaNames: r.URL.Query()["schema"],
		TableLike:   r.URL.Query().Get("table"),
	}

	res, err := h.metricsSvc.TimescaleStorageIndexHealthDashboard(r.Context(), id, from, to, filters)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (h *StorageIndexHealthTimescaleHandlers) GetFilterOptions(w http.ResponseWriter, r *http.Request) {
	id, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	db := r.URL.Query().Get("db")
	schema := r.URL.Query().Get("schema")

	res, err := h.metricsSvc.TimescaleStorageIndexHealthFilterOptions(r.Context(), id, from, to, db, schema)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (h *StorageIndexHealthTimescaleHandlers) GetIndexDefinition(w http.ResponseWriter, r *http.Request) {
	id, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")

	res, err := h.metricsSvc.TimescaleStorageIndexDefinition(r.Context(), id, from, to)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (h *StorageIndexHealthTimescaleHandlers) GetIndexUsageTrend(w http.ResponseWriter, r *http.Request) {
	id, ok := h.parseID(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")

	res, err := h.metricsSvc.TimescaleStorageIndexHealthIndexUsage(r.Context(), id, from, to)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}
