// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: API handlers for managing collector frequency configurations.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/rsharma155/sql_optima/internal/apiresponse"
	"github.com/rsharma155/sql_optima/internal/middleware"
	"github.com/rsharma155/sql_optima/internal/repository"
	"github.com/rsharma155/sql_optima/internal/service"
)

type AdminCollectorHandlers struct {
	repo       *repository.CollectorConfigRepository
	metricsSvc *service.MetricsService
}

func NewAdminCollectorHandlers(metricsSvc *service.MetricsService) *AdminCollectorHandlers {
	var repo *repository.CollectorConfigRepository
	if pool := metricsSvc.GetTimescaleDBPool(); pool != nil {
		repo = repository.NewCollectorConfigRepository(pool)
	}
	return &AdminCollectorHandlers{
		repo:       repo,
		metricsSvc: metricsSvc,
	}
}

func (h *AdminCollectorHandlers) getRepo() *repository.CollectorConfigRepository {
	if h.repo != nil {
		return h.repo
	}
	if h.metricsSvc != nil {
		if pool := h.metricsSvc.GetTimescaleDBPool(); pool != nil {
			h.repo = repository.NewCollectorConfigRepository(pool)
		}
	}
	return h.repo
}

func (h *AdminCollectorHandlers) ListConfigs(w http.ResponseWriter, r *http.Request) {
	repo := h.getRepo()
	if repo == nil {
		apiresponse.WritePlainError(w, http.StatusServiceUnavailable, "repository unavailable", nil)
		return
	}
	configs, err := repo.ListAll(r.Context())
	if err != nil {
		apiresponse.WritePlainError(w, http.StatusInternalServerError, "failed to list collector configs", err, "handler", "ListConfigs")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(configs)
}

func (h *AdminCollectorHandlers) UpdateConfig(w http.ResponseWriter, r *http.Request) {
	repo := h.getRepo()
	if repo == nil {
		apiresponse.WritePlainError(w, http.StatusServiceUnavailable, "repository unavailable", nil)
		return
	}
	vars := mux.Vars(r)
	id, _ := strconv.Atoi(vars["id"])

	var input struct {
		FrequencySeconds int `json:"frequency_seconds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		apiresponse.WritePlainError(w, http.StatusBadRequest, "invalid input", err)
		return
	}

	// Validation: 15 seconds to 7 days (604800 seconds)
	if input.FrequencySeconds < 15 || input.FrequencySeconds > 604800 {
		apiresponse.WritePlainError(w, http.StatusBadRequest, "frequency must be between 15 and 604800 seconds (7 days)", nil)
		return
	}

	claims := middleware.GetAuthClaims(r)
	updatedBy := "admin"
	if claims != nil {
		updatedBy = claims.Username
	}

	if err := repo.UpdateFrequency(r.Context(), id, input.FrequencySeconds, updatedBy); err != nil {
		apiresponse.WritePlainError(w, http.StatusInternalServerError, "failed to update collector frequency", err, "handler", "UpdateConfig", "id", id)
		return
	}

	middleware.AuditAction(slog.Default(), r, "admin_update_collector_frequency",
		slog.Int("id", id),
		slog.Int("frequency_seconds", input.FrequencySeconds),
	)
	if h.metricsSvc != nil && h.metricsSvc.AuditRepo != nil {
		_ = h.metricsSvc.AuditRepo.Log(r.Context(), "update_collector_frequency", uuid.Nil, updatedBy, r.RemoteAddr, map[string]interface{}{
			"id":                id,
			"frequency_seconds": input.FrequencySeconds,
		})
	}

	w.WriteHeader(http.StatusNoContent)
}
