// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Authentication handlers for login, token generation, and user session management with role-based access.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/rsharma155/sql_optima/internal/middleware"
	"github.com/rsharma155/sql_optima/internal/service"
)

type AuthHandlers struct {
	metricsSvc *service.MetricsService
}

func NewAuthHandlers(metricsSvc *service.MetricsService) *AuthHandlers {
	return &AuthHandlers{metricsSvc: metricsSvc}
}

func (h *AuthHandlers) Login(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if h.metricsSvc.UserRepo == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "User management not configured"})
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
		return
	}

	user, err := h.metricsSvc.UserRepo.AuthenticateUser(r.Context(), req.Username, req.Password)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid username or password"})
		return
	}

	token, err := middleware.GenerateToken(user.UserID, user.Username, user.Role)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to generate token"})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"token":    token,
		"user_id":  user.UserID,
		"username": user.Username,
		"role":     user.Role,
	})
}

func (h *AuthHandlers) Me(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	claims := middleware.GetAuthClaims(r)
	if claims == nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "not authenticated"})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"user_id":  claims.UserID,
		"username": claims.Username,
		"role":     claims.Role,
	})
}
