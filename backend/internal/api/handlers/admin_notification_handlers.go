// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Admin API handlers for managing outbound alert notification channels.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rsharma155/sql_optima/internal/apiresponse"
	"github.com/rsharma155/sql_optima/internal/middleware"
	"github.com/rsharma155/sql_optima/internal/repository"
	"github.com/rsharma155/sql_optima/internal/service"
)

type AdminNotificationHandlers struct {
	repo       *repository.NotificationConfigRepository
	notifier   *service.Notifier
	metricsSvc *service.MetricsService
}

func NewAdminNotificationHandlers(repo *repository.NotificationConfigRepository, notifier *service.Notifier, metricsSvc *service.MetricsService) *AdminNotificationHandlers {
	return &AdminNotificationHandlers{repo: repo, notifier: notifier, metricsSvc: metricsSvc}
}

func isAllowedNotificationChannel(ch string) bool {
	switch ch {
	case "webhook", "slack", "pagerduty", "email":
		return true
	default:
		return false
	}
}

// GetConfig returns all notification channel configs, masking secrets.
func (h *AdminNotificationHandlers) GetConfig(w http.ResponseWriter, r *http.Request) {
	if h.repo == nil {
		apiresponse.WritePlainError(w, http.StatusServiceUnavailable, "repository unavailable", nil)
		return
	}
	rows, err := h.repo.ListAll(r.Context())
	if err != nil {
		apiresponse.WritePlainError(w, http.StatusInternalServerError, "failed to load notification config", err, "handler", "GetConfig")
		return
	}
	type safeRow struct {
		Channel      string          `json:"channel"`
		URLMasked    string          `json:"url_masked"`
		IsEnabled    bool            `json:"is_enabled"`
		HasURL       bool            `json:"has_url"`
		EmailSettings json.RawMessage `json:"email_settings,omitempty"`
	}
	out := make([]safeRow, 0, len(rows))
	for _, row := range rows {
		sr := safeRow{
			Channel:   row.Channel,
			IsEnabled: row.IsEnabled,
			HasURL:    row.URL != "",
		}
		if row.Channel == "email" && row.URL != "" {
			if cfg, err := service.ParseEmailSMTPConfig(row.URL); err == nil {
				sr.EmailSettings = json.RawMessage(cfg.RedactedJSON())
				sr.URLMasked = "smtp://" + cfg.Host
			} else {
				sr.URLMasked = "••••••"
			}
		} else if len(row.URL) > 6 {
			sr.URLMasked = "••••••" + row.URL[len(row.URL)-6:]
		} else if row.URL != "" {
			sr.URLMasked = "••••••"
		}
		out = append(out, sr)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// UpdateConfig saves a channel config.
// Webhook/Slack/PagerDuty: { "channel", "url", "is_enabled" }
// Email: { "channel":"email", "is_enabled", "email": { host, port, username, password?, from, to, starttls } }
func (h *AdminNotificationHandlers) UpdateConfig(w http.ResponseWriter, r *http.Request) {
	if h.repo == nil {
		apiresponse.WritePlainError(w, http.StatusServiceUnavailable, "repository unavailable", nil)
		return
	}
	var input struct {
		Channel   string          `json:"channel"`
		URL       string          `json:"url"`
		IsEnabled bool            `json:"is_enabled"`
		Email     json.RawMessage `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		apiresponse.WritePlainError(w, http.StatusBadRequest, "invalid request body", err)
		return
	}
	input.Channel = strings.ToLower(strings.TrimSpace(input.Channel))
	if !isAllowedNotificationChannel(input.Channel) {
		apiresponse.WritePlainError(w, http.StatusBadRequest, "channel must be webhook, slack, pagerduty, or email", nil)
		return
	}

	urlProvided := strings.TrimSpace(input.URL) != "" || len(input.Email) > 0
	storeURL := strings.TrimSpace(input.URL)

	if input.Channel == "email" {
		merged, err := h.mergeEmailConfig(r, input.Email)
		if err != nil {
			apiresponse.WritePlainError(w, http.StatusBadRequest, err.Error(), err)
			return
		}
		raw, _ := json.Marshal(merged)
		storeURL = string(raw)
		urlProvided = true
	} else if storeURL == "" {
		rows, err := h.repo.ListAll(r.Context())
		if err != nil {
			apiresponse.WritePlainError(w, http.StatusInternalServerError, "failed to load notification config", err, "handler", "UpdateConfig")
			return
		}
		for _, row := range rows {
			if row.Channel == input.Channel {
				storeURL = row.URL
				break
			}
		}
	}

	actor := ""
	if claims := middleware.GetAuthClaims(r); claims != nil {
		actor = claims.Username
	}
	if err := h.repo.Upsert(r.Context(), input.Channel, storeURL, input.IsEnabled, actor); err != nil {
		apiresponse.WritePlainError(w, http.StatusInternalServerError, "failed to update notification channel", err, "handler", "UpdateConfig", "channel", input.Channel)
		return
	}

	middleware.AuditAction(slog.Default(), r, "admin_update_notification_channel",
		slog.String("channel", input.Channel),
		slog.Bool("enabled", input.IsEnabled),
		slog.Bool("url_changed", urlProvided),
	)
	if h.metricsSvc != nil && h.metricsSvc.AuditRepo != nil {
		_ = h.metricsSvc.AuditRepo.Log(r.Context(), "update_notification_channel", uuid.Nil, actor, r.RemoteAddr, map[string]interface{}{
			"channel":     input.Channel,
			"is_enabled":  input.IsEnabled,
			"url_changed": urlProvided,
		})
	}
	slog.Info("[AdminNotification] channel updated", "channel", input.Channel, "actor", actor, "enabled", input.IsEnabled)

	if h.notifier != nil {
		h.notifier.LoadFromDB(r.Context(), h.repo)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AdminNotificationHandlers) mergeEmailConfig(r *http.Request, raw json.RawMessage) (service.EmailSMTPConfig, error) {
	if len(raw) == 0 {
		return service.EmailSMTPConfig{}, errEmailConfigRequired()
	}
	var partial service.EmailSMTPConfig
	if err := json.Unmarshal(raw, &partial); err != nil {
		return service.EmailSMTPConfig{}, err
	}
	if partial.Password == "" && h.repo != nil {
		rows, lerr := h.repo.ListAll(r.Context())
		if lerr == nil {
			for _, row := range rows {
				if row.Channel == "email" && row.URL != "" {
					if prev, perr := service.ParseEmailSMTPConfig(row.URL); perr == nil {
						partial.Password = prev.Password
					}
					break
				}
			}
		}
	}
	b, _ := json.Marshal(partial)
	return service.ParseEmailSMTPConfig(string(b))
}

func errEmailConfigRequired() error {
	return &emailConfigError{msg: "email settings object is required"}
}

type emailConfigError struct{ msg string }

func (e *emailConfigError) Error() string { return e.msg }

// TestNotification sends a test payload to the specified channel.
func (h *AdminNotificationHandlers) TestNotification(w http.ResponseWriter, r *http.Request) {
	if h.repo == nil {
		apiresponse.WritePlainError(w, http.StatusServiceUnavailable, "repository unavailable", nil)
		return
	}
	var input struct {
		Channel string `json:"channel"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		apiresponse.WritePlainError(w, http.StatusBadRequest, "invalid request body", err)
		return
	}
	input.Channel = strings.ToLower(strings.TrimSpace(input.Channel))
	if !isAllowedNotificationChannel(input.Channel) {
		apiresponse.WritePlainError(w, http.StatusBadRequest, "channel must be webhook, slack, pagerduty, or email", nil)
		return
	}

	rows, err := h.repo.ListAll(r.Context())
	if err != nil {
		apiresponse.WritePlainError(w, http.StatusInternalServerError, "failed to load notification config", err, "handler", "TestNotification")
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
		apiresponse.WritePlainError(w, http.StatusBadRequest, "channel not configured or not enabled", nil)
		return
	}
	if h.notifier == nil {
		apiresponse.WritePlainError(w, http.StatusServiceUnavailable, "notifier unavailable", nil)
		return
	}

	type testResult struct {
		OK      bool   `json:"ok"`
		Message string `json:"message,omitempty"`
	}
	var errSend error
	switch input.Channel {
	case "pagerduty":
		errSend = h.notifier.PostSyncPagerDuty(r.Context(), targetURL, service.TestPagerDutyPayload(targetURL))
	case "email":
		cfg, perr := service.ParseEmailSMTPConfig(targetURL)
		if perr != nil {
			apiresponse.WritePlainError(w, http.StatusBadRequest, "invalid email smtp config", perr)
			return
		}
		subject, body := service.BuildAlertEmail(service.WebhookPayload{
			EventType: "test", Title: "SQL Optima — Test Notification", Severity: "info",
			ServerName: "SQL Optima", Category: "test", FiredAt: time.Now().UTC(),
		})
		errSend = service.SendEmailSMTP(r.Context(), cfg, subject, body)
	default:
		errSend = h.notifier.PostSync(r.Context(), targetURL, service.TestPayload(input.Channel))
	}
	if errSend != nil {
		slog.Error("[AdminNotification] test notification failed", "channel", input.Channel, "err", errSend)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(testResult{OK: false, Message: "test notification failed"})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(testResult{OK: true})
}
