// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: HTTP handlers for alert engine endpoints – list, get, acknowledge,
//
//	resolve, open count, and maintenance window CRUD.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"

	"github.com/rsharma155/sql_optima/internal/domain/alerts"
	"github.com/rsharma155/sql_optima/internal/middleware"
	"github.com/rsharma155/sql_optima/internal/service"
)

// AlertHandlers provides HTTP handlers for the alert engine API.
type AlertHandlers struct {
	alertSvc  *service.AlertService
	alertRepo alerts.AlertStore
	maintRepo alerts.MaintenanceStore
}

func NewAlertHandlers(
	alertSvc *service.AlertService,
	alertRepo alerts.AlertStore,
	maintRepo alerts.MaintenanceStore,
) *AlertHandlers {
	return &AlertHandlers{
		alertSvc:  alertSvc,
		alertRepo: alertRepo,
		maintRepo: maintRepo,
	}
}

// ── List alerts ────────────────────────────────────────────────

func (h *AlertHandlers) ListAlerts(w http.ResponseWriter, r *http.Request) {
	f := alerts.AlertFilter{
		InstanceName: r.URL.Query().Get("instance"),
		Category:     r.URL.Query().Get("category"),
	}
	if v := r.URL.Query().Get("engine"); v != "" {
		f.Engine = alerts.Engine(v)
	}
	if v := r.URL.Query().Get("severity"); v != "" {
		f.Severity = alerts.Severity(v)
	}
	if v := r.URL.Query().Get("status"); v != "" {
		f.Status = alerts.Status(v)
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			f.Limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			f.Offset = n
		}
	}

	items, total, err := h.alertRepo.List(r.Context(), f)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "failed to list alerts")
		return
	}

	jsonSuccess(w, map[string]interface{}{
		"alerts": items,
		"total":  total,
	})
}

// ── Get single alert ───────────────────────────────────────────

func (h *AlertHandlers) GetAlert(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid alert id")
		return
	}

	a, err := h.alertRepo.GetByID(r.Context(), id)
	if err != nil {
		if err == alerts.ErrAlertNotFound {
			jsonError(w, http.StatusNotFound, "alert not found")
			return
		}
		jsonError(w, http.StatusInternalServerError, "failed to get alert")
		return
	}

	jsonSuccess(w, a)
}

// ── Acknowledge ────────────────────────────────────────────────

func (h *AlertHandlers) AcknowledgeAlert(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var body struct {
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	actor := actorFromRequest(r)

	if err := h.alertSvc.Acknowledge(r.Context(), id, actor, body.Reason); err != nil {
		writeAlertError(w, err, "failed to acknowledge alert")
		return
	}

	jsonSuccess(w, map[string]string{"status": "acknowledged"})
}

// ── Resolve ────────────────────────────────────────────────────

func (h *AlertHandlers) ResolveAlert(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var body struct {
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	actor := actorFromRequest(r)

	if err := h.alertSvc.Resolve(r.Context(), id, actor, body.Reason); err != nil {
		writeAlertError(w, err, "failed to resolve alert")
		return
	}

	jsonSuccess(w, map[string]string{"status": "resolved"})
}

// ── Count open alerts ──────────────────────────────────────────

func (h *AlertHandlers) CountOpen(w http.ResponseWriter, r *http.Request) {
	instance := r.URL.Query().Get("instance")
	if instance == "" {
		jsonError(w, http.StatusBadRequest, "instance parameter is required")
		return
	}

	count, err := h.alertRepo.CountOpen(r.Context(), instance)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "failed to count alerts")
		return
	}

	jsonSuccess(w, map[string]interface{}{
		"instance":   instance,
		"open_count": count,
	})
}

// ── Maintenance windows ────────────────────────────────────────

func (h *AlertHandlers) CreateMaintenanceWindow(w http.ResponseWriter, r *http.Request) {
	var body struct {
		InstanceName string    `json:"instance_name"`
		Engine       string    `json:"engine"`
		Reason       string    `json:"reason"`
		StartsAt     time.Time `json:"starts_at"`
		EndsAt       time.Time `json:"ends_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	mw := alerts.MaintenanceWindow{
		InstanceName: body.InstanceName,
		Engine:       alerts.Engine(body.Engine),
		Reason:       body.Reason,
		StartsAt:     body.StartsAt,
		EndsAt:       body.EndsAt,
		CreatedBy:    actorFromRequest(r),
	}
	if err := mw.Validate(); err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}

	created, err := h.maintRepo.Create(r.Context(), mw)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "failed to create maintenance window")
		return
	}

	w.WriteHeader(http.StatusCreated)
	jsonSuccess(w, created)
}

func (h *AlertHandlers) ListMaintenanceWindows(w http.ResponseWriter, r *http.Request) {
	windows, err := h.maintRepo.ListActive(r.Context(), time.Now().UTC())
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "failed to list maintenance windows")
		return
	}
	jsonSuccess(w, windows)
}

func (h *AlertHandlers) DeleteMaintenanceWindow(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid maintenance window id")
		return
	}
	if err := h.maintRepo.Delete(r.Context(), id); err != nil {
		jsonError(w, http.StatusInternalServerError, "failed to delete maintenance window")
		return
	}
	jsonSuccess(w, map[string]string{"status": "deleted"})
}

// ── JSON helpers (consistent envelope) ─────────────────────────

func jsonSuccess(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    data,
	})
}

func jsonError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": false,
		"error":   msg,
	})
}

// actorFromRequest extracts the authenticated username from JWT claims,
// falling back to "system" when auth is not required.
func actorFromRequest(r *http.Request) string {
	if claims := middleware.GetAuthClaims(r); claims != nil && claims.Username != "" {
		return claims.Username
	}
	return "system"
}

// writeAlertError maps domain errors to the correct HTTP status code.
func writeAlertError(w http.ResponseWriter, err error, fallbackMsg string) {
	switch {
	case errors.Is(err, alerts.ErrInvalidAlertID):
		jsonError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, alerts.ErrAlertNotFound):
		jsonError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, alerts.ErrAlertAlreadyResolved):
		jsonError(w, http.StatusConflict, err.Error())
	default:
		jsonError(w, http.StatusInternalServerError, fallbackMsg)
	}
}
