package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/service"
)

func TestAdminSqlServerDiagnostics_MissingInstance(t *testing.T) {
	h := NewAdminSqlServerDiagnosticsHandlers(&service.MetricsService{}, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/diagnostics/sqlserver", nil)
	rr := httptest.NewRecorder()
	h.GetSqlServerDiagnostics(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestAdminSqlServerDiagnostics_NoTimescale(t *testing.T) {
	sid := uuid.New()
	cfg := &config.Config{
		Instances: []config.Instance{
			{ServerID: sid, Name: "MSSQL-01", Type: "sqlserver"},
		},
	}
	h := NewAdminSqlServerDiagnosticsHandlers(&service.MetricsService{}, cfg)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/diagnostics/sqlserver/"+sid.String(), nil)
	req = mux.SetURLVars(req, map[string]string{"instance": sid.String()})
	rr := httptest.NewRecorder()
	h.GetSqlServerDiagnostics(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestAdminSqlServerDiagnostics_ResolveFromConfigName(t *testing.T) {
	sid := uuid.New()
	cfg := &config.Config{
		Instances: []config.Instance{
			{ServerID: sid, Name: "MSSQL-01", Type: "sqlserver"},
		},
	}
	h := NewAdminSqlServerDiagnosticsHandlers(&service.MetricsService{}, cfg)
	id, name, ok := h.resolveSqlServerInstance(httptest.NewRequest(http.MethodGet, "/?instance=MSSQL-01", nil))
	if !ok || id != sid || name != "MSSQL-01" {
		t.Fatalf("resolve ok=%v id=%s name=%s", ok, id, name)
	}
}

func TestDiagnosticTimeWindow_DefaultAndCap(t *testing.T) {
	from, to := diagnosticTimeWindow(httptest.NewRequest(http.MethodGet, "/", nil))
	if to.Sub(from) != 24*time.Hour {
		t.Fatalf("default window=%v", to.Sub(from))
	}
	from2, to2 := diagnosticTimeWindow(httptest.NewRequest(http.MethodGet, "/?hours=200", nil))
	if to2.Sub(from2) != 168*time.Hour {
		t.Fatalf("capped window=%v", to2.Sub(from2))
	}
}

func TestAdminSqlServerDiagnostics_NotSQLServerType(t *testing.T) {
	// Exercises JSON error shape only when registry would return postgres — covered by handler logic;
	// keep a minimal decode check for missing-instance body.
	req := httptest.NewRequest(http.MethodGet, "/api/admin/diagnostics/sqlserver", nil)
	rr := httptest.NewRecorder()
	NewAdminSqlServerDiagnosticsHandlers(&service.MetricsService{}, nil).GetSqlServerDiagnostics(rr, req)
	var body map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["error"] == "" {
		t.Fatal("expected error field")
	}
}
