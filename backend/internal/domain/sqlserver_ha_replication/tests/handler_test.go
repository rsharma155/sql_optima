// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose : Unit tests for HAReplicationHandler HTTP endpoints.
//           Tests use httptest and a nil TimescaleDB pool so no database
//           connection is required.  They verify request parsing and error
//           response codes — not business logic (covered in domain_test.go).
//
// Author  : Ravi Sharma <ravisharma155@gmail.com>
// Created : 2026-05-14
// License : MIT
package tests

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rsharma155/sql_optima/internal/config"
	ha_api "github.com/rsharma155/sql_optima/internal/domain/sqlserver_ha_replication/api"
	"github.com/rsharma155/sql_optima/internal/service"
)

// newTestHandler builds a handler with a minimal config and no TimescaleDB pool.
// All endpoints that require the repo will return 503.
func newTestHandler() *ha_api.HAReplicationHandler {
	cfg := &config.Config{
		Instances: []config.Instance{
			{Name: "sql-test", Type: "sqlserver"},
		},
	}
	svc := &service.MetricsService{Config: cfg}
	return ha_api.NewHAReplicationHandler(svc)
}

// ---------------------------------------------------------------------------
// FeatureStatus
// ---------------------------------------------------------------------------

func TestFeatureStatus_MissingServerID_Returns400(t *testing.T) {
	h := newTestHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/sqlserver/ha/feature-status", nil)
	rr := httptest.NewRecorder()
	h.FeatureStatus(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}

func TestFeatureStatus_NoTimescale_ReturnsDisabled(t *testing.T) {
	h := newTestHandler()
	req := httptest.NewRequest(http.MethodGet,
		"/api/sqlserver/ha/feature-status?server_id=00000000-0000-0000-0000-000000000001", nil)
	rr := httptest.NewRecorder()
	h.FeatureStatus(rr, req)
	// With no repo, handler gracefully returns 200 with both features disabled.
	if rr.Code != http.StatusOK {
		t.Errorf("want 200 (graceful degradation) when repo is nil, got %d", rr.Code)
	}
}

func TestFeatureStatus_DatabaseParam_NoTimescale_ReturnsDisabled(t *testing.T) {
	h := newTestHandler()
	// database= param must be accepted without error; with no repo it returns 200 + disabled.
	req := httptest.NewRequest(http.MethodGet,
		"/api/sqlserver/ha/feature-status?server_id=00000000-0000-0000-0000-000000000001&database=AdventureWorks", nil)
	rr := httptest.NewRecorder()
	h.FeatureStatus(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200 with database param and nil repo, got %d", rr.Code)
	}
}

func TestFeatureStatus_DatabaseAll_NoTimescale_ReturnsDisabled(t *testing.T) {
	h := newTestHandler()
	// database=all is the sentinel for instance-level detection; must behave the same as no database param.
	req := httptest.NewRequest(http.MethodGet,
		"/api/sqlserver/ha/feature-status?server_id=00000000-0000-0000-0000-000000000001&database=all", nil)
	rr := httptest.NewRecorder()
	h.FeatureStatus(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200 with database=all and nil repo, got %d", rr.Code)
	}
}

// ---------------------------------------------------------------------------
// DashboardSummary
// ---------------------------------------------------------------------------

func TestDashboardSummary_MissingServerID_Returns400(t *testing.T) {
	h := newTestHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/sqlserver/ha/dashboard-summary", nil)
	rr := httptest.NewRecorder()
	h.DashboardSummary(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}

func TestDashboardSummary_NoTimescale_Returns503(t *testing.T) {
	h := newTestHandler()
	req := httptest.NewRequest(http.MethodGet,
		"/api/sqlserver/ha/dashboard-summary?server_id=00000000-0000-0000-0000-000000000001", nil)
	rr := httptest.NewRecorder()
	h.DashboardSummary(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("want 503 when TimescaleDB not connected, got %d", rr.Code)
	}
}

// ---------------------------------------------------------------------------
// RPOTrend
// ---------------------------------------------------------------------------

func TestRPOTrend_MissingServerID_Returns400(t *testing.T) {
	h := newTestHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/sqlserver/ha/rpo/trend", nil)
	rr := httptest.NewRecorder()
	h.RPOTrend(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}

// ---------------------------------------------------------------------------
// RTOTrend
// ---------------------------------------------------------------------------

func TestRTOTrend_MissingServerID_Returns400(t *testing.T) {
	h := newTestHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/sqlserver/ha/rto/trend", nil)
	rr := httptest.NewRecorder()
	h.RTOTrend(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}

// ---------------------------------------------------------------------------
// ReplicaHealth
// ---------------------------------------------------------------------------

func TestReplicaHealth_MissingServerID_Returns400(t *testing.T) {
	h := newTestHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/sqlserver/ha/replicas", nil)
	rr := httptest.NewRecorder()
	h.ReplicaHealth(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}

// ---------------------------------------------------------------------------
// FailoverReadiness
// ---------------------------------------------------------------------------

func TestFailoverReadiness_MissingServerID_Returns400(t *testing.T) {
	h := newTestHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/sqlserver/ha/failover-readiness", nil)
	rr := httptest.NewRecorder()
	h.FailoverReadiness(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}

// ---------------------------------------------------------------------------
// DatabaseCoverage
// ---------------------------------------------------------------------------

func TestDatabaseCoverage_MissingServerID_Returns400(t *testing.T) {
	h := newTestHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/sqlserver/ha/database-coverage", nil)
	rr := httptest.NewRecorder()
	h.DatabaseCoverage(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}

// ---------------------------------------------------------------------------
// ReplicationTopology
// ---------------------------------------------------------------------------

func TestReplicationTopology_MissingServerID_Returns400(t *testing.T) {
	h := newTestHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/sqlserver/ha/replication/topology", nil)
	rr := httptest.NewRecorder()
	h.ReplicationTopology(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}
