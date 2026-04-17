// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Unit tests for alert engine HTTP handlers.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"

	"github.com/rsharma155/sql_optima/internal/domain/alerts"
	"github.com/rsharma155/sql_optima/internal/middleware"
	"github.com/rsharma155/sql_optima/internal/service"
)

// ── mock stores for handler tests ──────────────────────────────

type handlerMockAlertStore struct {
	alerts map[uuid.UUID]alerts.Alert
}

func newHandlerMockAlertStore() *handlerMockAlertStore {
	return &handlerMockAlertStore{alerts: make(map[uuid.UUID]alerts.Alert)}
}

func (m *handlerMockAlertStore) Upsert(_ context.Context, a alerts.Alert) (alerts.Alert, error) {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	m.alerts[a.ID] = a
	return a, nil
}

func (m *handlerMockAlertStore) GetByID(_ context.Context, id uuid.UUID) (alerts.Alert, error) {
	a, ok := m.alerts[id]
	if !ok {
		return a, alerts.ErrAlertNotFound
	}
	return a, nil
}

func (m *handlerMockAlertStore) List(_ context.Context, f alerts.AlertFilter) ([]alerts.Alert, int, error) {
	var result []alerts.Alert
	for _, a := range m.alerts {
		if f.Status != "" && a.Status != f.Status {
			continue
		}
		if f.InstanceName != "" && a.InstanceName != f.InstanceName {
			continue
		}
		result = append(result, a)
	}
	return result, len(result), nil
}

func (m *handlerMockAlertStore) UpdateStatus(_ context.Context, id uuid.UUID, status alerts.Status, actor, _ string, at time.Time) error {
	a, ok := m.alerts[id]
	if !ok {
		return alerts.ErrAlertNotFound
	}
	a.Status = status
	a.UpdatedAt = at
	m.alerts[id] = a
	return nil
}

func (m *handlerMockAlertStore) CountOpen(_ context.Context, instanceName string) (int, error) {
	count := 0
	for _, a := range m.alerts {
		if a.InstanceName == instanceName && a.Status != alerts.StatusResolved {
			count++
		}
	}
	return count, nil
}

type handlerMockMaintenanceStore struct{}

func (m *handlerMockMaintenanceStore) Create(_ context.Context, mw alerts.MaintenanceWindow) (alerts.MaintenanceWindow, error) {
	mw.ID = uuid.New()
	mw.CreatedAt = time.Now()
	return mw, nil
}
func (m *handlerMockMaintenanceStore) IsUnderMaintenance(_ context.Context, _ string, _ alerts.Engine, _ time.Time) (bool, error) {
	return false, nil
}
func (m *handlerMockMaintenanceStore) ListActive(_ context.Context, _ time.Time) ([]alerts.MaintenanceWindow, error) {
	return nil, nil
}
func (m *handlerMockMaintenanceStore) Delete(_ context.Context, _ uuid.UUID) error {
	return nil
}

func setupHandlers() (*AlertHandlers, *handlerMockAlertStore) {
	store := newHandlerMockAlertStore()
	maintStore := &handlerMockMaintenanceStore{}
	svc := service.NewAlertService(store, maintStore, nil)
	h := NewAlertHandlers(svc, store, maintStore)
	return h, store
}

// ── tests ──────────────────────────────────────────────────────

func TestListAlerts_Empty(t *testing.T) {
	h, _ := setupHandlers()

	req := httptest.NewRequest(http.MethodGet, "/api/alerts", nil)
	rr := httptest.NewRecorder()
	h.ListAlerts(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["success"] != true {
		t.Error("expected success=true")
	}
}

func TestListAlerts_WithData(t *testing.T) {
	h, store := setupHandlers()

	id := uuid.New()
	store.alerts[id] = alerts.Alert{
		ID:           id,
		InstanceName: "prod-db-01",
		Engine:       alerts.EngineSQLServer,
		Severity:     alerts.SeverityCritical,
		Status:       alerts.StatusOpen,
		Category:     "blocking",
		Title:        "Active blocking",
	}

	req := httptest.NewRequest(http.MethodGet, "/api/alerts?instance=prod-db-01", nil)
	rr := httptest.NewRecorder()
	h.ListAlerts(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	data := resp["data"].(map[string]interface{})
	total := data["total"].(float64)
	if total != 1 {
		t.Errorf("total = %v, want 1", total)
	}
}

func TestGetAlert_NotFound(t *testing.T) {
	h, _ := setupHandlers()

	req := httptest.NewRequest(http.MethodGet, "/api/alerts/"+uuid.New().String(), nil)
	req = mux.SetURLVars(req, map[string]string{"id": uuid.New().String()})
	rr := httptest.NewRecorder()
	h.GetAlert(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusNotFound)
	}
}

func TestGetAlert_Found(t *testing.T) {
	h, store := setupHandlers()

	id := uuid.New()
	store.alerts[id] = alerts.Alert{
		ID:       id,
		Title:    "Test alert",
		Status:   alerts.StatusOpen,
		Severity: alerts.SeverityWarning,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/alerts/"+id.String(), nil)
	req = mux.SetURLVars(req, map[string]string{"id": id.String()})
	rr := httptest.NewRecorder()
	h.GetAlert(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestAcknowledgeAlert(t *testing.T) {
	h, store := setupHandlers()

	id := uuid.New()
	store.alerts[id] = alerts.Alert{
		ID:       id,
		Status:   alerts.StatusOpen,
		Severity: alerts.SeverityWarning,
	}

	body, _ := json.Marshal(map[string]string{"reason": "investigating"})
	req := httptest.NewRequest(http.MethodPost, "/api/alerts/"+id.String()+"/acknowledge", bytes.NewReader(body))
	req = mux.SetURLVars(req, map[string]string{"id": id.String()})
	rr := httptest.NewRecorder()
	h.AcknowledgeAlert(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if store.alerts[id].Status != alerts.StatusAcknowledged {
		t.Errorf("alert status = %q, want acknowledged", store.alerts[id].Status)
	}
}

func TestResolveAlert(t *testing.T) {
	h, store := setupHandlers()

	id := uuid.New()
	store.alerts[id] = alerts.Alert{
		ID:       id,
		Status:   alerts.StatusOpen,
		Severity: alerts.SeverityCritical,
	}

	body, _ := json.Marshal(map[string]string{"reason": "fixed"})
	req := httptest.NewRequest(http.MethodPost, "/api/alerts/"+id.String()+"/resolve", bytes.NewReader(body))
	req = mux.SetURLVars(req, map[string]string{"id": id.String()})
	rr := httptest.NewRecorder()
	h.ResolveAlert(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if store.alerts[id].Status != alerts.StatusResolved {
		t.Errorf("alert status = %q, want resolved", store.alerts[id].Status)
	}
}

func TestResolveAlert_AlreadyResolved(t *testing.T) {
	h, store := setupHandlers()

	id := uuid.New()
	now := time.Now()
	prev := "prev"
	store.alerts[id] = alerts.Alert{
		ID:         id,
		Status:     alerts.StatusResolved,
		ResolvedBy: &prev,
		ResolvedAt: &now,
	}

	body, _ := json.Marshal(map[string]string{"reason": ""})
	req := httptest.NewRequest(http.MethodPost, "/api/alerts/"+id.String()+"/resolve", bytes.NewReader(body))
	req = mux.SetURLVars(req, map[string]string{"id": id.String()})
	rr := httptest.NewRecorder()
	h.ResolveAlert(rr, req)

	if rr.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusConflict)
	}
}

func TestCountOpen(t *testing.T) {
	h, store := setupHandlers()

	for i := 0; i < 3; i++ {
		id := uuid.New()
		store.alerts[id] = alerts.Alert{
			ID:           id,
			InstanceName: "prod-db-01",
			Status:       alerts.StatusOpen,
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/alerts/count?instance=prod-db-01", nil)
	rr := httptest.NewRecorder()
	h.CountOpen(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	data := resp["data"].(map[string]interface{})
	if data["open_count"].(float64) != 3 {
		t.Errorf("open_count = %v, want 3", data["open_count"])
	}
}

func TestCountOpen_MissingInstance(t *testing.T) {
	h, _ := setupHandlers()

	req := httptest.NewRequest(http.MethodGet, "/api/alerts/count", nil)
	rr := httptest.NewRecorder()
	h.CountOpen(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestCreateMaintenanceWindow(t *testing.T) {
	h, _ := setupHandlers()

	body, _ := json.Marshal(map[string]interface{}{
		"instance_name": "prod-db-01",
		"engine":        "sqlserver",
		"reason":        "Scheduled patching",
		"starts_at":     time.Now().Format(time.RFC3339),
		"ends_at":       time.Now().Add(2 * time.Hour).Format(time.RFC3339),
		"created_by":    "admin",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/alerts/maintenance", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	h.CreateMaintenanceWindow(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusCreated)
	}
}

func TestCreateMaintenanceWindow_InvalidEngine(t *testing.T) {
	h, _ := setupHandlers()

	body, _ := json.Marshal(map[string]interface{}{
		"instance_name": "prod-db-01",
		"engine":        "mysql",
		"starts_at":     time.Now().Format(time.RFC3339),
		"ends_at":       time.Now().Add(2 * time.Hour).Format(time.RFC3339),
	})

	req := httptest.NewRequest(http.MethodPost, "/api/alerts/maintenance", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	h.CreateMaintenanceWindow(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestGetAlert_InvalidID(t *testing.T) {
	h, _ := setupHandlers()

	req := httptest.NewRequest(http.MethodGet, "/api/alerts/not-a-uuid", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "not-a-uuid"})
	rr := httptest.NewRecorder()
	h.GetAlert(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestAcknowledgeAlert_InvalidID(t *testing.T) {
	h, _ := setupHandlers()

	body, _ := json.Marshal(map[string]string{"reason": "investigating"})
	req := httptest.NewRequest(http.MethodPost, "/api/alerts/not-a-uuid/acknowledge", bytes.NewReader(body))
	req = mux.SetURLVars(req, map[string]string{"id": "not-a-uuid"})
	rr := httptest.NewRecorder()
	h.AcknowledgeAlert(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestAcknowledgeAlert_MalformedBody(t *testing.T) {
	h, store := setupHandlers()

	id := uuid.New()
	store.alerts[id] = alerts.Alert{
		ID:       id,
		Status:   alerts.StatusOpen,
		Severity: alerts.SeverityWarning,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/alerts/"+id.String()+"/acknowledge", bytes.NewReader([]byte("{invalid")))
	req = mux.SetURLVars(req, map[string]string{"id": id.String()})
	rr := httptest.NewRecorder()
	h.AcknowledgeAlert(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestAcknowledgeAlert_NotFound(t *testing.T) {
	h, _ := setupHandlers()

	id := uuid.New()
	body, _ := json.Marshal(map[string]string{"reason": "investigating"})
	req := httptest.NewRequest(http.MethodPost, "/api/alerts/"+id.String()+"/acknowledge", bytes.NewReader(body))
	req = mux.SetURLVars(req, map[string]string{"id": id.String()})
	rr := httptest.NewRecorder()
	h.AcknowledgeAlert(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusNotFound)
	}
}

func TestResolveAlert_InvalidID(t *testing.T) {
	h, _ := setupHandlers()

	body, _ := json.Marshal(map[string]string{"reason": "fixed"})
	req := httptest.NewRequest(http.MethodPost, "/api/alerts/not-a-uuid/resolve", bytes.NewReader(body))
	req = mux.SetURLVars(req, map[string]string{"id": "not-a-uuid"})
	rr := httptest.NewRecorder()
	h.ResolveAlert(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestResolveAlert_MalformedBody(t *testing.T) {
	h, store := setupHandlers()

	id := uuid.New()
	store.alerts[id] = alerts.Alert{
		ID:       id,
		Status:   alerts.StatusOpen,
		Severity: alerts.SeverityCritical,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/alerts/"+id.String()+"/resolve", bytes.NewReader([]byte("{invalid")))
	req = mux.SetURLVars(req, map[string]string{"id": id.String()})
	rr := httptest.NewRecorder()
	h.ResolveAlert(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestResolveAlert_NotFound(t *testing.T) {
	h, _ := setupHandlers()

	id := uuid.New()
	body, _ := json.Marshal(map[string]string{"reason": "fixed"})
	req := httptest.NewRequest(http.MethodPost, "/api/alerts/"+id.String()+"/resolve", bytes.NewReader(body))
	req = mux.SetURLVars(req, map[string]string{"id": id.String()})
	rr := httptest.NewRecorder()
	h.ResolveAlert(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusNotFound)
	}
}

func TestAcknowledgeAlert_UsesAuthClaims(t *testing.T) {
	h, store := setupHandlers()

	id := uuid.New()
	store.alerts[id] = alerts.Alert{
		ID:       id,
		Status:   alerts.StatusOpen,
		Severity: alerts.SeverityWarning,
	}

	body, _ := json.Marshal(map[string]string{"reason": "investigating"})
	req := httptest.NewRequest(http.MethodPost, "/api/alerts/"+id.String()+"/acknowledge", bytes.NewReader(body))
	req = mux.SetURLVars(req, map[string]string{"id": id.String()})

	// Inject auth claims into request context
	ctx := middleware.WithAuthClaims(req.Context(), &middleware.AuthClaims{Username: "dba_user"})
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	h.AcknowledgeAlert(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestAcknowledgeAlert_FallbackToSystem(t *testing.T) {
	h, store := setupHandlers()

	id := uuid.New()
	store.alerts[id] = alerts.Alert{
		ID:       id,
		Status:   alerts.StatusOpen,
		Severity: alerts.SeverityWarning,
	}

	body, _ := json.Marshal(map[string]string{"reason": "investigating"})
	req := httptest.NewRequest(http.MethodPost, "/api/alerts/"+id.String()+"/acknowledge", bytes.NewReader(body))
	req = mux.SetURLVars(req, map[string]string{"id": id.String()})
	// No auth claims injected — should fallback to "system"

	rr := httptest.NewRecorder()
	h.AcknowledgeAlert(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}
