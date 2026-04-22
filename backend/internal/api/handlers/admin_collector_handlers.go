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
	"log"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
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
		http.Error(w, "Repository not available (TimescaleDB not connected)", http.StatusServiceUnavailable)
		return
	}
	configs, err := repo.ListAll(r.Context())
	if err != nil {
		log.Printf("[AdminCollectorHandlers] ListConfigs error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(configs)
}

func (h *AdminCollectorHandlers) UpdateConfig(w http.ResponseWriter, r *http.Request) {
	repo := h.getRepo()
	if repo == nil {
		http.Error(w, "Repository not available (TimescaleDB not connected)", http.StatusServiceUnavailable)
		return
	}
	vars := mux.Vars(r)
	id, _ := strconv.Atoi(vars["id"])

	var input struct {
		FrequencySeconds int `json:"frequency_seconds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "Invalid input", http.StatusBadRequest)
		return
	}

	// Validation: 15 seconds to 7 days (604800 seconds)
	if input.FrequencySeconds < 15 || input.FrequencySeconds > 604800 {
		http.Error(w, "Frequency must be between 15 and 604800 seconds (7 days)", http.StatusBadRequest)
		return
	}

	claims := middleware.GetAuthClaims(r)
	updatedBy := "admin"
	if claims != nil {
		updatedBy = claims.Username
	}

	if err := h.repo.UpdateFrequency(r.Context(), id, input.FrequencySeconds, updatedBy); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
