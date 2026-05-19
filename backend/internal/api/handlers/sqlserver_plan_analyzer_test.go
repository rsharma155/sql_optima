package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/rsharma155/sql_optima/internal/service"
)

func TestSqlServerPlanAnalyzer_Analyze(t *testing.T) {
	// 1. Setup handler
	metricsSvc := &service.MetricsService{}
	h := NewSqlServerPlanHandlers(metricsSvc, nil)

	// 2. Load sample plan from examples directory
	// Note: Adjust path if needed based on test execution context
	planPath := filepath.Join("..", "..", "..", "..", "examples", "execplan_test.sqlplan")
	planData, err := os.ReadFile(planPath)
	if err != nil {
		t.Skipf("Skipping test: sample plan not found at %s", planPath)
	}

	// 3. Create request
	req := httptest.NewRequest(http.MethodPost, "/api/sqlserver/plan/analyze", bytes.NewReader(planData))
	req.Header.Set("Content-Type", "application/xml")
	rr := httptest.NewRecorder()

	// 4. Execute
	h.AnalyzePlan(rr, req)

	// 5. Assertions
	if rr.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if success, ok := resp["success"].(bool); !ok || !success {
		t.Errorf("Expected success: true, got %v", resp["success"])
	}

	analysis, ok := resp["analysis"].(map[string]interface{})
	if !ok {
		t.Fatal("Response missing 'analysis' object")
	}

	// Verify key fields (snake_case)
	if _, ok := analysis["health_score"]; !ok {
		t.Error("Analysis missing 'health_score'")
	}
	if _, ok := analysis["findings"]; !ok {
		t.Error("Analysis missing 'findings'")
	}
	if _, ok := analysis["operators"]; !ok {
		t.Error("Analysis missing 'operators'")
	}

	htmlReport, ok := resp["html_report"].(string)
	if !ok || len(htmlReport) < 1000 {
		t.Errorf("html_report missing or too short (length: %d)", len(htmlReport))
	}

	t.Logf("Plan analysis successful. Found %d findings. HTML length: %d", len(analysis["findings"].([]interface{})), len(htmlReport))
}
