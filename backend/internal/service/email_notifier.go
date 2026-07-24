// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: SMTP email channel for alert notifications (native email delivery).
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package service

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"
)

// EmailSMTPConfig is stored as JSON in optima_notification_config.url for channel=email.
// Password is never returned by admin GET (see RedactedJSON).
type EmailSMTPConfig struct {
	Host     string   `json:"host"`
	Port     int      `json:"port"`
	Username string   `json:"username,omitempty"`
	Password string   `json:"password,omitempty"`
	From     string   `json:"from"`
	To       []string `json:"to"`
	StartTLS bool     `json:"starttls"`
}

// ParseEmailSMTPConfig validates and parses SMTP JSON from the notification config store.
func ParseEmailSMTPConfig(raw string) (EmailSMTPConfig, error) {
	var c EmailSMTPConfig
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &c); err != nil {
		return c, fmt.Errorf("invalid email smtp json")
	}
	c.Host = strings.TrimSpace(c.Host)
	c.From = strings.TrimSpace(c.From)
	c.Username = strings.TrimSpace(c.Username)
	cleanTo := make([]string, 0, len(c.To))
	for _, t := range c.To {
		t = strings.TrimSpace(t)
		if t != "" {
			cleanTo = append(cleanTo, t)
		}
	}
	c.To = cleanTo
	if c.Host == "" || c.Port <= 0 || c.Port > 65535 || c.From == "" || len(c.To) == 0 {
		return c, fmt.Errorf("email smtp requires host, port, from, and at least one to address")
	}
	return c, nil
}

// RedactedJSON returns client-safe JSON without the password.
func (c EmailSMTPConfig) RedactedJSON() string {
	type safe struct {
		Host        string   `json:"host"`
		Port        int      `json:"port"`
		Username    string   `json:"username,omitempty"`
		From        string   `json:"from"`
		To          []string `json:"to"`
		StartTLS    bool     `json:"starttls"`
		HasPassword bool     `json:"has_password"`
	}
	b, _ := json.Marshal(safe{
		Host: c.Host, Port: c.Port, Username: c.Username, From: c.From, To: c.To,
		StartTLS: c.StartTLS, HasPassword: c.Password != "",
	})
	return string(b)
}

// BuildAlertEmail returns subject + plain-text body for an alert event.
func BuildAlertEmail(p WebhookPayload) (subject, body string) {
	subject = fmt.Sprintf("[SQL Optima][%s] %s", strings.ToUpper(p.Severity), p.Title)
	var b strings.Builder
	b.WriteString("SQL Optima alert notification\n")
	b.WriteString("==============================\n\n")
	fmt.Fprintf(&b, "Event:       %s\n", p.EventType)
	fmt.Fprintf(&b, "Severity:    %s\n", p.Severity)
	fmt.Fprintf(&b, "Server:      %s\n", p.ServerName)
	fmt.Fprintf(&b, "Engine:      %s\n", p.Engine)
	fmt.Fprintf(&b, "Category:    %s\n", p.Category)
	fmt.Fprintf(&b, "Title:       %s\n", p.Title)
	fmt.Fprintf(&b, "Fingerprint: %s\n", p.Fingerprint)
	fmt.Fprintf(&b, "Fired at:    %s\n\n", p.FiredAt.UTC().Format(time.RFC3339))
	if p.Description != "" {
		fmt.Fprintf(&b, "Description:\n%s\n", p.Description)
	}
	return subject, b.String()
}

// SendEmailSMTP delivers a plain-text message. Uses STARTTLS when configured.
// Timeouts are short to avoid blocking the alert path (call from a goroutine).
func SendEmailSMTP(ctx context.Context, cfg EmailSMTPConfig, subject, body string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	msg := buildRFC822(cfg.From, cfg.To, subject, body)

	d := net.Dialer{Timeout: 8 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("smtp dial: %w", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(15 * time.Second))

	client, err := smtp.NewClient(conn, cfg.Host)
	if err != nil {
		return fmt.Errorf("smtp client: %w", err)
	}
	defer client.Close()

	if cfg.StartTLS {
		tlsCfg := &tls.Config{ServerName: cfg.Host, MinVersion: tls.VersionTLS12}
		if err := client.StartTLS(tlsCfg); err != nil {
			return fmt.Errorf("smtp starttls: %w", err)
		}
	}
	if cfg.Username != "" {
		auth := smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}
	if err := client.Mail(cfg.From); err != nil {
		return err
	}
	for _, to := range cfg.To {
		if err := client.Rcpt(to); err != nil {
			return err
		}
	}
	wc, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := wc.Write(msg); err != nil {
		_ = wc.Close()
		return err
	}
	if err := wc.Close(); err != nil {
		return err
	}
	return client.Quit()
}

func buildRFC822(from string, to []string, subject, body string) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", strings.Join(to, ", "))
	fmt.Fprintf(&b, "Subject: %s\r\n", sanitizeHeader(subject))
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(body)
	return []byte(b.String())
}

func sanitizeHeader(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' {
			return -1
		}
		return r
	}, s)
}
