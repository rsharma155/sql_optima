// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Alert API handlers for listing and lifecycle management.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/domain/alerts"
	"github.com/rsharma155/sql_optima/internal/middleware"
	"github.com/rsharma155/sql_optima/internal/service"
)

type AlertHandlers struct {
	svc *service.AlertService
	cfg *config.Config
}

func NewAlertHandlers(svc *service.AlertService, cfg *config.Config) *AlertHandlers {
	return &AlertHandlers{svc: svc, cfg: cfg}
}

func (h *AlertHandlers) ListAlerts(w http.ResponseWriter, r *http.Request) {
	filter := alerts.AlertFilter{}

	if sid, ok := ParseServerID(r, h.cfg); ok && sid != uuid.Nil {
		filter.ServerID = &sid
	}
	if v := r.URL.Query().Get("engine"); v != "" {
		e := alerts.Engine(v)
		filter.Engine = &e
	}
	if v := r.URL.Query().Get("severity"); v != "" {
		s := alerts.Severity(v)
		filter.Severity = &s
	}
	if v := r.URL.Query().Get("status"); v != "" {
		s := alerts.Status(v)
		filter.Status = &s
	}
	if v := r.URL.Query().Get("category"); v != "" {
		filter.Category = &v
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		if l, err := strconv.Atoi(v); err == nil {
			filter.Limit = l
		}
	}

	list, err := h.svc.List(r.Context(), filter)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"list":  list,
			"total": len(list),
		},
	})
}

func (h *AlertHandlers) GetAlert(w http.ResponseWriter, r *http.Request) {
	idStr := mux.Vars(r)["id"]
	id, err := uuid.Parse(idStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	a, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(a)
}

func (h *AlertHandlers) CountOpen(w http.ResponseWriter, r *http.Request) {
	sid, ok := ParseServerID(r, h.cfg)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	count, err := h.svc.CountOpen(r.Context(), sid)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]int{"count": count})
}

func (h *AlertHandlers) AcknowledgeAlert(w http.ResponseWriter, r *http.Request) {
	idStr := mux.Vars(r)["id"]
	id, err := uuid.Parse(idStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var req struct {
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	actor := "system"
	if claims := middleware.GetAuthClaims(r); claims != nil && claims.Username != "" {
		actor = claims.Username
	}

	if err := h.svc.Acknowledge(r.Context(), id, actor, req.Reason); err != nil {
		if err == alerts.ErrAlertNotFound {
			w.WriteHeader(http.StatusNotFound)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AlertHandlers) ResolveAlert(w http.ResponseWriter, r *http.Request) {
	idStr := mux.Vars(r)["id"]
	id, err := uuid.Parse(idStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var req struct {
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	actor := "system"
	if claims := middleware.GetAuthClaims(r); claims != nil && claims.Username != "" {
		actor = claims.Username
	}

	if err := h.svc.Resolve(r.Context(), id, actor, req.Reason); err != nil {
		switch err {
		case alerts.ErrAlertAlreadyResolved:
			w.WriteHeader(http.StatusConflict)
		case alerts.ErrAlertNotFound:
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AlertHandlers) ListMaintenanceWindows(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode([]alerts.MaintenanceWindow{})
}

func (h *AlertHandlers) CreateMaintenanceWindow(w http.ResponseWriter, r *http.Request) {
	var req struct {
		InstanceName string `json:"instance_name"`
		Engine       string `json:"engine"`
		Reason       string `json:"reason"`
		StartsAt     string `json:"starts_at"`
		EndsAt       string `json:"ends_at"`
		CreatedBy    string `json:"created_by"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	eng := alerts.Engine(req.Engine)
	if eng != alerts.EngineSQLServer && eng != alerts.EnginePostgres {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "engine must be 'sqlserver' or 'postgres'"})
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (h *AlertHandlers) DeleteMaintenanceWindow(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}
