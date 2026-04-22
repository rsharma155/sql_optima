package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/models"
	"github.com/rsharma155/sql_optima/internal/service"
)

// pgCfg returns a config with one postgres instance for testing.
func pgCfg() *config.Config {
	return &config.Config{
		Instances: []config.Instance{
			{Name: "pg-test", Type: "postgres", Host: "localhost", Port: 5432},
		},
	}
}

// sqlserverCfg returns a config with one SQL Server instance for testing.
func sqlserverCfg() *config.Config {
	return &config.Config{
		Instances: []config.Instance{
			{Name: "ms-test", Type: "sqlserver", Host: "localhost", Port: 1433},
		},
	}
}

// ---- PgssStatus handler tests ----

func TestPgssStatus_MissingInstance(t *testing.T) {
	h := NewPostgresHandlers(&service.MetricsService{}, &config.Config{})
	req := httptest.NewRequest(http.MethodGet, "/api/postgres/pgss/status", nil)
	rr := httptest.NewRecorder()
	h.PgssStatus(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestPgssWorkload_MissingInstance(t *testing.T) {
	h := NewPostgresHandlers(&service.MetricsService{}, &config.Config{})
	req := httptest.NewRequest(http.MethodGet, "/api/postgres/pgss/workload", nil)
	rr := httptest.NewRecorder()
	h.PgssWorkload(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestPgssLatency_MissingInstance(t *testing.T) {
	h := NewPostgresHandlers(&service.MetricsService{}, &config.Config{})
	req := httptest.NewRequest(http.MethodGet, "/api/postgres/pgss/latency", nil)
	rr := httptest.NewRecorder()
	h.PgssLatency(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestPgssTop_MissingInstance(t *testing.T) {
	h := NewPostgresHandlers(&service.MetricsService{}, &config.Config{})
	req := httptest.NewRequest(http.MethodGet, "/api/postgres/pgss/top", nil)
	rr := httptest.NewRecorder()
	h.PgssTop(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestPgssRegressions_MissingInstance(t *testing.T) {
	h := NewPostgresHandlers(&service.MetricsService{}, &config.Config{})
	req := httptest.NewRequest(http.MethodGet, "/api/postgres/pgss/regressions", nil)
	rr := httptest.NewRecorder()
	h.PgssRegressions(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestPgssSummary_MissingInstance(t *testing.T) {
	h := NewPostgresHandlers(&service.MetricsService{}, &config.Config{})
	req := httptest.NewRequest(http.MethodGet, "/api/postgres/pgss/summary", nil)
	rr := httptest.NewRecorder()
	h.PgssSummary(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestPgssSummary_InstanceNotFound(t *testing.T) {
	h := NewPostgresHandlers(&service.MetricsService{}, pgCfg())
	req := httptest.NewRequest(http.MethodGet, "/api/postgres/pgss/summary?instance=nonexistent", nil)
	rr := httptest.NewRecorder()
	h.PgssSummary(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("want %d, got %d", http.StatusNotFound, rr.Code)
	}
}

func TestPgssSummary_WrongType(t *testing.T) {
	h := NewPostgresHandlers(&service.MetricsService{}, sqlserverCfg())
	req := httptest.NewRequest(http.MethodGet, "/api/postgres/pgss/summary?instance=ms-test", nil)
	rr := httptest.NewRecorder()
	h.PgssSummary(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

// ---- parseTimeRange tests ----

func TestParseTimeRange_Defaults(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	from, to := parseTimeRange(req)
	diff := to.Sub(from)
	if diff < 59*time.Minute || diff > 61*time.Minute {
		t.Fatalf("default range should be ~1 hour, got %v", diff)
	}
}

func TestParseTimeRange_CustomValues(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test?from=2026-04-17T10:00:00Z&to=2026-04-17T12:00:00Z", nil)
	from, to := parseTimeRange(req)
	if from.Hour() != 10 || to.Hour() != 12 {
		t.Fatalf("expected 10:00-12:00, got %v - %v", from, to)
	}
}

func TestParseTimeRange_SwapsInverted(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test?from=2026-04-17T14:00:00Z&to=2026-04-17T10:00:00Z", nil)
	from, to := parseTimeRange(req)
	if from.After(to) {
		t.Fatal("from should not be after to when swapped")
	}
}

func TestParseTimeRange_CapsAt7Days(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test?from=2020-01-01T00:00:00Z&to=2026-04-17T00:00:00Z", nil)
	from, to := parseTimeRange(req)
	diff := to.Sub(from)
	if diff > 7*24*time.Hour+time.Second {
		t.Fatalf("range should be capped at 7 days, got %v", diff)
	}
}

func TestParseTimeRange_InvalidIgnored(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test?from=not-a-date&to=also-bad", nil)
	from, to := parseTimeRange(req)
	// Should fall back to defaults (1 hour range)
	diff := to.Sub(from)
	if diff < 59*time.Minute || diff > 61*time.Minute {
		t.Fatalf("invalid input should fall back to ~1h default, got %v", diff)
	}
}

// ---- sort allowlist test ----

func TestPgssTop_InvalidSortDefaultsToTotalTime(t *testing.T) {
	h := NewPostgresHandlers(&service.MetricsService{}, &config.Config{})
	// Passing a malicious sort param (URL-encoded)
	req := httptest.NewRequest(http.MethodGet, "/api/postgres/pgss/top?instance=test&sort=DROP+TABLE", nil)
	rr := httptest.NewRecorder()
	h.PgssTop(rr, req)
	// Should get validation error (instance not found), not panic or injection
	if rr.Code == http.StatusInternalServerError {
		t.Fatal("should not get 500 for malicious sort param")
	}
	var body map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&body)
	// Instance validation should fail first, not sort
	if _, ok := body["error"]; !ok {
		t.Fatal("expected error response")
	}
}

// ---- Instance type mismatch tests (SQL Server instance should be rejected by PGSS) ----

func TestPgssStatus_WrongInstanceType(t *testing.T) {
	h := NewPostgresHandlers(&service.MetricsService{}, sqlserverCfg())
	req := httptest.NewRequest(http.MethodGet, "/api/postgres/pgss/status?instance=ms-test", nil)
	rr := httptest.NewRecorder()
	h.PgssStatus(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want %d for wrong type, got %d", http.StatusBadRequest, rr.Code)
	}
	var body map[string]string
	json.NewDecoder(rr.Body).Decode(&body)
	if body["error"] != "instance is not postgres" {
		t.Fatalf("want 'instance is not postgres', got %q", body["error"])
	}
}

func TestPgssWorkload_WrongInstanceType(t *testing.T) {
	h := NewPostgresHandlers(&service.MetricsService{}, sqlserverCfg())
	req := httptest.NewRequest(http.MethodGet, "/api/postgres/pgss/workload?instance=ms-test", nil)
	rr := httptest.NewRecorder()
	h.PgssWorkload(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestPgssLatency_WrongInstanceType(t *testing.T) {
	h := NewPostgresHandlers(&service.MetricsService{}, sqlserverCfg())
	req := httptest.NewRequest(http.MethodGet, "/api/postgres/pgss/latency?instance=ms-test", nil)
	rr := httptest.NewRecorder()
	h.PgssLatency(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestPgssTop_WrongInstanceType(t *testing.T) {
	h := NewPostgresHandlers(&service.MetricsService{}, sqlserverCfg())
	req := httptest.NewRequest(http.MethodGet, "/api/postgres/pgss/top?instance=ms-test", nil)
	rr := httptest.NewRecorder()
	h.PgssTop(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestPgssRegressions_WrongInstanceType(t *testing.T) {
	h := NewPostgresHandlers(&service.MetricsService{}, sqlserverCfg())
	req := httptest.NewRequest(http.MethodGet, "/api/postgres/pgss/regressions?instance=ms-test", nil)
	rr := httptest.NewRecorder()
	h.PgssRegressions(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

// ---- Instance not found (known name but not in config) ----

func TestPgssStatus_InstanceNotFound(t *testing.T) {
	h := NewPostgresHandlers(&service.MetricsService{}, &config.Config{})
	req := httptest.NewRequest(http.MethodGet, "/api/postgres/pgss/status?instance=ghost", nil)
	rr := httptest.NewRecorder()
	h.PgssStatus(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("want %d, got %d", http.StatusNotFound, rr.Code)
	}
}

func TestPgssWorkload_InstanceNotFound(t *testing.T) {
	h := NewPostgresHandlers(&service.MetricsService{}, &config.Config{})
	req := httptest.NewRequest(http.MethodGet, "/api/postgres/pgss/workload?instance=ghost", nil)
	rr := httptest.NewRecorder()
	h.PgssWorkload(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("want %d, got %d", http.StatusNotFound, rr.Code)
	}
}

// ---- XSS / SQL injection in instance name ----

func TestPgss_XSSInInstanceName(t *testing.T) {
	h := NewPostgresHandlers(&service.MetricsService{}, pgCfg())
	req := httptest.NewRequest(http.MethodGet, "/api/postgres/pgss/status?instance=%3Cscript%3Ealert(1)%3C%2Fscript%3E", nil)
	rr := httptest.NewRecorder()
	h.PgssStatus(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("XSS instance name should be rejected, got %d", rr.Code)
	}
}

func TestPgss_SQLInjectionInInstanceName(t *testing.T) {
	h := NewPostgresHandlers(&service.MetricsService{}, pgCfg())
	req := httptest.NewRequest(http.MethodGet, "/api/postgres/pgss/workload?instance=pg';DROP+TABLE+users;--", nil)
	rr := httptest.NewRecorder()
	h.PgssWorkload(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("SQL injection instance name should be rejected, got %d", rr.Code)
	}
}

// ---- Configured postgres instance with nil tsLogger returns 200 + empty data ----

func TestPgssWorkload_NilTsLogger_Returns200(t *testing.T) {
	h := NewPostgresHandlers(&service.MetricsService{}, pgCfg())
	req := httptest.NewRequest(http.MethodGet, "/api/postgres/pgss/workload?instance=pg-test", nil)
	rr := httptest.NewRecorder()
	h.PgssWorkload(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d; body: %s", rr.Code, rr.Body.String())
	}
	var resp models.PgssWorkloadResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp.Instance != "pg-test" {
		t.Fatalf("want instance=pg-test, got %q", resp.Instance)
	}
	if len(resp.Points) != 0 {
		t.Fatalf("want nil/empty points with nil tsLogger, got %d", len(resp.Points))
	}
}

func TestPgssLatency_NilTsLogger_Returns200(t *testing.T) {
	h := NewPostgresHandlers(&service.MetricsService{}, pgCfg())
	req := httptest.NewRequest(http.MethodGet, "/api/postgres/pgss/latency?instance=pg-test", nil)
	rr := httptest.NewRecorder()
	h.PgssLatency(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d; body: %s", rr.Code, rr.Body.String())
	}
	var resp models.PgssLatencyResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp.Instance != "pg-test" {
		t.Fatalf("want instance=pg-test, got %q", resp.Instance)
	}
}

func TestPgssTop_NilTsLogger_Returns200(t *testing.T) {
	h := NewPostgresHandlers(&service.MetricsService{}, pgCfg())
	req := httptest.NewRequest(http.MethodGet, "/api/postgres/pgss/top?instance=pg-test", nil)
	rr := httptest.NewRecorder()
	h.PgssTop(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d; body: %s", rr.Code, rr.Body.String())
	}
	var resp models.PgssTopQueriesResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp.Instance != "pg-test" {
		t.Fatalf("want instance=pg-test, got %q", resp.Instance)
	}
	if resp.SortBy != "total_time" {
		t.Fatalf("want default sort=total_time, got %q", resp.SortBy)
	}
}

func TestPgssRegressions_NilTsLogger_Returns200(t *testing.T) {
	h := NewPostgresHandlers(&service.MetricsService{}, pgCfg())
	req := httptest.NewRequest(http.MethodGet, "/api/postgres/pgss/regressions?instance=pg-test", nil)
	rr := httptest.NewRecorder()
	h.PgssRegressions(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d; body: %s", rr.Code, rr.Body.String())
	}
	var resp models.PgssRegressionsResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp.Instance != "pg-test" {
		t.Fatalf("want instance=pg-test, got %q", resp.Instance)
	}
}

// ---- Sort allowlist edge cases ----

func TestPgssTop_AllValidSortValues(t *testing.T) {
	h := NewPostgresHandlers(&service.MetricsService{}, pgCfg())
	validSorts := []string{"total_time", "mean_time", "calls", "io", "temp", "wal", "planning"}
	for _, sort := range validSorts {
		req := httptest.NewRequest(http.MethodGet, "/api/postgres/pgss/top?instance=pg-test&sort="+sort, nil)
		rr := httptest.NewRecorder()
		h.PgssTop(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("sort=%s: want 200, got %d", sort, rr.Code)
		}
		var resp models.PgssTopQueriesResponse
		json.NewDecoder(rr.Body).Decode(&resp)
		if resp.SortBy != sort {
			t.Fatalf("want sort_by=%s, got %q", sort, resp.SortBy)
		}
	}
}

func TestPgssTop_InvalidSortFallsBackToTotalTime(t *testing.T) {
	h := NewPostgresHandlers(&service.MetricsService{}, pgCfg())
	req := httptest.NewRequest(http.MethodGet, "/api/postgres/pgss/top?instance=pg-test&sort=invalid_col", nil)
	rr := httptest.NewRecorder()
	h.PgssTop(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rr.Code)
	}
	var resp models.PgssTopQueriesResponse
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp.SortBy != "total_time" {
		t.Fatalf("invalid sort should fall back to total_time, got %q", resp.SortBy)
	}
}

func TestPgssTop_EmptySortDefaultsToTotalTime(t *testing.T) {
	h := NewPostgresHandlers(&service.MetricsService{}, pgCfg())
	req := httptest.NewRequest(http.MethodGet, "/api/postgres/pgss/top?instance=pg-test", nil)
	rr := httptest.NewRecorder()
	h.PgssTop(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rr.Code)
	}
	var resp models.PgssTopQueriesResponse
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp.SortBy != "total_time" {
		t.Fatalf("empty sort should default to total_time, got %q", resp.SortBy)
	}
}

// ---- parseTimeRange additional edge cases ----

func TestParseTimeRange_OnlyFromProvided(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test?from=2026-04-17T10:00:00Z", nil)
	from, to := parseTimeRange(req)
	if from.Hour() != 10 {
		t.Fatalf("expected from=10:00, got %v", from)
	}
	// to should be ~now (within 2 seconds)
	if time.Since(to) > 2*time.Second {
		t.Fatalf("expected to≈now, got %v", to)
	}
}

func TestParseTimeRange_OnlyToProvided(t *testing.T) {
	// When only "to" is provided in the past, fromT defaults to now()-1h which
	// is after to, so the swap fires and the 7-day cap may apply.
	// Just verify from < to and range is valid.
	req := httptest.NewRequest(http.MethodGet, "/test?to=2026-04-17T12:00:00Z", nil)
	from, to := parseTimeRange(req)
	if from.After(to) {
		t.Fatalf("from should not be after to, got from=%v to=%v", from, to)
	}
}

func TestParseTimeRange_WhitespaceTrimed(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test?from=%202026-04-17T10:00:00Z%20&to=%202026-04-17T12:00:00Z%20", nil)
	from, to := parseTimeRange(req)
	if from.Hour() != 10 || to.Hour() != 12 {
		t.Fatalf("expected trimmed times 10:00-12:00, got %v - %v", from, to)
	}
}

// ---- Content-Type header tests ----

func TestPgssWorkload_SetsJSONContentType(t *testing.T) {
	h := NewPostgresHandlers(&service.MetricsService{}, pgCfg())
	req := httptest.NewRequest(http.MethodGet, "/api/postgres/pgss/workload?instance=pg-test", nil)
	rr := httptest.NewRecorder()
	h.PgssWorkload(rr, req)
	ct := rr.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Fatalf("want Content-Type=application/json, got %q", ct)
	}
}
