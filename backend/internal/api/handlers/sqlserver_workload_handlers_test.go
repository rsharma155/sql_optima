package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rsharma155/sql_optima/internal/domain"
	"github.com/rsharma155/sql_optima/internal/service"
)

func TestWorkloadSummary_MissingInstance(t *testing.T) {
	h := NewSqlServerWorkloadHandlers(&service.MetricsService{}, sqlserverCfg())
	req := httptest.NewRequest(http.MethodGet, "/api/sqlserver/workload/summary", nil)
	rr := httptest.NewRecorder()
	h.GetSummary(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestWorkloadSummary_InvalidInstance(t *testing.T) {
	h := NewSqlServerWorkloadHandlers(&service.MetricsService{}, sqlserverCfg())
	req := httptest.NewRequest(http.MethodGet, "/api/sqlserver/workload/summary?instance=nonexistent", nil)
	rr := httptest.NewRecorder()
	h.GetSummary(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestParseWorkloadFilterParams(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/?database=AdventureWorks&exclude_system=false", nil)
	db, auto, ex := parseWorkloadFilterParams(req)
	if db != "AdventureWorks" || auto || ex {
		t.Fatalf("unexpected params: db=%q auto=%v ex=%v", db, auto, ex)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/?database=all", nil)
	db2, auto2, _ := parseWorkloadFilterParams(req2)
	if db2 != "" || auto2 {
		t.Fatalf("want all-databases scope, got db=%q auto=%v", db2, auto2)
	}
}

func TestWorkloadTopQueries_LimitParam(t *testing.T) {
	h := NewSqlServerWorkloadHandlers(&service.MetricsService{}, sqlserverCfg())
	req := httptest.NewRequest(http.MethodGet, "/api/sqlserver/workload/top-queries?instance=nonexistent&limit=50", nil)
	rr := httptest.NewRecorder()
	h.GetTopOffenders(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for missing instance, got %d", rr.Code)
	}
}

func TestWorkloadSummary_TimescaleHeaderWhenNoLogger(t *testing.T) {
	cfg := sqlserverCfg()
	h := NewSqlServerWorkloadHandlers(&service.MetricsService{}, cfg)
	inst := cfg.Instances[0].Name
	req := httptest.NewRequest(http.MethodGet, "/api/sqlserver/workload/summary?instance="+inst, nil)
	rr := httptest.NewRecorder()
	h.GetSummary(rr, req)
	if rr.Header().Get("X-Data-Source") != "timescale" {
		t.Fatalf("expected X-Data-Source=timescale, got %q", rr.Header().Get("X-Data-Source"))
	}
	var body domain.SqlServerWorkloadSummary
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
}
