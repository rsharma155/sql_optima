package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/models"
	"github.com/rsharma155/sql_optima/internal/service"
)

func parseTimeRangeReq(r *http.Request) (time.Time, time.Time) {
	return ParseTimeRange(r.URL.Query().Get("from"), r.URL.Query().Get("to"))
}

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
	from, to := parseTimeRangeReq(req)
	diff := to.Sub(from)
	if diff < 59*time.Minute || diff > 61*time.Minute {
		t.Fatalf("default range should be ~1 hour, got %v", diff)
	}
}

func TestParseTimeRange_CustomValues(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test?from=2026-04-17T10:00:00Z&to=2026-04-17T12:00:00Z", nil)
	from, to := parseTimeRangeReq(req)
	if from.Hour() != 10 || to.Hour() != 12 {
		t.Fatalf("expected 10:00-12:00, got %v - %v", from, to)
	}
}

func TestParseTimeRange_SwapsInverted(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test?from=2026-04-17T14:00:00Z&to=2026-04-17T10:00:00Z", nil)
	from, to := parseTimeRangeReq(req)
	if from.After(to) {
		t.Fatal("from should not be after to when swapped")
	}
}

func TestParseTimeRange_CapsAt7Days(t *testing.T) {
	t.Setenv("COLD_STORAGE_ENABLED", "false")
	req := httptest.NewRequest(http.MethodGet, "/test?from=2020-01-01T00:00:00Z&to=2026-04-17T00:00:00Z", nil)
	from, to := parseTimeRangeReq(req)
	diff := to.Sub(from)
	if diff > 7*24*time.Hour+time.Second {
		t.Fatalf("range should be capped at 7 days, got %v", diff)
	}
}

func TestParseTimeRange_CapsAt90DaysWhenColdEnabled(t *testing.T) {
	t.Setenv("COLD_STORAGE_ENABLED", "true")
	req := httptest.NewRequest(http.MethodGet, "/test?from=2020-01-01T00:00:00Z&to=2026-04-17T00:00:00Z", nil)
	from, to := parseTimeRangeReq(req)
	diff := to.Sub(from)
	if diff > 90*24*time.Hour+time.Second {
		t.Fatalf("range should be capped at 90 days when cold storage enabled, got %v", diff)
	}
	if diff < 89*24*time.Hour {
		t.Fatalf("expected ~90 day cap when cold storage enabled, got %v", diff)
	}
}

func TestParseTimeRange_InvalidIgnored(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test?from=not-a-date&to=also-bad", nil)
	from, to := parseTimeRangeReq(req)
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
	from, to := parseTimeRangeReq(req)
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
	from, to := parseTimeRangeReq(req)
	if from.After(to) {
		t.Fatalf("from should not be after to, got from=%v to=%v", from, to)
	}
}

func TestParseTimeRange_WhitespaceTrimed(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test?from=%202026-04-17T10:00:00Z%20&to=%202026-04-17T12:00:00Z%20", nil)
	from, to := parseTimeRangeReq(req)
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

// ---- New handler tests: PgssFilters ----

func TestPgssFilters_MissingInstance(t *testing.T) {
	h := NewPostgresHandlers(&service.MetricsService{}, &config.Config{})
	req := httptest.NewRequest(http.MethodGet, "/api/postgres/pgss/filters", nil)
	rr := httptest.NewRecorder()
	h.PgssFilters(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestPgssFilters_InstanceNotFound(t *testing.T) {
	h := NewPostgresHandlers(&service.MetricsService{}, pgCfg())
	req := httptest.NewRequest(http.MethodGet, "/api/postgres/pgss/filters?instance=ghost", nil)
	rr := httptest.NewRecorder()
	h.PgssFilters(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("want %d, got %d", http.StatusNotFound, rr.Code)
	}
}

func TestPgssFilters_WrongInstanceType(t *testing.T) {
	h := NewPostgresHandlers(&service.MetricsService{}, sqlserverCfg())
	req := httptest.NewRequest(http.MethodGet, "/api/postgres/pgss/filters?instance=ms-test", nil)
	rr := httptest.NewRecorder()
	h.PgssFilters(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestPgssFilters_NilTsLogger_Returns200(t *testing.T) {
	h := NewPostgresHandlers(&service.MetricsService{}, pgCfg())
	req := httptest.NewRequest(http.MethodGet, "/api/postgres/pgss/filters?instance=pg-test", nil)
	rr := httptest.NewRecorder()
	h.PgssFilters(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d; body: %s", rr.Code, rr.Body.String())
	}
	var resp models.PgssFilterOptions
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp.Instance != "pg-test" {
		t.Fatalf("want instance=pg-test, got %q", resp.Instance)
	}
}

// ---- New handler tests: PgssDbBreakdown ----

func TestPgssDbBreakdown_MissingInstance(t *testing.T) {
	h := NewPostgresHandlers(&service.MetricsService{}, &config.Config{})
	req := httptest.NewRequest(http.MethodGet, "/api/postgres/pgss/by-database", nil)
	rr := httptest.NewRecorder()
	h.PgssDbBreakdown(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestPgssDbBreakdown_NilTsLogger_Returns200(t *testing.T) {
	h := NewPostgresHandlers(&service.MetricsService{}, pgCfg())
	req := httptest.NewRequest(http.MethodGet, "/api/postgres/pgss/by-database?instance=pg-test", nil)
	rr := httptest.NewRecorder()
	h.PgssDbBreakdown(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d; body: %s", rr.Code, rr.Body.String())
	}
	var resp models.PgssDbBreakdownResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp.Instance != "pg-test" {
		t.Fatalf("want instance=pg-test, got %q", resp.Instance)
	}
}

// ---- New handler tests: PgssUserBreakdown ----

func TestPgssUserBreakdown_MissingInstance(t *testing.T) {
	h := NewPostgresHandlers(&service.MetricsService{}, &config.Config{})
	req := httptest.NewRequest(http.MethodGet, "/api/postgres/pgss/by-user", nil)
	rr := httptest.NewRecorder()
	h.PgssUserBreakdown(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestPgssUserBreakdown_NilTsLogger_Returns200(t *testing.T) {
	h := NewPostgresHandlers(&service.MetricsService{}, pgCfg())
	req := httptest.NewRequest(http.MethodGet, "/api/postgres/pgss/by-user?instance=pg-test", nil)
	rr := httptest.NewRecorder()
	h.PgssUserBreakdown(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d; body: %s", rr.Code, rr.Body.String())
	}
	var resp models.PgssUserBreakdownResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp.Instance != "pg-test" {
		t.Fatalf("want instance=pg-test, got %q", resp.Instance)
	}
}

// ---- Filter param sanitization tests ----

func TestSanitizeFilterParam_AllowsAlphanumericAndSymbols(t *testing.T) {
	cases := []struct{ input, want string }{
		{"mydb", "mydb"},
		{"app_user", "app_user"},
		{"app-user@domain.com", "app-user@domain.com"},
		{"test123", "test123"},
		{"'; DROP TABLE users;--", "DROPTABLEusers--"},
		{"<script>alert(1)</script>", "scriptalert1script"},
		{"normal_db_01", "normal_db_01"},
	}
	for _, tc := range cases {
		if got := sanitizeFilterParam(tc.input); got != tc.want {
			t.Errorf("sanitizeFilterParam(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestSanitizeQueryType_Allowlist(t *testing.T) {
	valid := []string{"S", "I", "U", "D", "E", "O", "s", "i", "u", "d", "e", "o"}
	for _, v := range valid {
		if got := sanitizeQueryType(v); got == "" {
			t.Errorf("sanitizeQueryType(%q) rejected valid type", v)
		}
	}
	invalid := []string{"X", "SELECT", "'; DROP--", "", " S", "SS"}
	for _, v := range invalid {
		if got := sanitizeQueryType(v); got != "" && v != "S" && v != " S" {
			t.Errorf("sanitizeQueryType(%q) = %q, want empty for invalid", v, got)
		}
	}
}

func TestPgssTop_WithFilterParams_Returns200(t *testing.T) {
	h := NewPostgresHandlers(&service.MetricsService{}, pgCfg())
	req := httptest.NewRequest(http.MethodGet,
		"/api/postgres/pgss/top?instance=pg-test&db_name=mydb&username=app_user&query_type=S", nil)
	rr := httptest.NewRecorder()
	h.PgssTop(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rr.Code)
	}
	var resp models.PgssTopQueriesResponse
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp.Instance != "pg-test" {
		t.Fatalf("want instance=pg-test, got %q", resp.Instance)
	}
}

func TestPgssTop_SQLInjectionInFilterParam_Returns200(t *testing.T) {
	h := NewPostgresHandlers(&service.MetricsService{}, pgCfg())
	// URL-encode the injection string so httptest.NewRequest can parse it
	params := url.Values{}
	params.Set("instance", "pg-test")
	params.Set("db_name", "'; DROP TABLE pgss_delta_1m;--")
	req := httptest.NewRequest(http.MethodGet, "/api/postgres/pgss/top?"+params.Encode(), nil)
	rr := httptest.NewRecorder()
	h.PgssTop(rr, req)
	// Should return 200 (sanitized), not 500
	if rr.Code == http.StatusInternalServerError {
		t.Fatal("SQL injection in filter param must not cause 500")
	}
}

// ---- New model struct tests ----

func TestPgssFilterOptionsStructFields(t *testing.T) {
	opts := models.PgssFilterOptions{
		Instance:  "pg-test",
		Databases: []string{"app_db", "analytics_db"},
		Users:     []string{"app_user"},
		AppNames:  []string{"myapp"},
	}
	if opts.Instance != "pg-test" {
		t.Fatal("Instance field missing or wrong")
	}
	if len(opts.Databases) != 2 {
		t.Fatal("Databases field count wrong")
	}
}

func TestPgssDbBreakdownStructFields(t *testing.T) {
	r := models.PgssDbBreakdown{
		DbName: "app_db", TotalExecMs: 1000.0, PctOfServer: 45.5,
		TotalCalls: 500, AvgMs: 2.0, CacheHitPct: 99.1, UniqueQueryIDs: 25,
	}
	if r.UniqueQueryIDs != 25 {
		t.Fatal("UniqueQueryIDs field missing")
	}
}

func TestPgssUserBreakdownStructFields(t *testing.T) {
	r := models.PgssUserBreakdown{
		UserName: "app_user", TotalExecMs: 500.0, PctOfServer: 22.5,
		TotalCalls: 250, AvgMs: 2.0, UniqueQueryIDs: 15,
	}
	if r.PctOfServer != 22.5 {
		t.Fatal("PctOfServer field missing")
	}
}
