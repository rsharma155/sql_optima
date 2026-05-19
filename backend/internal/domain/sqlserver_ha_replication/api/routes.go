// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose : Route registrations for the SQL Server HA & Replication domain.
//           All endpoints are mounted under /api/sqlserver/ha/ to namespace
//           them from the legacy /api/sqlserver/ag-health endpoints which are
//           preserved for backwards compatibility.
//
// Author  : Ravi Sharma <ravisharma155@gmail.com>
// Created : 2026-05-14
// License : MIT
package api

import "github.com/gorilla/mux"

// RegisterHARoutes attaches all HA & Replication read endpoints to the given
// sub-router.  The sub-router is expected to already have auth middleware applied.
func RegisterHARoutes(sr *mux.Router, h *HAReplicationHandler) {
	// Page gate — called first on page load
	sr.HandleFunc("/sqlserver/ha/feature-status", h.FeatureStatus).Methods("GET")

	// Single aggregated payload for initial page render (1 round-trip)
	sr.HandleFunc("/sqlserver/ha/dashboard-summary", h.DashboardSummary).Methods("GET")

	// HA KPI & health endpoints (30 s refresh)
	sr.HandleFunc("/sqlserver/ha/replicas", h.ReplicaHealth).Methods("GET")
	sr.HandleFunc("/sqlserver/ha/failover-readiness", h.FailoverReadiness).Methods("GET")

	// Trend charts (1 min refresh)
	sr.HandleFunc("/sqlserver/ha/rpo/trend", h.RPOTrend).Methods("GET")
	sr.HandleFunc("/sqlserver/ha/rto/trend", h.RTOTrend).Methods("GET")

	// Failover history (5 min refresh)
	sr.HandleFunc("/sqlserver/ha/failover-history", h.FailoverHistory).Methods("GET")
	sr.HandleFunc("/sqlserver/ha/alerts-timeline", h.AlertsTimeline).Methods("GET")

	// Replication endpoints
	sr.HandleFunc("/sqlserver/ha/replication/kpis", h.ReplicationKPIs).Methods("GET")
	sr.HandleFunc("/sqlserver/ha/replication/topology", h.ReplicationTopology).Methods("GET")
	sr.HandleFunc("/sqlserver/ha/replication/backlog/trend", h.ReplicationBacklogTrend).Methods("GET")
	sr.HandleFunc("/sqlserver/ha/replication/articles", h.ReplicationArticles).Methods("GET")

	// Coverage panel (2 min refresh)
	sr.HandleFunc("/sqlserver/ha/database-coverage", h.DatabaseCoverage).Methods("GET")
}
