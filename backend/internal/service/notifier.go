// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Notification dispatcher for alert engine (Webhook/Slack).
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/rsharma155/sql_optima/internal/domain/alerts"
	"github.com/rsharma155/sql_optima/internal/repository"
)

// NotifierConfig holds outbound notification destinations.
// All fields are optional — if empty, that channel is skipped.
type NotifierConfig struct {
	WebhookURL      string // generic HTTP POST, receives WebhookPayload JSON
	SlackWebhookURL string // Slack Incoming Webhook URL
}

// Notifier dispatches alert notifications to configured channels.
// Config is protected by an RWMutex so admin panel updates take effect
// immediately without a server restart.
type Notifier struct {
	mu     sync.RWMutex
	cfg    NotifierConfig
	client *http.Client
	log    *slog.Logger
}

func NewNotifier(cfg NotifierConfig, log *slog.Logger) *Notifier {
	if log == nil {
		log = slog.Default()
	}
	return &Notifier{
		cfg:    cfg,
		client: &http.Client{Timeout: 10 * time.Second},
		log:    log,
	}
}

// UpdateConfig replaces the active config atomically. Called by the admin
// notification handler after a successful DB save.
func (n *Notifier) UpdateConfig(cfg NotifierConfig) {
	n.mu.Lock()
	n.cfg = cfg
	n.mu.Unlock()
}

// LoadFromDB reads the latest config from the repository and applies it.
// Called at startup so env-var overrides are preserved only as defaults.
func (n *Notifier) LoadFromDB(ctx context.Context, repo *repository.NotificationConfigRepository) {
	rows, err := repo.ListAll(ctx)
	if err != nil {
		n.log.Warn("notifier: could not load config from DB", "error", err)
		return
	}
	cfg := n.GetConfig() // start from current (env-var) defaults
	for _, r := range rows {
		if !r.IsEnabled || r.URL == "" {
			continue
		}
		switch r.Channel {
		case "webhook":
			cfg.WebhookURL = r.URL
		case "slack":
			cfg.SlackWebhookURL = r.URL
		}
	}
	n.UpdateConfig(cfg)
}

// GetConfig returns a snapshot of the current config (safe for reads).
func (n *Notifier) GetConfig() NotifierConfig {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.cfg
}

// WebhookPayload is the JSON body sent to generic webhooks.
type WebhookPayload struct {
	EventType   string         `json:"event_type"` // "alert.opened" | "alert.resolved"
	AlertID     string         `json:"alert_id"`
	Fingerprint string         `json:"fingerprint"`
	ServerName  string         `json:"server_name"`
	Engine      string         `json:"engine"`
	Severity    string         `json:"severity"`
	Category    string         `json:"category"`
	Title       string         `json:"title"`
	Description string         `json:"description,omitempty"`
	Status      string         `json:"status"`
	FiredAt     time.Time      `json:"fired_at"`
	Evidence    map[string]any `json:"evidence,omitempty"`
}

// Dispatch sends a notification for the given alert. It is non-blocking — call it
// from a goroutine or after the Upsert. Errors are logged but never propagated to
// the caller (a notification failure must never block alert persistence).
func (n *Notifier) Dispatch(ctx context.Context, a alerts.Alert, eventType string) {
	cfg := n.GetConfig()
	if cfg.WebhookURL == "" && cfg.SlackWebhookURL == "" {
		return // nothing configured — skip silently
	}

	desc := ""
	if a.Description != nil {
		desc = *a.Description
	}

	payload := WebhookPayload{
		EventType:   eventType,
		AlertID:     a.ID.String(),
		Fingerprint: a.Fingerprint,
		ServerName:  a.ServerName,
		Engine:      string(a.Engine),
		Severity:    string(a.Severity),
		Category:    a.Category,
		Title:       a.Title,
		Description: desc,
		Status:      string(a.Status),
		FiredAt:     a.FirstSeenAt,
		Evidence:    a.Evidence,
	}

	if cfg.WebhookURL != "" {
		go n.postJSON(ctx, cfg.WebhookURL, payload)
	}
	if cfg.SlackWebhookURL != "" {
		go n.postSlack(ctx, cfg.SlackWebhookURL, payload)
	}
}

func (n *Notifier) postJSON(ctx context.Context, url string, payload any) {
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		n.log.Error("notifier: failed to build webhook request", "error", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := n.client.Do(req)
	if err != nil {
		n.log.Error("notifier: webhook delivery failed", "url", url, "error", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		n.log.Warn("notifier: webhook returned non-2xx", "url", url, "status", resp.StatusCode)
	}
}

// slackMessage is the minimal Slack Incoming Webhook payload.
type slackMessage struct {
	Text        string            `json:"text"`
	Attachments []slackAttachment `json:"attachments"`
}

type slackAttachment struct {
	Color  string `json:"color"` // "danger" | "warning" | "good"
	Title  string `json:"title"`
	Text   string `json:"text"`
	Footer string `json:"footer"`
	Ts     int64  `json:"ts"`
}

func buildSlackMessage(p WebhookPayload) slackMessage {
	color := map[string]string{
		"critical": "danger",
		"warning":  "warning",
		"info":     "good",
	}[p.Severity]
	if color == "" {
		color = "warning"
	}

	emoji := map[string]string{"critical": "🔴", "warning": "🟡", "info": "🔵"}[p.Severity]
	server := p.ServerName
	if server == "" {
		server = "SQL Optima"
	}
	return slackMessage{
		Text: fmt.Sprintf("%s *[SQL Optima Alert]* %s on `%s`", emoji, p.Title, server),
		Attachments: []slackAttachment{{
			Color:  color,
			Title:  p.Title,
			Text:   fmt.Sprintf("*Severity:* %s\n*Category:* %s\n*Engine:* %s\n%s", p.Severity, p.Category, p.Engine, p.Description),
			Footer: fmt.Sprintf("SQL Optima • %s", server),
			Ts:     p.FiredAt.Unix(),
		}},
	}
}

func (n *Notifier) postSlack(ctx context.Context, url string, p WebhookPayload) {
	n.postJSON(ctx, url, buildSlackMessage(p))
}

// TestPayload returns the JSON body used by the admin "Test" action for a channel.
func TestPayload(channel string) any {
	if channel == "slack" {
		return buildSlackMessage(WebhookPayload{
			EventType:   "test",
			ServerName:  "SQL Optima",
			Severity:    "info",
			Category:    "test",
			Title:       "SQL Optima — Test Notification",
			Description: "This is a test message from SQL Optima admin panel.",
			FiredAt:     time.Now(),
		})
	}
	return WebhookPayload{
		EventType:   "test",
		ServerName:  "SQL Optima",
		Severity:    "info",
		Category:    "test",
		Title:       "SQL Optima — Test Notification",
		Description: "This is a test message from SQL Optima admin panel.",
		FiredAt:     time.Now(),
	}
}

// PostSync delivers payload to url and returns a non-nil error on failure.
func (n *Notifier) PostSync(ctx context.Context, url string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := n.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("endpoint returned %s", resp.Status)
	}
	return nil
}
