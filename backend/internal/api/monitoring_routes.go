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
	pg_backup_api "github.com/rsharma155/sql_optima/internal/domain/postgres_backup_dr/api"
	pg_obs_api "github.com/rsharma155/sql_optima/internal/domain/postgres_observability/api"
	pg_security_api "github.com/rsharma155/sql_optima/internal/domain/postgres_security/api"
	ha_api "github.com/rsharma155/sql_optima/internal/domain/sqlserver_ha_replication/api"
	sqlbackup_api "github.com/rsharma155/sql_optima/internal/domain/sqlserver_backup_recovery/api"
)

// monitoringHandlers groups HTTP handlers registered for dashboard and live APIs.
type monitoringHandlers struct {
	SqlServer              *handlers.SqlServerHandlers
	Postgres               *handlers.PostgresHandlers
Timescale              *handlers.TimescaleHandlers
	Health                 *handlers.HealthHandlers
	Dashboard              *handlers.DashboardHandlers
	Query                  *handlers.QueryHandlers
	PgSIH                  *handlers.PgStorageIndexHealthHandlers
	SqlServerSIH           *handlers.SqlServerStorageHandlers
	UnifiedSIH             *handlers.StorageIndexHealthTimescaleHandlers
	SqlServerQueryAnalysis *handlers.SqlServerQueryAnalysisHandlers
	SqlServerWatchedQuery  *handlers.WatchedQueryHandlers
	SqlServerWorkload      *handlers.SqlServerWorkloadHandlers
	SqlServerPlanAnalyzer  *handlers.SqlServerPlanHandlers
	SqlServerLocks         *handlers.SqlServerLockHandlers
	SqlServerWaitStats     *handlers.SqlServerWaitStatsHandlers
	IntelligenceReport     *handlers.IntelligenceReportHandlers
	ColdStorage            *handlers.ColdStorageHandlers

	// New Postgres Domain Handlers
	PgObservability *pg_obs_api.PostgresObservabilityHandler
	PgBackup        *pg_backup_api.PostgresBackupHandler
	PgSecurity      *pg_security_api.PostgresSecurityHandler

	// SQL Server HA & Replication Domain Handler
	SQLServerHA *ha_api.HAReplicationHandler

	// SQL Server Backup & Recovery
	SQLServerBackup *sqlbackup_api.SQLServerBackupHandler
}

// registerMonitoringReadRoutes attaches read-only monitoring endpoints (viewer, dba, or admin).
func registerMonitoringReadRoutes(sr *mux.Router, h *monitoringHandlers, rulesBestPractices http.HandlerFunc) {
	m := h.SqlServer
	p := h.Postgres
	ts := h.Timescale
	he := h.Health
	q := h.Query
	pgSih := h.PgSIH
	msSih := h.SqlServerSIH
	uSih := h.UnifiedSIH
	wld := h.SqlServerWorkload
	lk := h.SqlServerLocks
	wsV2 := h.SqlServerWaitStats
	ir := h.IntelligenceReport

	sr.HandleFunc("/sqlserver/dashboard", m.Dashboard).Methods("GET")
	sr.HandleFunc("/sqlserver/health-v2", m.HealthV2).Methods("GET")
	sr.HandleFunc("/sqlserver/dashboard/v2", m.DashboardV2).Methods("GET")
	sr.HandleFunc("/sqlserver/enterprise-dashboard/v2", m.EnterpriseDashboardV2).Methods("GET")
	sr.HandleFunc("/sqlserver/wait-stats/dashboard", m.WaitStatsDashboardV2).Methods("GET")
	sr.HandleFunc("/sqlserver/wait-stats/kpis", wsV2.GetKPIs).Methods("GET")
	sr.HandleFunc("/sqlserver/wait-stats/trends", wsV2.GetTrends).Methods("GET")
	sr.HandleFunc("/sqlserver/wait-stats/top-waits", wsV2.GetTopWaitTypes).Methods("GET")
	sr.HandleFunc("/sqlserver/wait-stats/cpu-pressure", wsV2.GetCPUPressure).Methods("GET")
	sr.HandleFunc("/sqlserver/wait-stats/active-sessions", wsV2.GetActiveSessions).Methods("GET")
	sr.HandleFunc("/sqlserver/wait-stats/blocking-tree", wsV2.GetBlockingTree).Methods("GET")
	sr.HandleFunc("/sqlserver/wait-stats/db-impact", wsV2.GetDatabaseImpact).Methods("GET")
	sr.HandleFunc("/sqlserver/wait-stats/type-help", wsV2.GetWaitTypeHelp).Methods("GET")
	sr.HandleFunc("/sqlserver/dashboard/timeseries", m.DashboardTimeSeries).Methods("GET")
	sr.HandleFunc("/sqlserver/storage-index/table-drilldown", m.TableDrilldown).Methods("GET")
	sr.HandleFunc("/sqlserver/performance-debt", m.PerformanceDebt).Methods("GET")
	sr.HandleFunc("/sqlserver/performance-debt/refresh", m.TriggerPerformanceDebtScan).Methods("POST")
	sr.HandleFunc("/sqlserver/server-vitals", m.GetServerVitals).Methods("GET")
	sr.HandleFunc("/sqlserver/volume-stats/live", m.GetLiveVolumeStats).Methods("GET")
	sr.HandleFunc("/sqlserver/workload/default-database", wld.GetDefaultDatabase).Methods("GET")
	sr.HandleFunc("/sqlserver/workload/databases", wld.GetDatabases).Methods("GET")
	sr.HandleFunc("/sqlserver/workload/summary", wld.GetSummary).Methods("GET")
	sr.HandleFunc("/sqlserver/workload/trends", wld.GetTrends).Methods("GET")
	sr.HandleFunc("/sqlserver/workload/top-queries", wld.GetTopOffenders).Methods("GET")
	sr.HandleFunc("/sqlserver/workload/app-load", wld.GetAppLoadTimeline).Methods("GET")
	sr.HandleFunc("/sqlserver/workload/login-load", wld.GetLoginLoadTimeline).Methods("GET")
	sr.HandleFunc("/sqlserver/workload/top-apps", wld.GetTopApps).Methods("GET")
	sr.HandleFunc("/sqlserver/workload/top-logins", wld.GetTopLogins).Methods("GET")

	sr.HandleFunc("/sqlserver/blocking/kpis", lk.GetBlockingKpis).Methods("GET")
	sr.HandleFunc("/sqlserver/blocking/timeline", lk.GetBlockingTimeline).Methods("GET")
	sr.HandleFunc("/sqlserver/blocking/details", lk.GetBlockingDetails).Methods("GET")
	sr.HandleFunc("/sqlserver/blocking/locks", lk.GetBlockingLocks).Methods("GET")
	sr.HandleFunc("/sqlserver/blocking/most-blocked-databases", lk.GetMostBlockedDatabases).Methods("GET")
	sr.HandleFunc("/sqlserver/blocking/most-blocked-objects", lk.GetMostBlockedObjects).Methods("GET")
	sr.HandleFunc("/sqlserver/blocking/top-queries", lk.GetTopBlockingQueries).Methods("GET")
	sr.HandleFunc("/sqlserver/blocking/recurrence", lk.GetBlockingRecurrence).Methods("GET")
	sr.HandleFunc("/postgres/dashboard", p.Dashboard).Methods("GET")
	sr.HandleFunc("/pg/dashboard", p.DashboardV2).Methods("GET")
	sr.HandleFunc("/postgres/db-observation", p.DBObservation).Methods("GET")
	sr.HandleFunc("/postgres/overview", p.Overview).Methods("GET")
	sr.HandleFunc("/postgres/server-info", p.ServerInfo).Methods("GET")
	sr.HandleFunc("/postgres/system-stats", p.SystemStats).Methods("GET")
	sr.HandleFunc("/postgres/system-stats/history", p.SystemStatsHistory).Methods("GET")
	sr.HandleFunc("/cpu/history", p.CPUHistory).Methods("GET")
	sr.HandleFunc("/cpu/saturation", p.CPUSaturation).Methods("GET")
	sr.HandleFunc("/cpu/database", p.CPUDatabase).Methods("GET")
	sr.HandleFunc("/cpu/top-queries", p.CPUTopQueries).Methods("GET")
	sr.HandleFunc("/cpu/pg-timeseries", p.CPUPgTimeSeries).Methods("GET")
	sr.HandleFunc("/cpu/query-types", p.CPUQueryTypes).Methods("GET")
	sr.HandleFunc("/postgres/bgwriter", p.BGWriter).Methods("GET")
	sr.HandleFunc("/postgres/archiver", p.Archiver).Methods("GET")
	sr.HandleFunc("/postgres/waits/history", p.WaitEventsHistory).Methods("GET")
	sr.HandleFunc("/postgres/wait-events", p.WaitEventsHistory).Methods("GET")
	sr.HandleFunc("/postgres/io/history", p.DbIOHistory).Methods("GET")
	sr.HandleFunc("/postgres/db-io", p.DbIOHistory).Methods("GET")
	sr.HandleFunc("/postgres/sessions", p.Sessions).Methods("GET")
	sr.HandleFunc("/postgres/databases", p.Databases).Methods("GET")
	sr.HandleFunc("/postgres/config", p.Config).Methods("GET")
	sr.HandleFunc("/postgres/settings/drift", p.SettingsDrift).Methods("GET")
	sr.HandleFunc("/postgres/best-practices", p.BestPractices).Methods("GET")
	sr.HandleFunc("/postgres/best-practices/export", p.ExportBestPracticesCSV).Methods("GET")
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
	sr.HandleFunc("/postgres/locks-blocking/incidents", p.IncidentList).Methods("GET")
	sr.HandleFunc("/postgres/locks-blocking/chronic-blockers", p.ChronicBlockers).Methods("GET")
	sr.HandleFunc("/postgres/replication", p.Replication).Methods("GET")
	sr.HandleFunc("/postgres/sessions", p.Sessions).Methods("GET")
	sr.HandleFunc("/postgres/locks", p.Locks).Methods("GET")
	sr.HandleFunc("/postgres/blocking-tree", p.BlockingTree).Methods("GET")
	sr.HandleFunc("/postgres/queries", p.Queries).Methods("GET")
	sr.HandleFunc("/postgres/alerts", p.Alerts).Methods("GET")
	sr.HandleFunc("/postgres/wait-summary", p.WaitSummary).Methods("GET")
	sr.HandleFunc("/postgres/control-center", p.ControlCenter).Methods("GET")
	sr.HandleFunc("/postgres/control-center/history", p.ControlCenterHistory).Methods("GET")
	sr.HandleFunc("/postgres/metrics/timeseries", p.TimeseriesMetrics).Methods("GET")
	sr.HandleFunc("/postgres/replication-lag/history", p.ReplicationLagHistory).Methods("GET")
	sr.HandleFunc("/postgres/replication-slots", p.ReplicationSlots).Methods("GET")
	sr.HandleFunc("/postgres/disk", p.Disk).Methods("GET")

	// Control Center v2 new routes
	sr.HandleFunc("/postgres/incident-feed", p.IncidentFeed).Methods("GET")
	sr.HandleFunc("/postgres/checkpoint-health/history", p.CheckpointHealthHistory).Methods("GET")
	sr.HandleFunc("/postgres/connection-utilization", p.ConnectionUtilizationHistory).Methods("GET")

	// New Postgres Domain Routes
	sr.HandleFunc("/pg/observability/dashboard", h.PgObservability.GetDashboardData).Methods("GET")
	sr.HandleFunc("/pg/backup/dashboard", h.PgBackup.GetDashboardData).Methods("GET")
	sr.HandleFunc("/postgres/dr-policy", h.PgBackup.GetDRPolicy).Methods("GET")
	sr.HandleFunc("/pg/security/dashboard", h.PgSecurity.GetDashboardData).Methods("GET")

	// SQL Server HA & Replication Domain Routes
	if h.SQLServerHA != nil {
		ha_api.RegisterHARoutes(sr, h.SQLServerHA)
	}
	if h.SQLServerBackup != nil {
		sqlbackup_api.RegisterSQLServerBackupRoutes(sr, h.SQLServerBackup)
	}

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
	sr.HandleFunc("/postgres/pgss/filters", p.PgssFilters).Methods("GET")
	sr.HandleFunc("/postgres/pgss/by-database", p.PgssDbBreakdown).Methods("GET")
	sr.HandleFunc("/postgres/pgss/by-user", p.PgssUserBreakdown).Methods("GET")
	sr.HandleFunc("/sqlserver/cpu-drilldown", m.CPUDrilldown).Methods("GET")
	sr.HandleFunc("/sqlserver/cpu-scheduler-stats", m.CPUSchedulerStats).Methods("GET")
	sr.HandleFunc("/sqlserver/server-properties", m.ServerProperties).Methods("GET")
	sr.HandleFunc("/sqlserver/ag-health", m.AGHealth).Methods("GET")
	sr.HandleFunc("/sqlserver/ag-health/history", m.AGHealthTimeSeries).Methods("GET")
	sr.HandleFunc("/sqlserver/ag-cluster", m.AGClusterStatus).Methods("GET")
	sr.HandleFunc("/sqlserver/replication-status", m.ReplicationStatus).Methods("GET")
	sr.HandleFunc("/sqlserver/log-shipping", m.LogShipping).Methods("GET")
	sr.HandleFunc("/sqlserver/db-throughput", m.DBThroughput).Methods("GET")
	sr.HandleFunc("/sqlserver/best-practices", m.BestPractices).Methods("GET")
	sr.HandleFunc("/sqlserver/best-practices/export", m.ExportBestPracticesCSV).Methods("GET")
	sr.HandleFunc("/sqlserver/guardrails", m.Guardrails).Methods("GET")
	sr.HandleFunc("/sqlserver/guardrails/export", m.ExportGuardrailsCSV).Methods("GET")
	sr.HandleFunc("/sqlserver/jobs", m.Jobs).Methods("GET")
	sr.HandleFunc("/sqlserver/overview", m.Overview).Methods("GET")
	sr.HandleFunc("/sqlserver/deadlocks/history", m.DeadlockHistory).Methods("GET")
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
	sr.HandleFunc("/timescale/sqlserver/metrics", ts.GetSQLServerMetrics).Methods("GET")
	sr.HandleFunc("/timescale/sqlserver/cpu-history", ts.SqlServerCPUHistory).Methods("GET")
	sr.HandleFunc("/timescale/sqlserver/memory-drilldown", ts.SqlServerMemoryDrilldown).Methods("GET")
	sr.HandleFunc("/timescale/sqlserver/top-queries", ts.SqlServerTopQueries).Methods("GET")
	sr.HandleFunc("/timescale/sqlserver/query-stats-dashboard", ts.SqlServerQueryStatsDashboard).Methods("GET")
	sr.HandleFunc("/timescale/sqlserver/query-stats-timeseries", ts.SqlServerQueryStatsTimeSeries).Methods("GET")
	sr.HandleFunc("/timescale/sqlserver/query-stats/trend", ts.SqlServerQueryStatsTrend).Methods("GET")
	sr.HandleFunc("/timescale/sqlserver/long-running-queries", ts.SqlServerLongRunningQueries).Methods("GET")
	sr.HandleFunc("/timescale/postgres/throughput", ts.GetPostgresThroughput).Methods("GET")
	sr.HandleFunc("/timescale/postgres/connections", ts.PostgresConnections).Methods("GET")
	if pgSih != nil {
		sr.HandleFunc("/timescale/postgres/storage-index-health/index-usage", pgSih.GetIndexUsage).Methods("GET")
		sr.HandleFunc("/timescale/postgres/storage-index-health/table-usage", pgSih.GetTableUsage).Methods("GET")
		sr.HandleFunc("/timescale/postgres/storage-index-health/growth", pgSih.GetGrowth).Methods("GET")
		sr.HandleFunc("/timescale/postgres/storage-index-health/dashboard", pgSih.GetDashboard).Methods("GET")
		sr.HandleFunc("/timescale/postgres/storage-index-health/filters", pgSih.GetFilterOptions).Methods("GET")
		sr.HandleFunc("/timescale/postgres/storage-index-health/index-definition", pgSih.GetIndexDefinition).Methods("GET")
	}
	if msSih != nil {
		sr.HandleFunc("/timescale/sqlserver/storage-index-health/index-usage", msSih.GetIndexUsage).Methods("GET")
		sr.HandleFunc("/timescale/sqlserver/storage-index-health/table-usage", msSih.GetTableUsage).Methods("GET")
		sr.HandleFunc("/timescale/sqlserver/storage-index-health/growth", msSih.GetGrowth).Methods("GET")
		sr.HandleFunc("/timescale/sqlserver/storage-index-health/dashboard", msSih.GetDashboard).Methods("GET")
		sr.HandleFunc("/timescale/sqlserver/storage-index-health/filters", msSih.GetFilterOptions).Methods("GET")
		sr.HandleFunc("/timescale/sqlserver/storage-index-health/index-definition", msSih.GetIndexDefinition).Methods("GET")
	}
	if uSih != nil {
		sr.HandleFunc("/timescale/storage-index-health/index-usage", uSih.GetIndexUsage).Methods("GET")
		sr.HandleFunc("/timescale/storage-index-health/table-usage", uSih.GetTableUsage).Methods("GET")
		sr.HandleFunc("/timescale/storage-index-health/growth", uSih.GetGrowth).Methods("GET")
		sr.HandleFunc("/timescale/storage-index-health/dashboard", uSih.GetDashboard).Methods("GET")
		sr.HandleFunc("/timescale/storage-index-health/filters", uSih.GetFilterOptions).Methods("GET")
		sr.HandleFunc("/timescale/storage-index-health/index-definition", uSih.GetIndexDefinition).Methods("GET")
		sr.HandleFunc("/timescale/storage-index-health/index-usage-trend", uSih.GetIndexUsageTrend).Methods("GET")
	}
sr.HandleFunc("/health/score", he.Score).Methods("GET")
	sr.HandleFunc("/health/anomalies", he.Anomalies).Methods("GET")
	sr.HandleFunc("/health/regressed-queries", he.RegressedQueries).Methods("GET")
	sr.HandleFunc("/health/wait-spikes", he.WaitSpikes).Methods("GET")
	sr.HandleFunc("/health/metrics-history", he.MetricsHistory).Methods("GET")
	sr.HandleFunc("/health/intervals", he.GetCollectorIntervals).Methods("GET")
	sr.HandleFunc("/incidents/timeline", he.IncidentsTimeline).Methods("GET")
	sr.HandleFunc("/queries/bottlenecks", q.GetQueryBottlenecks).Methods("GET")
	sr.HandleFunc("/queries/query-store/sql-text", q.GetSQLText).Methods("GET")
	sr.HandleFunc("/rules/best-practices", rulesBestPractices).Methods("GET")

	// SQL Server Query Analysis Dashboard
	if qa := h.SqlServerQueryAnalysis; qa != nil {
		sr.HandleFunc("/sqlserver/query-analysis/default-database", qa.GetDefaultDatabase).Methods("GET")
		sr.HandleFunc("/sqlserver/query-analysis/summary", qa.GetSummary).Methods("GET")
		sr.HandleFunc("/sqlserver/query-analysis/regressions", qa.GetRegressions).Methods("GET")
		sr.HandleFunc("/sqlserver/query-analysis/plan-instability", qa.GetPlanInstability).Methods("GET")
		sr.HandleFunc("/sqlserver/query-analysis/top-queries", qa.GetTopQueries).Methods("GET")
		sr.HandleFunc("/sqlserver/query-analysis/top-queries-trends", qa.GetTopQueryTrends).Methods("GET")
		sr.HandleFunc("/sqlserver/query-analysis/query-plans", qa.GetQueryPlans).Methods("GET")
		sr.HandleFunc("/sqlserver/query-analysis/query-wait-stats", qa.GetQueryWaitStats).Methods("GET")
	}

	// SQL Server Watched Query Analyzer
	if wq := h.SqlServerWatchedQuery; wq != nil {
		sr.HandleFunc("/sqlserver/watched-queries", wq.ListQueries).Methods("GET")
		sr.HandleFunc("/sqlserver/watched-queries", wq.AddQuery).Methods("POST")
		sr.HandleFunc("/sqlserver/watched-queries", wq.DeleteQuery).Methods("DELETE")
		sr.HandleFunc("/sqlserver/watched-queries/detail", wq.GetDetail).Methods("GET")
		sr.HandleFunc("/sqlserver/watched-queries/plan-analysis", wq.GetPlanAnalysis).Methods("GET")
	}

	// SQL Server Plan Analyzer
	if pa := h.SqlServerPlanAnalyzer; pa != nil {
		sr.HandleFunc("/sqlserver/plan/analyze", pa.AnalyzePlan).Methods("POST")
	}

	// SQL Server Intelligence Report
	sr.HandleFunc("/sqlserver/intelligence-report/status", ir.Status).Methods("GET")
	sr.HandleFunc("/sqlserver/intelligence-report/latest", ir.Latest).Methods("GET")
	sr.HandleFunc("/sqlserver/intelligence-report/analyze", ir.Analyze).Methods("POST")
	sr.HandleFunc("/sqlserver/intelligence-report/report/{run_id}", ir.GetReport).Methods("GET")

	// Cold Storage
	if h.ColdStorage != nil {
		sr.HandleFunc("/cold-storage/status", h.ColdStorage.GetStatus).Methods("GET")
		sr.HandleFunc("/cold-storage/runs", h.ColdStorage.GetRuns).Methods("GET")
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
	sr.HandleFunc("/postgres/session/cancel", p.CancelBackend).Methods("POST")
	sr.HandleFunc("/postgres/session/terminate", p.TerminateBackend).Methods("POST")
}

// registerDashboardWidgetRoutes attaches widget list and (future) query execute for authenticated users.
func registerDashboardWidgetRoutes(sr *mux.Router, d *handlers.DashboardHandlers) {
	sr.HandleFunc("/dashboard/widgets", d.GetDashboardWidgets).Methods("GET")
	sr.HandleFunc("/dashboard/query/execute", d.ExecuteWidgetQuery).Methods("POST")
}
