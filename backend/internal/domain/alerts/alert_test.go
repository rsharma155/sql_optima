// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Unit tests for alert value objects and entity lifecycle methods.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package alerts

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestSeverity_Valid(t *testing.T) {
	tests := []struct {
		sev  Severity
		want bool
	}{
		{SeverityInfo, true},
		{SeverityWarning, true},
		{SeverityCritical, true},
		{Severity("unknown"), false},
		{Severity(""), false},
	}
	for _, tt := range tests {
		if got := tt.sev.Valid(); got != tt.want {
			t.Errorf("Severity(%q).Valid() = %v, want %v", tt.sev, got, tt.want)
		}
	}
}

func TestSeverity_Weight(t *testing.T) {
	if SeverityCritical.Weight() <= SeverityWarning.Weight() {
		t.Error("critical should outweigh warning")
	}
	if SeverityWarning.Weight() <= SeverityInfo.Weight() {
		t.Error("warning should outweigh info")
	}
	if Severity("bogus").Weight() != 0 {
		t.Error("unknown severity should have zero weight")
	}
}

func TestStatus_Valid(t *testing.T) {
	tests := []struct {
		s    Status
		want bool
	}{
		{StatusOpen, true},
		{StatusAcknowledged, true},
		{StatusResolved, true},
		{Status("pending"), false},
	}
	for _, tt := range tests {
		if got := tt.s.Valid(); got != tt.want {
			t.Errorf("Status(%q).Valid() = %v, want %v", tt.s, got, tt.want)
		}
	}
}

func TestEngine_Valid(t *testing.T) {
	tests := []struct {
		e    Engine
		want bool
	}{
		{EnginePostgres, true},
		{EngineSQLServer, true},
		{Engine("mysql"), false},
	}
	for _, tt := range tests {
		if got := tt.e.Valid(); got != tt.want {
			t.Errorf("Engine(%q).Valid() = %v, want %v", tt.e, got, tt.want)
		}
	}
}

func newTestAlert() Alert {
	now := time.Now()
	return Alert{
		ID:           uuid.New(),
		Fingerprint:  "abc123",
		InstanceName: "prod-db-01",
		Engine:       EngineSQLServer,
		Severity:     SeverityWarning,
		Status:       StatusOpen,
		Category:     "blocking",
		Title:        "Active blocking detected",
		FirstSeenAt:  now,
		LastSeenAt:   now,
		HitCount:     1,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

func TestAlert_Acknowledge(t *testing.T) {
	a := newTestAlert()
	now := time.Now()

	if err := a.Acknowledge("admin", now); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a.Status != StatusAcknowledged {
		t.Errorf("status = %q, want %q", a.Status, StatusAcknowledged)
	}
	if a.AcknowledgedBy == nil || *a.AcknowledgedBy != "admin" {
		t.Errorf("acknowledged_by = %v, want admin", a.AcknowledgedBy)
	}
	if a.AcknowledgedAt == nil || !a.AcknowledgedAt.Equal(now) {
		t.Error("acknowledged_at not set correctly")
	}
}

func TestAlert_Acknowledge_AlreadyResolved(t *testing.T) {
	a := newTestAlert()
	a.Status = StatusResolved

	err := a.Acknowledge("admin", time.Now())
	if err != ErrAlertAlreadyResolved {
		t.Errorf("expected ErrAlertAlreadyResolved, got %v", err)
	}
}

func TestAlert_Resolve(t *testing.T) {
	a := newTestAlert()
	now := time.Now()

	if err := a.Resolve("admin", now); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a.Status != StatusResolved {
		t.Errorf("status = %q, want %q", a.Status, StatusResolved)
	}
	if a.ResolvedBy == nil || *a.ResolvedBy != "admin" {
		t.Errorf("resolved_by = %v, want admin", a.ResolvedBy)
	}
}

func TestAlert_Resolve_AlreadyResolved(t *testing.T) {
	a := newTestAlert()
	a.Status = StatusResolved

	err := a.Resolve("admin", time.Now())
	if err != ErrAlertAlreadyResolved {
		t.Errorf("expected ErrAlertAlreadyResolved, got %v", err)
	}
}

func TestAlert_BumpHitCount(t *testing.T) {
	a := newTestAlert()
	original := a.HitCount
	later := time.Now().Add(5 * time.Minute)

	a.BumpHitCount(later)

	if a.HitCount != original+1 {
		t.Errorf("hit_count = %d, want %d", a.HitCount, original+1)
	}
	if !a.LastSeenAt.Equal(later) {
		t.Error("last_seen_at should be updated")
	}
}

func TestAlert_IsOpen(t *testing.T) {
	tests := []struct {
		status Status
		want   bool
	}{
		{StatusOpen, true},
		{StatusAcknowledged, true},
		{StatusResolved, false},
	}
	for _, tt := range tests {
		a := newTestAlert()
		a.Status = tt.status
		if got := a.IsOpen(); got != tt.want {
			t.Errorf("IsOpen() with status %q = %v, want %v", tt.status, got, tt.want)
		}
	}
}
