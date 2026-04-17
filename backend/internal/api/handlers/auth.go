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
	"strings"

	"github.com/rsharma155/sql_optima/internal/middleware"
	"github.com/rsharma155/sql_optima/internal/service"
)

type AuthHandlers struct {
	metricsSvc   *service.MetricsService
	loginLimiter *middleware.LoginRateLimiter
}

func NewAuthHandlers(metricsSvc *service.MetricsService, loginLimiter *middleware.LoginRateLimiter) *AuthHandlers {
	return &AuthHandlers{metricsSvc: metricsSvc, loginLimiter: loginLimiter}
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
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MiB max login payload
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
		return
	}
	// Reject requests with trailing garbage after the JSON object.
	if dec.More() {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "unexpected trailing data in request body"})
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "username and password are required"})
		return
	}

	// Per-username throttle: a tighter limit per target account regardless of
	// source IP, protecting against distributed credential-stuffing.
	if h.loginLimiter != nil && !h.loginLimiter.AllowUsername(req.Username) {
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(map[string]string{"error": "rate limit exceeded"})
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

	// Set HttpOnly auth cookie (browser-safe, not accessible to JS).
	http.SetCookie(w, &http.Cookie{
		Name:     middleware.AuthCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400, // 24h — matches JWT expiry
	})

	// Set CSRF token cookie (JS-readable, not HttpOnly).
	csrfToken, csrfErr := middleware.GenerateCSRFToken()
	if csrfErr != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to generate CSRF token"})
		return
	}
	middleware.SetCSRFCookie(w, r, csrfToken)

	// Return user info in JSON body. The JWT is still included in the response
	// for backward compatibility with API clients that use Authorization header.
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

// Logout clears the auth and CSRF cookies.
func (h *AuthHandlers) Logout(w http.ResponseWriter, r *http.Request) {
	// Expire the auth cookie.
	http.SetCookie(w, &http.Cookie{
		Name:     middleware.AuthCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
	// Expire the CSRF cookie.
	http.SetCookie(w, &http.Cookie{
		Name:     middleware.CSRFCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: false,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "logged out"})
}
