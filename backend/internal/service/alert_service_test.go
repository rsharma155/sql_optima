// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Unit tests for AlertService orchestration logic.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/rsharma155/sql_optima/internal/domain/alerts"
)

// ── mock stores ────────────────────────────────────────────────

type mockAlertStore struct {
	mu     sync.Mutex
	alerts map[uuid.UUID]alerts.Alert
}

func newMockAlertStore() *mockAlertStore {
	return &mockAlertStore{alerts: make(map[uuid.UUID]alerts.Alert)}
}

func (m *mockAlertStore) Upsert(_ context.Context, a alerts.Alert) (alerts.Alert, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	// Check for existing open alert with same fingerprint
	for id, existing := range m.alerts {
		if existing.Fingerprint == a.Fingerprint && existing.Status != alerts.StatusResolved {
			existing.BumpHitCount(a.LastSeenAt)
			existing.Evidence = a.Evidence
			existing.Severity = a.Severity
			m.alerts[id] = existing
			return existing, nil
		}
	}
	a.CreatedAt = time.Now()
	a.UpdatedAt = time.Now()
	m.alerts[a.ID] = a
	return a, nil
}

func (m *mockAlertStore) GetByID(_ context.Context, id uuid.UUID) (alerts.Alert, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.alerts[id]
	if !ok {
		return a, alerts.ErrAlertNotFound
	}
	return a, nil
}

func (m *mockAlertStore) List(_ context.Context, f alerts.AlertFilter) ([]alerts.Alert, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []alerts.Alert
	for _, a := range m.alerts {
		if f.Status != nil && a.Status != *f.Status {
			continue
		}
		if f.ServerID != nil && a.ServerID != *f.ServerID {
			continue
		}
		result = append(result, a)
	}
	return result, nil
}

func (m *mockAlertStore) UpdateStatus(_ context.Context, id uuid.UUID, status alerts.Status, actor, _ string, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.alerts[id]
	if !ok {
		return alerts.ErrAlertNotFound
	}
	a.Status = status
	a.UpdatedAt = at
	if status == alerts.StatusAcknowledged {
		a.AcknowledgedBy = &actor
		a.AcknowledgedAt = &at
	}
	if status == alerts.StatusResolved {
		a.ResolvedBy = &actor
		a.ResolvedAt = &at
	}
	m.alerts[id] = a
	return nil
}

func (m *mockAlertStore) PruneResolved(_ context.Context, _ time.Duration) (int64, error) {
	return 0, nil
}

func (m *mockAlertStore) CountOpen(_ context.Context, serverID uuid.UUID) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for _, a := range m.alerts {
		if a.ServerID == serverID && a.IsOpen() {
			count++
		}
	}
	return count, nil
}

func (m *mockAlertStore) ResolveByFingerprint(_ context.Context, fingerprint, actor, _ string, at time.Time) (alerts.Alert, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, a := range m.alerts {
		if a.Fingerprint == fingerprint && a.Status != alerts.StatusResolved {
			a.Status = alerts.StatusResolved
			a.ResolvedBy = &actor
			a.ResolvedAt = &at
			m.alerts[id] = a
			return a, true, nil
		}
	}
	return alerts.Alert{}, false, nil
}

type mockMaintenanceStore struct {
	underMaintenance bool
}

func (m *mockMaintenanceStore) Create(_ context.Context, mw alerts.MaintenanceWindow) (alerts.MaintenanceWindow, error) {
	return mw, nil
}
func (m *mockMaintenanceStore) IsUnderMaintenance(_ context.Context, _ uuid.UUID, _ alerts.Engine, _ time.Time) (bool, error) {
	return m.underMaintenance, nil
}
func (m *mockMaintenanceStore) ListActive(_ context.Context, _ time.Time) ([]alerts.MaintenanceWindow, error) {
	return nil, nil
}
func (m *mockMaintenanceStore) Delete(_ context.Context, _ uuid.UUID) error {
	return nil
}

// ── mock evaluator ─────────────────────────────────────────────

type mockEvaluator struct {
	engine  alerts.Engine
	results []AlertEvaluatorResult
	err     error
}

func (m *mockEvaluator) Evaluate(_ context.Context, _ uuid.UUID) ([]AlertEvaluatorResult, error) {
	return m.results, m.err
}
func (m *mockEvaluator) Engine() alerts.Engine { return m.engine }

// ── tests ──────────────────────────────────────────────────────

func TestAlertService_RunEvaluation_CreatesAlerts(t *testing.T) {
	store := newMockAlertStore()
	maintStore := &mockMaintenanceStore{underMaintenance: false}

	id := uuid.New()
	ev := &mockEvaluator{
		engine: alerts.EngineSQLServer,
		results: []AlertEvaluatorResult{
			{
				RuleName:     "sqlserver_blocking",
				Category:     "blocking",
				Severity:     alerts.SeverityCritical,
				Title:        "Active blocking detected",
				Description:  "Session 55 blocking 3 others",
				ServerID:     id,
				Engine:       alerts.EngineSQLServer,
			},
		},
	}

	svc := NewAlertService(store, maintStore, []AlertEvaluator{ev}, nil)

	count, err := svc.RunEvaluation(context.Background(), id, alerts.EngineSQLServer, "test-instance")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
	if len(store.alerts) != 1 {
		t.Errorf("store has %d alerts, want 1", len(store.alerts))
	}
}

func TestAlertService_RunEvaluation_Deduplicates(t *testing.T) {
	store := newMockAlertStore()
	maintStore := &mockMaintenanceStore{underMaintenance: false}

	ev := &mockEvaluator{
		engine: alerts.EngineSQLServer,
		results: []AlertEvaluatorResult{
			{
				RuleName: "sqlserver_blocking",
				Category: "blocking",
				Severity: alerts.SeverityCritical,
				Title:    "Active blocking detected",
				Engine:   alerts.EngineSQLServer,
			},
		},
	}

	svc := NewAlertService(store, maintStore, []AlertEvaluator{ev}, nil)

	// Run twice with the same server ID so deduplication by fingerprint fires
	serverID := uuid.New()
	svc.RunEvaluation(context.Background(), serverID, alerts.EngineSQLServer, "test-instance")
	svc.RunEvaluation(context.Background(), serverID, alerts.EngineSQLServer, "test-instance")

	if len(store.alerts) != 1 {
		t.Errorf("expected 1 deduplicated alert, got %d", len(store.alerts))
	}
	for _, a := range store.alerts {
		if a.HitCount < 2 {
			t.Errorf("hit_count = %d, want >= 2", a.HitCount)
		}
	}
}

func TestAlertService_RunEvaluation_SkipsMaintenanceWindow(t *testing.T) {
	store := newMockAlertStore()
	maintStore := &mockMaintenanceStore{underMaintenance: true}

	ev := &mockEvaluator{
		engine: alerts.EngineSQLServer,
		results: []AlertEvaluatorResult{
			{RuleName: "sqlserver_blocking", Category: "blocking", Severity: alerts.SeverityCritical},
		},
	}

	svc := NewAlertService(store, maintStore, []AlertEvaluator{ev}, nil)
	count, err := svc.RunEvaluation(context.Background(), uuid.New(), alerts.EngineSQLServer, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 alerts during maintenance, got %d", count)
	}
}

func TestAlertService_RunEvaluation_SkipsMismatchedEngine(t *testing.T) {
	store := newMockAlertStore()
	maintStore := &mockMaintenanceStore{underMaintenance: false}

	ev := &mockEvaluator{
		engine: alerts.EnginePostgres,
		results: []AlertEvaluatorResult{
			{RuleName: "pg_replication_lag", Category: "replication", Severity: alerts.SeverityWarning},
		},
	}

	svc := NewAlertService(store, maintStore, []AlertEvaluator{ev}, nil)
	count, _ := svc.RunEvaluation(context.Background(), uuid.New(), alerts.EngineSQLServer, "")
	if count != 0 {
		t.Errorf("expected 0 (wrong engine), got %d", count)
	}
}

func TestAlertService_Acknowledge(t *testing.T) {
	store := newMockAlertStore()
	maintStore := &mockMaintenanceStore{}
	svc := NewAlertService(store, maintStore, nil, nil)

	// Seed an alert
	id := uuid.New()
	store.alerts[id] = alerts.Alert{
		ID:          id,
		Status:      alerts.StatusOpen,
		Severity:    alerts.SeverityWarning,
		FirstSeenAt: time.Now(),
		LastSeenAt:  time.Now(),
		HitCount:    1,
	}

	err := svc.Acknowledge(context.Background(), id, "admin", "investigating")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	a := store.alerts[id]
	if a.Status != alerts.StatusAcknowledged {
		t.Errorf("status = %q, want acknowledged", a.Status)
	}
}

func TestAlertService_Resolve(t *testing.T) {
	store := newMockAlertStore()
	maintStore := &mockMaintenanceStore{}
	svc := NewAlertService(store, maintStore, nil, nil)

	id := uuid.New()
	store.alerts[id] = alerts.Alert{
		ID:       id,
		Status:   alerts.StatusOpen,
		Severity: alerts.SeverityCritical,
	}

	err := svc.Resolve(context.Background(), id, "admin", "root cause fixed")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	a := store.alerts[id]
	if a.Status != alerts.StatusResolved {
		t.Errorf("status = %q, want resolved", a.Status)
	}
}

func TestAlertService_Resolve_AlreadyResolved(t *testing.T) {
	store := newMockAlertStore()
	maintStore := &mockMaintenanceStore{}
	svc := NewAlertService(store, maintStore, nil, nil)

	id := uuid.New()
	now := time.Now()
	prev := "prev"
	store.alerts[id] = alerts.Alert{
		ID:         id,
		Status:     alerts.StatusResolved,
		ResolvedBy: &prev,
		ResolvedAt: &now,
	}

	err := svc.Resolve(context.Background(), id, "admin", "")
	if err != alerts.ErrAlertAlreadyResolved {
		t.Errorf("expected ErrAlertAlreadyResolved, got %v", err)
	}
}

func TestAlertService_Acknowledge_InvalidID(t *testing.T) {
	store := newMockAlertStore()
	svc := NewAlertService(store, &mockMaintenanceStore{}, nil, nil)

	err := svc.Acknowledge(context.Background(), uuid.New(), "admin", "")
	if err == nil {
		t.Error("expected error for invalid UUID")
	}
}
