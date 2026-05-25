// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Admin-only handlers for user management (create, list, delete, update roles) and system configuration.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/rsharma155/sql_optima/internal/apiresponse"
	"github.com/rsharma155/sql_optima/internal/middleware"
	"github.com/rsharma155/sql_optima/internal/service"
)

func (h *AdminHandlers) auditAdmin(ctx context.Context, eventType string, r *http.Request, metadata map[string]interface{}) {
	if h.metricsSvc == nil || h.metricsSvc.AuditRepo == nil {
		return
	}
	actor := ""
	if claims := middleware.GetAuthClaims(r); claims != nil {
		actor = claims.Username
	}
	_ = h.metricsSvc.AuditRepo.Log(ctx, eventType, uuid.Nil, actor, r.RemoteAddr, metadata)
}

type AdminHandlers struct {
	metricsSvc *service.MetricsService
}

func NewAdminHandlers(metricsSvc *service.MetricsService) *AdminHandlers {
	return &AdminHandlers{metricsSvc: metricsSvc}
}

func (h *AdminHandlers) CreateUser(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if h.metricsSvc.UserRepo == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "User management not configured"})
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
		return
	}
	if req.Username == "" || req.Password == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "username and password are required"})
		return
	}
	if req.Role != "admin" && req.Role != "dba" && req.Role != "viewer" {
		req.Role = "viewer"
	}

	user, err := h.metricsSvc.UserRepo.CreateUser(r.Context(), req.Username, req.Password, req.Role)
	if err != nil {
		apiresponse.WriteJSONError(w, http.StatusBadRequest, "could not create user", err)
		return
	}

	middleware.AuditAction(slog.Default(), r, "admin_create_user", slog.String("new_username", req.Username), slog.String("role", req.Role))
	h.auditAdmin(r.Context(), "create_user", r, map[string]interface{}{
		"username": req.Username,
		"role":     req.Role,
		"user_id":  user.UserID,
	})

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"user_id":  user.UserID,
		"username": user.Username,
		"role":     user.Role,
	})
}

func (h *AdminHandlers) ListUsers(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if h.metricsSvc.UserRepo == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "User management not configured"})
		return
	}

	users, err := h.metricsSvc.UserRepo.GetAllUsers(r.Context())
	if err != nil {
		apiresponse.WriteJSONError(w, http.StatusInternalServerError, "failed to list users", err)
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{"users": users})
}

func (h *AdminHandlers) DeleteUser(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if h.metricsSvc.UserRepo == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "User management not configured"})
		return
	}

	idStr := mux.Vars(r)["id"]
	id, err := strconv.Atoi(idStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid user id"})
		return
	}

	if err := h.metricsSvc.UserRepo.DeleteUser(r.Context(), id); err != nil {
		apiresponse.WriteJSONError(w, http.StatusBadRequest, "could not delete user", err)
		return
	}

	middleware.AuditAction(slog.Default(), r, "admin_delete_user", slog.Int("user_id", id))
	h.auditAdmin(r.Context(), "delete_user", r, map[string]interface{}{"user_id": id})

	json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}

func (h *AdminHandlers) UpdateUserRole(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if h.metricsSvc.UserRepo == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "User management not configured"})
		return
	}

	idStr := mux.Vars(r)["id"]
	id, err := strconv.Atoi(idStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid user id"})
		return
	}

	var req struct {
		Role string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
		return
	}
	if req.Role != "admin" && req.Role != "viewer" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "role must be 'admin' or 'viewer'"})
		return
	}

	if err := h.metricsSvc.UserRepo.UpdateUserRole(r.Context(), id, req.Role); err != nil {
		apiresponse.WriteJSONError(w, http.StatusBadRequest, "could not update user role", err)
		return
	}

	middleware.AuditAction(slog.Default(), r, "admin_update_user_role", slog.Int("user_id", id), slog.String("role", req.Role))
	h.auditAdmin(r.Context(), "update_user_role", r, map[string]interface{}{"user_id": id, "role": req.Role})

	json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}
