package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/service"
)

func TestQueryAnalysisSummary_MissingInstance(t *testing.T) {
	h := NewMssqlQueryAnalysisHandlers(&service.MetricsService{}, mssqlCfg())
	req := httptest.NewRequest(http.MethodGet, "/api/mssql/query-analysis/summary", nil)
	rr := httptest.NewRecorder()
	h.Summary(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestQueryAnalysisSummary_InstanceNotFound(t *testing.T) {
	h := NewMssqlQueryAnalysisHandlers(&service.MetricsService{}, mssqlCfg())
	req := httptest.NewRequest(http.MethodGet, "/api/mssql/query-analysis/summary?instance=nonexistent", nil)
	rr := httptest.NewRecorder()
	h.Summary(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("want %d, got %d", http.StatusNotFound, rr.Code)
	}
}

func TestQueryAnalysisSummary_WrongType(t *testing.T) {
	cfg := &config.Config{Instances: []config.Instance{
		{Name: "pg-test", Type: "postgres", Host: "localhost", Port: 5432},
	}}
	h := NewMssqlQueryAnalysisHandlers(&service.MetricsService{}, cfg)
	req := httptest.NewRequest(http.MethodGet, "/api/mssql/query-analysis/summary?instance=pg-test", nil)
	rr := httptest.NewRecorder()
	h.Summary(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestQueryAnalysisRegressions_MissingInstance(t *testing.T) {
	h := NewMssqlQueryAnalysisHandlers(&service.MetricsService{}, mssqlCfg())
	req := httptest.NewRequest(http.MethodGet, "/api/mssql/query-analysis/regressions", nil)
	rr := httptest.NewRecorder()
	h.Regressions(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestQueryAnalysisRegressions_InstanceNotFound(t *testing.T) {
	h := NewMssqlQueryAnalysisHandlers(&service.MetricsService{}, mssqlCfg())
	req := httptest.NewRequest(http.MethodGet, "/api/mssql/query-analysis/regressions?instance=nonexistent", nil)
	rr := httptest.NewRecorder()
	h.Regressions(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("want %d, got %d", http.StatusNotFound, rr.Code)
	}
}

func TestQueryAnalysisRegressions_OK_NilTsLogger(t *testing.T) {
	h := NewMssqlQueryAnalysisHandlers(&service.MetricsService{}, mssqlCfg())
	req := httptest.NewRequest(http.MethodGet, "/api/mssql/query-analysis/regressions?instance=ms-test", nil)
	rr := httptest.NewRecorder()
	h.Regressions(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("want %d, got %d: %s", http.StatusOK, rr.Code, rr.Body.String())
	}
	var body map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["instance"] != "ms-test" {
		t.Fatalf("want instance ms-test, got %v", body["instance"])
	}
}

func TestQueryAnalysisPlanInstability_MissingInstance(t *testing.T) {
	h := NewMssqlQueryAnalysisHandlers(&service.MetricsService{}, mssqlCfg())
	req := httptest.NewRequest(http.MethodGet, "/api/mssql/query-analysis/plan-instability", nil)
	rr := httptest.NewRecorder()
	h.PlanInstability(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestQueryAnalysisTopQueries_MissingInstance(t *testing.T) {
	h := NewMssqlQueryAnalysisHandlers(&service.MetricsService{}, mssqlCfg())
	req := httptest.NewRequest(http.MethodGet, "/api/mssql/query-analysis/top-queries", nil)
	rr := httptest.NewRecorder()
	h.TopQueries(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestQueryAnalysisQueryPlans_MissingParams(t *testing.T) {
	h := NewMssqlQueryAnalysisHandlers(&service.MetricsService{}, mssqlCfg())
	req := httptest.NewRequest(http.MethodGet, "/api/mssql/query-analysis/query-plans?instance=ms-test", nil)
	rr := httptest.NewRecorder()
	h.QueryPlans(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestQueryAnalysisWaitStats_MissingParams(t *testing.T) {
	h := NewMssqlQueryAnalysisHandlers(&service.MetricsService{}, mssqlCfg())
	req := httptest.NewRequest(http.MethodGet, "/api/mssql/query-analysis/query-wait-stats?instance=ms-test", nil)
	rr := httptest.NewRecorder()
	h.QueryWaitStats(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want %d, got %d", http.StatusBadRequest, rr.Code)
	}
}
