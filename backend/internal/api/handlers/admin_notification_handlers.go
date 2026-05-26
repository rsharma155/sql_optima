// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Admin API handlers for managing outbound alert notification channels.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"log/slog"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/rsharma155/sql_optima/internal/middleware"
	"github.com/rsharma155/sql_optima/internal/repository"
	"github.com/rsharma155/sql_optima/internal/service"
)

type AdminNotificationHandlers struct {
	repo     *repository.NotificationConfigRepository
	notifier *service.Notifier
}

func NewAdminNotificationHandlers(repo *repository.NotificationConfigRepository, notifier *service.Notifier) *AdminNotificationHandlers {
	return &AdminNotificationHandlers{repo: repo, notifier: notifier}
}

// GetConfig returns all notification channel configs, masking the URL to last 6 chars.
func (h *AdminNotificationHandlers) GetConfig(w http.ResponseWriter, r *http.Request) {
	if h.repo == nil {
		http.Error(w, "repository unavailable", http.StatusServiceUnavailable)
		return
	}
	rows, err := h.repo.ListAll(r.Context())
	if err != nil {
		slog.Error("[AdminNotification] GetConfig error", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	type safeRow struct {
		Channel   string `json:"channel"`
		URLMasked string `json:"url_masked"`
		IsEnabled bool   `json:"is_enabled"`
		HasURL    bool   `json:"has_url"`
	}
	out := make([]safeRow, 0, len(rows))
	for _, r := range rows {
		masked := ""
		if len(r.URL) > 6 {
			masked = "••••••" + r.URL[len(r.URL)-6:]
		} else if r.URL != "" {
			masked = "••••••"
		}
		out = append(out, safeRow{
			Channel:   r.Channel,
			URLMasked: masked,
			IsEnabled: r.IsEnabled,
			HasURL:    r.URL != "",
		})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// UpdateConfig saves a single channel's URL and enabled state.
// Body: { "channel": "webhook"|"slack", "url": "...", "is_enabled": true }
func (h *AdminNotificationHandlers) UpdateConfig(w http.ResponseWriter, r *http.Request) {
	if h.repo == nil {
		http.Error(w, "repository unavailable", http.StatusServiceUnavailable)
		return
	}
	var input struct {
		Channel   string `json:"channel"`
		URL       string `json:"url"`
		IsEnabled bool   `json:"is_enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	input.Channel = strings.ToLower(strings.TrimSpace(input.Channel))
	if input.Channel != "webhook" && input.Channel != "slack" {
		http.Error(w, "channel must be 'webhook' or 'slack'", http.StatusBadRequest)
		return
	}
	input.URL = strings.TrimSpace(input.URL)
	if input.URL == "" {
		rows, err := h.repo.ListAll(r.Context())
		if err != nil {
			slog.Error("[AdminNotification] UpdateConfig list error", "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		for _, row := range rows {
			if row.Channel == input.Channel {
				input.URL = row.URL
				break
			}
		}
	}

	actor := ""
	if claims := middleware.GetAuthClaims(r); claims != nil {
		actor = claims.Username
	}
	if err := h.repo.Upsert(r.Context(), input.Channel, input.URL, input.IsEnabled, actor); err != nil {
		slog.Error("[AdminNotification] UpdateConfig error", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	slog.Info("[AdminNotification] channel updated", "channel", input.Channel, "actor", actor, "enabled", input.IsEnabled)

	// Apply change to the live notifier immediately.
	if h.notifier != nil {
		h.notifier.LoadFromDB(r.Context(), h.repo)
	}

	w.WriteHeader(http.StatusNoContent)
}

// TestNotification sends a test payload to the specified channel.
// Body: { "channel": "webhook"|"slack" }
func (h *AdminNotificationHandlers) TestNotification(w http.ResponseWriter, r *http.Request) {
	if h.repo == nil {
		http.Error(w, "repository unavailable", http.StatusServiceUnavailable)
		return
	}
	var input struct {
		Channel string `json:"channel"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	input.Channel = strings.ToLower(strings.TrimSpace(input.Channel))
	if input.Channel != "webhook" && input.Channel != "slack" {
		http.Error(w, "channel must be 'webhook' or 'slack'", http.StatusBadRequest)
		return
	}

	rows, err := h.repo.ListAll(r.Context())
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	var targetURL string
	for _, row := range rows {
		if row.Channel == input.Channel && row.IsEnabled && row.URL != "" {
			targetURL = row.URL
			break
		}
	}
	if targetURL == "" {
		http.Error(w, "channel not configured or not enabled", http.StatusBadRequest)
		return
	}

	if h.notifier == nil {
		http.Error(w, "notifier unavailable", http.StatusServiceUnavailable)
		return
	}

	type testResult struct {
		OK      bool   `json:"ok"`
		Message string `json:"message,omitempty"`
	}
	if err := h.notifier.PostSync(r.Context(), targetURL, service.TestPayload(input.Channel)); err != nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(testResult{OK: false, Message: err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(testResult{OK: true})
}
