package api

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/rsharma155/sql_optima/internal/api/handlers"
)

// monitoringHandlers groups HTTP handlers registered for dashboard and live APIs.
type monitoringHandlers struct {
	Mssql     *handlers.MssqlHandlers
	Postgres  *handlers.PostgresHandlers
	Live      *handlers.LiveHandlers
	Timescale *handlers.TimescaleHandlers
	Health    *handlers.HealthHandlers
	Dashboard *handlers.DashboardHandlers
	Query     *handlers.QueryHandlers
}

// registerMonitoringReadRoutes attaches read-only monitoring endpoints (viewer, dba, or admin).
func registerMonitoringReadRoutes(sr *mux.Router, h *monitoringHandlers, rulesBestPractices http.HandlerFunc) {
	m := h.Mssql
	p := h.Postgres
	l := h.Live
	ts := h.Timescale
	he := h.Health
	q := h.Query

	sr.HandleFunc("/mssql/dashboard", m.Dashboard).Methods("GET")
	sr.HandleFunc("/mssql/dashboard/v2", m.DashboardV2).Methods("GET")
	sr.HandleFunc("/mssql/performance-debt", m.PerformanceDebt).Methods("GET")
	sr.HandleFunc("/postgres/dashboard", p.Dashboard).Methods("GET")
	sr.HandleFunc("/postgres/db-observation", p.DBObservation).Methods("GET")
	sr.HandleFunc("/postgres/overview", p.Overview).Methods("GET")
	sr.HandleFunc("/postgres/server-info", p.ServerInfo).Methods("GET")
	sr.HandleFunc("/postgres/system-stats", p.SystemStats).Methods("GET")
	sr.HandleFunc("/postgres/system-stats/history", p.SystemStatsHistory).Methods("GET")
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
	sr.HandleFunc("/mssql/cpu-drilldown", m.CPUDrilldown).Methods("GET")
	sr.HandleFunc("/mssql/cpu-scheduler-stats", m.CPUSchedulerStats).Methods("GET")
	sr.HandleFunc("/mssql/server-properties", m.ServerProperties).Methods("GET")
	sr.HandleFunc("/mssql/ag-health", m.AGHealth).Methods("GET")
	sr.HandleFunc("/mssql/db-throughput", m.DBThroughput).Methods("GET")
	sr.HandleFunc("/mssql/best-practices", m.BestPractices).Methods("GET")
	sr.HandleFunc("/mssql/guardrails", m.Guardrails).Methods("GET")
	sr.HandleFunc("/mssql/jobs", m.Jobs).Methods("GET")
	sr.HandleFunc("/mssql/overview", m.Overview).Methods("GET")
	sr.HandleFunc("/mssql/latch-stats", m.LatchStats).Methods("GET")
	sr.HandleFunc("/mssql/waiting-tasks", m.WaitingTasks).Methods("GET")
	sr.HandleFunc("/mssql/memory-grants", m.MemoryGrants).Methods("GET")
	sr.HandleFunc("/mssql/scheduler-wg", m.SchedulerWorkers).Methods("GET")
	sr.HandleFunc("/mssql/procedure-stats", m.ProcedureStats).Methods("GET")
	sr.HandleFunc("/mssql/file-io-latency", m.FileIOLatency).Methods("GET")
	sr.HandleFunc("/mssql/spinlock-stats", m.SpinlockStats).Methods("GET")
	sr.HandleFunc("/mssql/memory-clerks", m.MemoryClerks).Methods("GET")
	sr.HandleFunc("/mssql/tempdb-stats", m.TempdbStats).Methods("GET")
	sr.HandleFunc("/mssql/plan-cache-health", m.PlanCacheHealth).Methods("GET")
	sr.HandleFunc("/mssql/memory-grant-waiters", m.MemoryGrantWaiters).Methods("GET")
	sr.HandleFunc("/mssql/tempdb-top-consumers", m.TempdbTopConsumers).Methods("GET")
	sr.HandleFunc("/mssql/wait-categories", m.WaitCategories).Methods("GET")
	sr.HandleFunc("/timescale/status", ts.Status).Methods("GET")
	sr.HandleFunc("/timescale/mssql/metrics", ts.MssqlMetrics).Methods("GET")
	sr.HandleFunc("/timescale/mssql/top-queries", ts.MssqlTopQueries).Methods("GET")
	sr.HandleFunc("/timescale/mssql/long-running-queries", ts.MssqlLongRunningQueries).Methods("GET")
	sr.HandleFunc("/timescale/postgres/throughput", ts.PostgresThroughput).Methods("GET")
	sr.HandleFunc("/timescale/postgres/connections", ts.PostgresConnections).Methods("GET")
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
	sr.HandleFunc("/rules/best-practices", rulesBestPractices).Methods("GET")
}

// registerMonitoringElevatedRoutes attaches diagnostics that should be limited to dba or admin.
func registerMonitoringElevatedRoutes(sr *mux.Router, m *handlers.MssqlHandlers, explainAnalyze, explainOptimize http.HandlerFunc) {
	sr.HandleFunc("/postgres/explain/analyze", explainAnalyze).Methods("POST")
	sr.HandleFunc("/postgres/explain/optimize", explainOptimize).Methods("POST")
	sr.HandleFunc("/mssql/xevents", m.XEvents).Methods("GET")
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
