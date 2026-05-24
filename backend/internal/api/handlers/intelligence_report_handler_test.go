package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rsharma155/sql_optima/internal/config"
)

func TestIntelligenceReportHandlers_NilService(t *testing.T) {
	cfg := &config.Config{}
	h := NewIntelligenceReportHandlers(nil, cfg)

	// Test Analyze
	req := httptest.NewRequest("POST", "/api/sqlserver/intelligence-report/analyze?server_id=123", nil)
	rr := httptest.NewRecorder()
	h.Analyze(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("Analyze returned wrong status code: got %v want %v", rr.Code, http.StatusServiceUnavailable)
	}

	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp["error"] == "" {
		t.Error("Analyze returned empty error message")
	}

	// Test Status
	req = httptest.NewRequest("GET", "/api/sqlserver/intelligence-report/status", nil)
	rr = httptest.NewRecorder()
	h.Status(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Status returned wrong status code: got %v want %v", rr.Code, http.StatusOK)
	}

	var statusResp map[string]bool
	if err := json.NewDecoder(rr.Body).Decode(&statusResp); err != nil {
		t.Fatal(err)
	}
	if statusResp["active"] != false {
		t.Error("Status returned active=true when service is nil")
	}
}
