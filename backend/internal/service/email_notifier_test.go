// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Tests for SMTP email notification config parsing and message building.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package service

import (
	"strings"
	"testing"
	"time"
)

func TestParseEmailSMTPConfig_Valid(t *testing.T) {
	raw := `{"host":"smtp.example.com","port":587,"username":"u","password":"secret","from":"a@ex.com","to":["ops@ex.com","oncall@ex.com"],"starttls":true}`
	cfg, err := ParseEmailSMTPConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Host != "smtp.example.com" || cfg.Port != 587 || !cfg.StartTLS {
		t.Fatalf("unexpected cfg: %+v", cfg)
	}
	if len(cfg.To) != 2 || cfg.Password != "secret" {
		t.Fatalf("to/password mismatch: %+v", cfg)
	}
}

func TestParseEmailSMTPConfig_RejectsMissingFields(t *testing.T) {
	cases := []string{
		`{}`,
		`{"host":"h","port":25,"from":"a@b.c","to":[]}`,
		`{"host":"h","port":0,"from":"a@b.c","to":["a@b.c"]}`,
		`{"host":"","port":587,"from":"a@b.c","to":["a@b.c"]}`,
	}
	for _, c := range cases {
		if _, err := ParseEmailSMTPConfig(c); err == nil {
			t.Fatalf("expected error for %s", c)
		}
	}
}

func TestBuildAlertEmail_ContainsNoPassword(t *testing.T) {
	p := WebhookPayload{
		EventType:   "alert.opened",
		Title:       "Disk full",
		Severity:    "critical",
		ServerName:  "pg-1",
		Category:    "Storage",
		Engine:      "postgres",
		Description: "mount /data at 98%",
		FiredAt:     time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC),
		Fingerprint: "fp-xyz",
	}
	subject, body := BuildAlertEmail(p)
	if !strings.Contains(strings.ToLower(subject), "critical") || !strings.Contains(subject, "Disk full") {
		t.Fatalf("subject = %q", subject)
	}
	if strings.Contains(strings.ToLower(body), "password") {
		t.Fatal("body must not mention password")
	}
	if !strings.Contains(body, "fp-xyz") || !strings.Contains(body, "pg-1") {
		t.Fatalf("body missing fields: %s", body)
	}
}

func TestEmailSMTPConfig_RedactedJSON_OmitsPassword(t *testing.T) {
	cfg := EmailSMTPConfig{
		Host: "smtp.example.com", Port: 587, Username: "u", Password: "secret",
		From: "a@ex.com", To: []string{"ops@ex.com"}, StartTLS: true,
	}
	s := cfg.RedactedJSON()
	if strings.Contains(s, "secret") {
		t.Fatalf("password leaked: %s", s)
	}
	if !strings.Contains(s, `"has_password":true`) {
		t.Fatalf("expected has_password: %s", s)
	}
}
