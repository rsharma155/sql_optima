package models

import (
	"encoding/json"
	"testing"
	"time"
)

func TestPgssWorkloadResponseJSON(t *testing.T) {
	resp := PgssWorkloadResponse{
		Instance: "pg-prod",
		Points: []PgssWorkloadPoint{
			{Timestamp: time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC), QPS: 150.5, CacheHitRatio: 99.2},
		},
	}
	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got PgssWorkloadResponse
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Instance != "pg-prod" {
		t.Fatalf("instance: want pg-prod, got %s", got.Instance)
	}
	if len(got.Points) != 1 || got.Points[0].QPS != 150.5 {
		t.Fatalf("points mismatch: %+v", got.Points)
	}
}

func TestPgssTopQueriesResponseJSON(t *testing.T) {
	resp := PgssTopQueriesResponse{
		Instance: "pg-prod",
		SortBy:   "total_time",
		Queries: []PgssTopQuery{
			{QueryID: 42, Query: "SELECT 1", TotalTime: 1500.0, Calls: 200, AvgMs: 7.5},
		},
	}
	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got PgssTopQueriesResponse
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Queries) != 1 || got.Queries[0].QueryID != 42 {
		t.Fatalf("queries mismatch: %+v", got.Queries)
	}
}

func TestPgssLatencyResponseJSON(t *testing.T) {
	resp := PgssLatencyResponse{
		Instance: "pg-dev",
		Points: []PgssLatencyPoint{
			{Timestamp: time.Now(), P50: 1.2, P95: 15.5, P99: 120.0, MaxExec: 500.0},
		},
	}
	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got PgssLatencyResponse
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Points[0].P95 != 15.5 {
		t.Fatalf("P95: want 15.5, got %f", got.Points[0].P95)
	}
}

func TestPgssRegressionsResponseJSON(t *testing.T) {
	resp := PgssRegressionsResponse{
		Instance: "pg-staging",
		Regressions: []PgssRegression{
			{QueryID: 99, Query: "SELECT * FROM orders", PrevAvgMs: 5.0, CurrAvgMs: 50.0, ChangePct: 900.0, Status: "Degraded"},
		},
	}
	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got PgssRegressionsResponse
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Regressions[0].ChangePct != 900.0 {
		t.Fatalf("change_pct: want 900, got %f", got.Regressions[0].ChangePct)
	}
}

func TestPgssStatusResponseJSON(t *testing.T) {
	resp := PgssStatusResponse{
		Instance:            "pg-prod",
		ExtensionInstalled:  true,
		SharedPreloadActive: true,
		Ready:               true,
	}
	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got PgssStatusResponse
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !got.Ready {
		t.Fatal("expected Ready=true")
	}
	if got.Message != "" {
		t.Fatalf("expected empty message when ready, got %q", got.Message)
	}
}

func TestPgssStatusResponse_OmitsEmptyMessage(t *testing.T) {
	resp := PgssStatusResponse{Instance: "pg", Ready: true}
	b, _ := json.Marshal(resp)
	var m map[string]interface{}
	json.Unmarshal(b, &m)
	if _, exists := m["message"]; exists {
		t.Fatal("message should be omitted when empty (omitempty tag)")
	}
}
