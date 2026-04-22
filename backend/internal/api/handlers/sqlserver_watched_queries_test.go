package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rsharma155/sql_optima/internal/service"
)

func TestWatchedQueryList_MissingInstance(t *testing.T) {
	h := NewSqlServerWatchedQueryHandlers(&service.MetricsService{}, sqlserverCfg())
	req := httptest.NewRequest(http.MethodGet, "/api/sqlserver/watched-queries", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestWatchedQueryList_InstanceNotFound(t *testing.T) {
	h := NewSqlServerWatchedQueryHandlers(&service.MetricsService{}, sqlserverCfg())
	req := httptest.NewRequest(http.MethodGet, "/api/sqlserver/watched-queries?instance=nonexistent", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("want %d, got %d", http.StatusNotFound, rr.Code)
	}
}

func TestWatchedQueryList_OK_NilTsLogger(t *testing.T) {
	h := NewSqlServerWatchedQueryHandlers(&service.MetricsService{}, sqlserverCfg())
	req := httptest.NewRequest(http.MethodGet, "/api/sqlserver/watched-queries?instance=ms-test", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)
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

func TestWatchedQueryAdd_MissingBody(t *testing.T) {
	h := NewSqlServerWatchedQueryHandlers(&service.MetricsService{}, sqlserverCfg())
	req := httptest.NewRequest(http.MethodPost, "/api/sqlserver/watched-queries?instance=ms-test", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.Add(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want %d, got %d: %s", http.StatusBadRequest, rr.Code, rr.Body.String())
	}
}

func TestWatchedQueryAdd_MissingQueryHash(t *testing.T) {
	h := NewSqlServerWatchedQueryHandlers(&service.MetricsService{}, sqlserverCfg())
	body := `{"name":"test query"}`
	req := httptest.NewRequest(http.MethodPost, "/api/sqlserver/watched-queries?instance=ms-test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.Add(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want %d, got %d: %s", http.StatusBadRequest, rr.Code, rr.Body.String())
	}
}

func TestWatchedQueryDelete_InvalidID(t *testing.T) {
	h := NewSqlServerWatchedQueryHandlers(&service.MetricsService{}, sqlserverCfg())
	req := httptest.NewRequest(http.MethodDelete, "/api/sqlserver/watched-queries?id=abc", nil)
	rr := httptest.NewRecorder()
	h.Delete(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestWatchedQueryDetail_InvalidID(t *testing.T) {
	h := NewSqlServerWatchedQueryHandlers(&service.MetricsService{}, sqlserverCfg())
	req := httptest.NewRequest(http.MethodGet, "/api/sqlserver/watched-queries/detail?id=abc", nil)
	rr := httptest.NewRecorder()
	h.Detail(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestWatchedQueryAddEvent_MissingEventType(t *testing.T) {
	h := NewSqlServerWatchedQueryHandlers(&service.MetricsService{}, sqlserverCfg())
	body := `{"notes":"test"}`
	req := httptest.NewRequest(http.MethodPost, "/api/sqlserver/watched-queries/event?id=1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.AddEvent(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want %d, got %d: %s", http.StatusBadRequest, rr.Code, rr.Body.String())
	}
}
