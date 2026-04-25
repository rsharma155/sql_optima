// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Defines monitoring handlers for database-specific metrics including CPU, memory, waits, connections, locks, and query statistics.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package api

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/rsharma155/sql_optima/internal/api/handlers"
)

// monitoringHandlers groups HTTP handlers registered for dashboard and live APIs.
type monitoringHandlers struct {
	SqlServer              *handlers.SqlServerHandlers
	Postgres               *handlers.PostgresHandlers
	Live                   *handlers.LiveHandlers
	Timescale              *handlers.TimescaleHandlers
	Health                 *handlers.HealthHandlers
	Dashboard              *handlers.DashboardHandlers
	Query                  *handlers.QueryHandlers
	SIH                    *handlers.StorageIndexHealthTimescaleHandlers
	SqlServerQueryAnalysis *handlers.SqlServerQueryAnalysisHandlers
	SqlServerWatchedQuery  *handlers.SqlServerWatchedQueryHandlers
}

// registerMonitoringReadRoutes attaches read-only monitoring endpoints (viewer, dba, or admin).
func registerMonitoringReadRoutes(sr *mux.Router, h *monitoringHandlers, rulesBestPractices http.HandlerFunc) {
	m := h.SqlServer
	p := h.Postgres
	l := h.Live
	ts := h.Timescale
	he := h.Health
	q := h.Query
	sih := h.SIH

	sr.HandleFunc("/sqlserver/dashboard", m.Dashboard).Methods("GET")
	sr.HandleFunc("/sqlserver/dashboard/v2", m.DashboardV2).Methods("GET")
	sr.HandleFunc("/sqlserver/enterprise-dashboard/v2", m.EnterpriseDashboardV2).Methods("GET")
	sr.HandleFunc("/sqlserver/dashboard/timeseries", m.DashboardTimeSeries).Methods("GET")
	sr.HandleFunc("/sqlserver/storage-index/table-drilldown", m.TableDrilldown).Methods("GET")
	sr.HandleFunc("/sqlserver/performance-debt", m.PerformanceDebt).Methods("GET")
	sr.HandleFunc("/postgres/dashboard", p.Dashboard).Methods("GET")
	sr.HandleFunc("/postgres/db-observation", p.DBObservation).Methods("GET")
	sr.HandleFunc("/postgres/overview", p.Overview).Methods("GET")
	sr.HandleFunc("/postgres/server-info", p.ServerInfo).Methods("GET")
	sr.HandleFunc("/postgres/system-stats", p.SystemStats).Methods("GET")
	sr.HandleFunc("/postgres/system-stats/history", p.SystemStatsHistory).Methods("GET")
	sr.HandleFunc("/cpu/history", p.CPUHistory).Methods("GET")
	sr.HandleFunc("/cpu/saturation", p.CPUSaturation).Methods("GET")
	sr.HandleFunc("/cpu/database", p.CPUDatabase).Methods("GET")
	sr.HandleFunc("/cpu/top-queries", p.CPUTopQueries).Methods("GET")
	sr.HandleFunc("/postgres/bgwriter", p.BGWriter).Methods("GET")
	sr.HandleFunc("/postgres/archiver", p.Archiver).Methods("GET")
	sr.HandleFunc("/postgres/waits/history", p.WaitEventsHistory).Methods("GET")
	sr.HandleFunc("/postgres/io/history", p.DbIOHistory).Methods("GET")
	sr.HandleFunc("/postgres/settings/drift", p.SettingsDrift).Methods("GET")
	sr.HandleFunc("/postgres/databases", p.Databases).Methods("GET")
	sr.HandleFunc("/postgres/config", p.Config).Methods("GET")
	sr.HandleFunc("/postgres/best-practices", p.BestPractices).Methods("GET")
	sr.HandleFunc("/postgres/storage", p.Storage).Methods("GET")
	sr.HandleFunc("/postgres/database-size", p.DatabaseSize).Methods("GET")
	sr.HandleFunc("/postgres/vacuum/progress", p.VacuumProgress).Methods("GET")
	sr.HandleFunc("/postgres/vacuum/progress/history", p.VacuumProgressHistory).Methods("GET")
	sr.HandleFunc("/postgres/sessions/state/history", p.SessionStateHistory).Methods("GET")
	sr.HandleFunc("/postgres/table-maintenance/history", p.TableMaintenanceHistory).Methods("GET")
	sr.HandleFunc("/postgres/table-maintenance/latest", p.TableMaintenanceLatest).Methods("GET")
	sr.HandleFunc("/postgres/pooler/latest", p.PoolerLatest).Methods("GET")
	sr.HandleFunc("/postgres/pooler/history", p.PoolerHistory).Methods("GET")
	sr.HandleFunc("/postgres/deadlocks/history", p.DeadlocksHistory).Methods("GET")
	sr.HandleFunc("/postgres/locks/wait-history", p.LockWaitHistory).Methods("GET")
	sr.HandleFunc("/postgres/locks-blocking/kpis", p.LocksBlockingKPIs).Methods("GET")
	sr.HandleFunc("/postgres/locks-blocking/timeline", p.LocksBlockingTimeline).Methods("GET")
	sr.HandleFunc("/postgres/locks-blocking/top-locked-tables", p.LocksBlockingTopLockedTables).Methods("GET")
	sr.HandleFunc("/postgres/locks-blocking/details", p.LocksBlockingDetails).Methods("GET")
	sr.HandleFunc("/postgres/replication", p.Replication).Methods("GET")
	sr.HandleFunc("/postgres/sessions", p.Sessions).Methods("GET")
	sr.HandleFunc("/postgres/locks", p.Locks).Methods("GET")
	sr.HandleFunc("/postgres/blocking-tree", p.BlockingTree).Methods("GET")
	sr.HandleFunc("/postgres/queries", p.Queries).Methods("GET")
	sr.HandleFunc("/postgres/alerts", p.Alerts).Methods("GET")
	sr.HandleFunc("/postgres/control-center", p.ControlCenter).Methods("GET")
	sr.HandleFunc("/postgres/control-center/history", p.ControlCenterHistory).Methods("GET")
	sr.HandleFunc("/postgres/replication-lag/history", p.ReplicationLagHistory).Methods("GET")
	sr.HandleFunc("/postgres/replication-slots", p.ReplicationSlots).Methods("GET")
	sr.HandleFunc("/postgres/disk", p.Disk).Methods("GET")
	sr.HandleFunc("/postgres/backups/latest", p.BackupLatest).Methods("GET")
	sr.HandleFunc("/postgres/backups/history", p.BackupHistory).Methods("GET")
	sr.HandleFunc("/postgres/logs/summary", p.LogsSummary).Methods("GET")
	sr.HandleFunc("/postgres/logs/recent", p.LogsRecent).Methods("GET")
	sr.HandleFunc("/postgres/memory/intelligence", p.PgMemoryIntelligence).Methods("GET")
	sr.HandleFunc("/postgres/bloat", p.BloatEstimates).Methods("GET")
	sr.HandleFunc("/postgres/idle-in-transaction", p.IdleInTransaction).Methods("GET")
	sr.HandleFunc("/postgres/xid-wraparound", p.XIDWraparoundRisk).Methods("GET")
	sr.HandleFunc("/postgres/wal/archiver-risk", p.WALArchiverRisk).Methods("GET")
	sr.HandleFunc("/postgres/long-running-transactions", p.LongRunningTransactions).Methods("GET")
	sr.HandleFunc("/postgres/index-bloat", p.IndexBloat).Methods("GET")
	sr.HandleFunc("/postgres/pgss/status", p.PgssStatus).Methods("GET")
	sr.HandleFunc("/postgres/pgss/workload", p.PgssWorkload).Methods("GET")
	sr.HandleFunc("/postgres/pgss/latency", p.PgssLatency).Methods("GET")
	sr.HandleFunc("/postgres/pgss/top", p.PgssTop).Methods("GET")
	sr.HandleFunc("/postgres/pgss/regressions", p.PgssRegressions).Methods("GET")
	sr.HandleFunc("/postgres/pgss/summary", p.PgssSummary).Methods("GET")
	sr.HandleFunc("/sqlserver/cpu-drilldown", m.CPUDrilldown).Methods("GET")
	sr.HandleFunc("/sqlserver/cpu-scheduler-stats", m.CPUSchedulerStats).Methods("GET")
	sr.HandleFunc("/sqlserver/server-properties", m.ServerProperties).Methods("GET")
	sr.HandleFunc("/sqlserver/ag-health", m.AGHealth).Methods("GET")
	sr.HandleFunc("/sqlserver/ag-health/history", m.AGHealthTimeSeries).Methods("GET")
	sr.HandleFunc("/sqlserver/replication-status", m.ReplicationStatus).Methods("GET")
	sr.HandleFunc("/sqlserver/log-shipping", m.LogShipping).Methods("GET")
	sr.HandleFunc("/sqlserver/db-throughput", m.DBThroughput).Methods("GET")
	sr.HandleFunc("/sqlserver/best-practices", m.BestPractices).Methods("GET")
	sr.HandleFunc("/sqlserver/guardrails", m.Guardrails).Methods("GET")
	sr.HandleFunc("/sqlserver/jobs", m.Jobs).Methods("GET")
	sr.HandleFunc("/sqlserver/overview", m.Overview).Methods("GET")
	sr.HandleFunc("/sqlserver/latch-stats", m.LatchStats).Methods("GET")
	sr.HandleFunc("/sqlserver/waiting-tasks", m.WaitingTasks).Methods("GET")
	sr.HandleFunc("/sqlserver/memory-grants", m.MemoryGrants).Methods("GET")
	sr.HandleFunc("/sqlserver/scheduler-wg", m.SchedulerWorkers).Methods("GET")
	sr.HandleFunc("/sqlserver/procedure-stats", m.ProcedureStats).Methods("GET")
	sr.HandleFunc("/sqlserver/file-io-latency", m.FileIOLatency).Methods("GET")
	sr.HandleFunc("/sqlserver/spinlock-stats", m.SpinlockStats).Methods("GET")
	sr.HandleFunc("/sqlserver/memory-clerks", m.MemoryClerks).Methods("GET")
	sr.HandleFunc("/sqlserver/tempdb-stats", m.TempdbStats).Methods("GET")
	sr.HandleFunc("/sqlserver/plan-cache-health", m.PlanCacheHealth).Methods("GET")
	sr.HandleFunc("/sqlserver/memory-grant-waiters", m.MemoryGrantWaiters).Methods("GET")
	sr.HandleFunc("/sqlserver/tempdb-top-consumers", m.TempdbTopConsumers).Methods("GET")
	sr.HandleFunc("/sqlserver/wait-categories", m.WaitCategories).Methods("GET")
	sr.HandleFunc("/timescale/status", ts.Status).Methods("GET")
	sr.HandleFunc("/timescale/sqlserver/metrics", ts.SqlServerMetrics).Methods("GET")
	sr.HandleFunc("/timescale/sqlserver/cpu-history", ts.SqlServerCPUHistory).Methods("GET")
	sr.HandleFunc("/timescale/sqlserver/memory-drilldown", ts.SqlServerMemoryDrilldown).Methods("GET")
	sr.HandleFunc("/timescale/sqlserver/top-queries", ts.SqlServerTopQueries).Methods("GET")
	sr.HandleFunc("/timescale/sqlserver/query-stats-dashboard", ts.SqlServerQueryStatsDashboard).Methods("GET")
	sr.HandleFunc("/timescale/sqlserver/query-stats-timeseries", ts.SqlServerQueryStatsTimeSeries).Methods("GET")
	sr.HandleFunc("/timescale/sqlserver/long-running-queries", ts.SqlServerLongRunningQueries).Methods("GET")
	sr.HandleFunc("/timescale/postgres/throughput", ts.PostgresThroughput).Methods("GET")
	sr.HandleFunc("/timescale/postgres/connections", ts.PostgresConnections).Methods("GET")
	if sih != nil {
		sr.HandleFunc("/timescale/storage-index-health/index-usage", sih.IndexUsage).Methods("GET")
		sr.HandleFunc("/timescale/storage-index-health/table-usage", sih.TableUsage).Methods("GET")
		sr.HandleFunc("/timescale/storage-index-health/growth", sih.Growth).Methods("GET")
		sr.HandleFunc("/timescale/storage-index-health/dashboard", sih.Dashboard).Methods("GET")
		sr.HandleFunc("/timescale/storage-index-health/filters", sih.Filters).Methods("GET")
		sr.HandleFunc("/timescale/storage-index-health/index-definition", sih.IndexDefinition).Methods("GET")
	}
	sr.HandleFunc("/live/kpis", l.KPIs).Methods("GET")
	sr.HandleFunc("/live/running-queries", l.RunningQueries).Methods("GET")
	sr.HandleFunc("/live/blocking", l.Blocking).Methods("GET")
	sr.HandleFunc("/live/io-latency", l.IOLatency).Methods("GET")
	sr.HandleFunc("/live/tempdb", l.TempDB).Methods("GET")
	sr.HandleFunc("/live/waits", l.Waits).Methods("GET")
	sr.HandleFunc("/live/connections", l.Connections).Methods("GET")
	sr.HandleFunc("/health/score", he.Score).Methods("GET")
	sr.HandleFunc("/health/anomalies", he.Anomalies).Methods("GET")
	sr.HandleFunc("/health/regressed-queries", he.RegressedQueries).Methods("GET")
	sr.HandleFunc("/health/wait-spikes", he.WaitSpikes).Methods("GET")
	sr.HandleFunc("/health/metrics-history", he.MetricsHistory).Methods("GET")
	sr.HandleFunc("/incidents/timeline", he.IncidentsTimeline).Methods("GET")
	sr.HandleFunc("/queries/bottlenecks", q.Bottlenecks).Methods("GET")
	sr.HandleFunc("/queries/query-store/sql-text", q.QueryStoreSQLText).Methods("GET")
	sr.HandleFunc("/rules/best-practices", rulesBestPractices).Methods("GET")

	// SQL Server Query Analysis Dashboard
	if qa := h.SqlServerQueryAnalysis; qa != nil {
		sr.HandleFunc("/sqlserver/query-analysis/summary", qa.Summary).Methods("GET")
		sr.HandleFunc("/sqlserver/query-analysis/regressions", qa.Regressions).Methods("GET")
		sr.HandleFunc("/sqlserver/query-analysis/plan-instability", qa.PlanInstability).Methods("GET")
		sr.HandleFunc("/sqlserver/query-analysis/top-queries", qa.TopQueries).Methods("GET")
		sr.HandleFunc("/sqlserver/query-analysis/query-plans", qa.QueryPlans).Methods("GET")
		sr.HandleFunc("/sqlserver/query-analysis/query-wait-stats", qa.QueryWaitStats).Methods("GET")
	}

	// SQL Server Watched Query Analyzer
	if wq := h.SqlServerWatchedQuery; wq != nil {
		sr.HandleFunc("/sqlserver/watched-queries", wq.List).Methods("GET")
		sr.HandleFunc("/sqlserver/watched-queries", wq.Add).Methods("POST")
		sr.HandleFunc("/sqlserver/watched-queries", wq.Delete).Methods("DELETE")
		sr.HandleFunc("/sqlserver/watched-queries/detail", wq.Detail).Methods("GET")
		sr.HandleFunc("/sqlserver/watched-queries/event", wq.AddEvent).Methods("POST")
	}
}

// registerMonitoringElevatedRoutes attaches diagnostics that should be limited to dba or admin.
func registerMonitoringElevatedRoutes(sr *mux.Router, m *handlers.SqlServerHandlers, explainAnalyze, explainOptimize, explainIndexAdvisor http.HandlerFunc) {
	sr.HandleFunc("/postgres/explain/analyze", explainAnalyze).Methods("POST")
	sr.HandleFunc("/postgres/explain/optimize", explainOptimize).Methods("POST")
	sr.HandleFunc("/postgres/explain/index-advisor", explainIndexAdvisor).Methods("POST")
	sr.HandleFunc("/sqlserver/xevents", m.XEvents).Methods("GET")
}

// registerPostgresDBAMutations attaches mutating Postgres endpoints (dba or admin).
func registerPostgresDBAMutations(sr *mux.Router, p *handlers.PostgresHandlers) {
	sr.HandleFunc("/postgres/kill-session", p.KillSession).Methods("POST")
	sr.HandleFunc("/postgres/reset-queries", p.ResetQueries).Methods("POST")
	sr.HandleFunc("/postgres/backups/report", p.BackupReport).Methods("POST")
	sr.HandleFunc("/postgres/logs/report", p.LogsReport).Methods("POST")
}

// registerDashboardWidgetRoutes attaches widget list and (future) query execute for authenticated users.
func registerDashboardWidgetRoutes(sr *mux.Router, d *handlers.DashboardHandlers) {
	sr.HandleFunc("/dashboard/widgets", d.Widgets).Methods("GET")
	sr.HandleFunc("/dashboard/query/execute", d.ExecuteQuery).Methods("POST")
}
