// Package api provides HTTP API endpoints for the monitoring dashboard.
// It handles routing, authentication, and response formatting for all REST endpoints.
package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"github.com/rsharma155/sql_optima/internal/api/handlers"
	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/middleware"
	"github.com/rsharma155/sql_optima/internal/service"

	"github.com/gorilla/mux"
)

type authContextKey string

const authCtxKey authContextKey = "auth_claims"

func requireAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "missing authorization header"})
			return
		}

		parts := splitAuthHeader(authHeader)
		if parts == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid authorization format"})
			return
		}

		claims, err := middleware.ValidateToken(parts[1])
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid or expired token"})
			return
		}

		ctx := context.WithValue(r.Context(), authCtxKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func splitAuthHeader(header string) []string {
	for i := 0; i < len(header); i++ {
		if header[i] == ' ' {
			prefix := header[:i]
			token := header[i+1:]
			if prefix == "Bearer" && token != "" {
				return []string{prefix, token}
			}
		}
	}
	return nil
}

func RegisterHealthRoutes(r *mux.Router, cfg *config.Config, metricsSvc *service.MetricsService, queriesLoaded bool, loginLimiter *middleware.LoginRateLimiter) {
	r.HandleFunc("/api/health", HandleHealthLiveness).Methods("GET")
	r.HandleFunc("/api/health/ready", func(w http.ResponseWriter, r *http.Request) {
		HandleHealthReadiness(w, r, cfg, queriesLoaded)
	}).Methods("GET")

	r.HandleFunc("/api/config", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"instances": cfg.Instances,
		})
	}).Methods("GET")

	protectedAPI := r.PathPrefix("/api").Subrouter()

	authH := handlers.NewAuthHandlers(metricsSvc)
	adminH := handlers.NewAdminHandlers(metricsSvc)
	widgetAdminH := handlers.NewWidgetAdminHandlers(metricsSvc)
	mssqlH := handlers.NewMssqlHandlers(metricsSvc, cfg)
	postgresH := handlers.NewPostgresHandlers(metricsSvc, cfg)
	liveH := handlers.NewLiveHandlers(metricsSvc, cfg)
	timescaleH := handlers.NewTimescaleHandlers(metricsSvc, cfg)
	healthH := handlers.NewHealthHandlers(metricsSvc, cfg)
	dashboardH := handlers.NewDashboardHandlers(metricsSvc, cfg)
	queryH := handlers.NewQueryHandlers(metricsSvc, cfg)

	// Public API routes - no auth required
	publicAPI := r.PathPrefix("/api").Subrouter()

	// JWT login: canonical /api/login plus /api/auth/login for REST-style clients.
	// Both share one rate-limited handler (counts toward the same per-IP budget).
	if loginLimiter != nil {
		loginHF := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authH.Login(w, r)
		})
		limitedLogin := middleware.LoginRateLimitMiddleware(loginLimiter, loginHF)
		publicAPI.Handle("/login", limitedLogin).Methods("POST")
		publicAPI.Handle("/auth/login", limitedLogin).Methods("POST")
	}

	publicAPI.HandleFunc("/mssql/dashboard", mssqlH.Dashboard).Methods("GET")
	publicAPI.HandleFunc("/mssql/dashboard/v2", mssqlH.DashboardV2).Methods("GET")
	publicAPI.HandleFunc("/mssql/performance-debt", mssqlH.PerformanceDebt).Methods("GET")
	publicAPI.HandleFunc("/postgres/dashboard", postgresH.Dashboard).Methods("GET")
	// PostgreSQL read-only dashboards: keep public like MSSQL dashboards.
	publicAPI.HandleFunc("/postgres/db-observation", postgresH.DBObservation).Methods("GET")
	publicAPI.HandleFunc("/postgres/overview", postgresH.Overview).Methods("GET")
	publicAPI.HandleFunc("/postgres/server-info", postgresH.ServerInfo).Methods("GET")
	publicAPI.HandleFunc("/postgres/system-stats", postgresH.SystemStats).Methods("GET")
	publicAPI.HandleFunc("/postgres/system-stats/history", postgresH.SystemStatsHistory).Methods("GET")
	publicAPI.HandleFunc("/postgres/bgwriter", postgresH.BGWriter).Methods("GET")
	publicAPI.HandleFunc("/postgres/archiver", postgresH.Archiver).Methods("GET")
	publicAPI.HandleFunc("/postgres/waits/history", postgresH.WaitEventsHistory).Methods("GET")
	publicAPI.HandleFunc("/postgres/io/history", postgresH.DbIOHistory).Methods("GET")
	publicAPI.HandleFunc("/postgres/settings/drift", postgresH.SettingsDrift).Methods("GET")
	publicAPI.HandleFunc("/postgres/databases", postgresH.Databases).Methods("GET")
	publicAPI.HandleFunc("/postgres/config", postgresH.Config).Methods("GET")
	publicAPI.HandleFunc("/postgres/best-practices", postgresH.BestPractices).Methods("GET")
	publicAPI.HandleFunc("/postgres/storage", postgresH.Storage).Methods("GET")
	publicAPI.HandleFunc("/postgres/database-size", postgresH.DatabaseSize).Methods("GET")
	publicAPI.HandleFunc("/postgres/vacuum/progress", postgresH.VacuumProgress).Methods("GET")
	publicAPI.HandleFunc("/postgres/vacuum/progress/history", postgresH.VacuumProgressHistory).Methods("GET")
	publicAPI.HandleFunc("/postgres/sessions/state/history", postgresH.SessionStateHistory).Methods("GET")
	publicAPI.HandleFunc("/postgres/table-maintenance/history", postgresH.TableMaintenanceHistory).Methods("GET")
	publicAPI.HandleFunc("/postgres/table-maintenance/latest", postgresH.TableMaintenanceLatest).Methods("GET")
	publicAPI.HandleFunc("/postgres/pooler/latest", postgresH.PoolerLatest).Methods("GET")
	publicAPI.HandleFunc("/postgres/pooler/history", postgresH.PoolerHistory).Methods("GET")
	publicAPI.HandleFunc("/postgres/deadlocks/history", postgresH.DeadlocksHistory).Methods("GET")
	publicAPI.HandleFunc("/postgres/locks/wait-history", postgresH.LockWaitHistory).Methods("GET")
	publicAPI.HandleFunc("/postgres/replication", postgresH.Replication).Methods("GET")
	publicAPI.HandleFunc("/postgres/sessions", postgresH.Sessions).Methods("GET")
	publicAPI.HandleFunc("/postgres/locks", postgresH.Locks).Methods("GET")
	publicAPI.HandleFunc("/postgres/blocking-tree", postgresH.BlockingTree).Methods("GET")
	publicAPI.HandleFunc("/postgres/queries", postgresH.Queries).Methods("GET")
	publicAPI.HandleFunc("/postgres/alerts", postgresH.Alerts).Methods("GET")
	publicAPI.HandleFunc("/postgres/control-center", postgresH.ControlCenter).Methods("GET")
	publicAPI.HandleFunc("/postgres/control-center/history", postgresH.ControlCenterHistory).Methods("GET")
	publicAPI.HandleFunc("/postgres/replication-lag/history", postgresH.ReplicationLagHistory).Methods("GET")
	publicAPI.HandleFunc("/postgres/replication-slots", postgresH.ReplicationSlots).Methods("GET")
	publicAPI.HandleFunc("/postgres/disk", postgresH.Disk).Methods("GET")
	publicAPI.HandleFunc("/postgres/backups/latest", postgresH.BackupLatest).Methods("GET")
	publicAPI.HandleFunc("/postgres/backups/history", postgresH.BackupHistory).Methods("GET")
	publicAPI.HandleFunc("/postgres/logs/summary", postgresH.LogsSummary).Methods("GET")
	publicAPI.HandleFunc("/postgres/logs/recent", postgresH.LogsRecent).Methods("GET")
	publicAPI.HandleFunc("/postgres/explain/analyze", handlers.PgExplainAnalyze).Methods("POST")
	publicAPI.HandleFunc("/postgres/explain/optimize", handlers.PgExplainOptimize).Methods("POST")
	publicAPI.HandleFunc("/mssql/cpu-drilldown", mssqlH.CPUDrilldown).Methods("GET")
	publicAPI.HandleFunc("/mssql/cpu-scheduler-stats", mssqlH.CPUSchedulerStats).Methods("GET")
	publicAPI.HandleFunc("/mssql/server-properties", mssqlH.ServerProperties).Methods("GET")
	publicAPI.HandleFunc("/mssql/ag-health", mssqlH.AGHealth).Methods("GET")
	publicAPI.HandleFunc("/mssql/db-throughput", mssqlH.DBThroughput).Methods("GET")
	publicAPI.HandleFunc("/mssql/best-practices", mssqlH.BestPractices).Methods("GET")
	publicAPI.HandleFunc("/mssql/guardrails", mssqlH.Guardrails).Methods("GET")
	publicAPI.HandleFunc("/mssql/jobs", mssqlH.Jobs).Methods("GET")
	publicAPI.HandleFunc("/mssql/overview", mssqlH.Overview).Methods("GET")
	publicAPI.HandleFunc("/mssql/latch-stats", mssqlH.LatchStats).Methods("GET")
	publicAPI.HandleFunc("/mssql/waiting-tasks", mssqlH.WaitingTasks).Methods("GET")
	publicAPI.HandleFunc("/mssql/memory-grants", mssqlH.MemoryGrants).Methods("GET")
	publicAPI.HandleFunc("/mssql/scheduler-wg", mssqlH.SchedulerWorkers).Methods("GET")
	publicAPI.HandleFunc("/mssql/procedure-stats", mssqlH.ProcedureStats).Methods("GET")
	publicAPI.HandleFunc("/mssql/file-io-latency", mssqlH.FileIOLatency).Methods("GET")
	publicAPI.HandleFunc("/mssql/spinlock-stats", mssqlH.SpinlockStats).Methods("GET")
	publicAPI.HandleFunc("/mssql/memory-clerks", mssqlH.MemoryClerks).Methods("GET")
	publicAPI.HandleFunc("/mssql/tempdb-stats", mssqlH.TempdbStats).Methods("GET")
	publicAPI.HandleFunc("/mssql/plan-cache-health", mssqlH.PlanCacheHealth).Methods("GET")
	publicAPI.HandleFunc("/mssql/memory-grant-waiters", mssqlH.MemoryGrantWaiters).Methods("GET")
	publicAPI.HandleFunc("/mssql/tempdb-top-consumers", mssqlH.TempdbTopConsumers).Methods("GET")
	publicAPI.HandleFunc("/mssql/wait-categories", mssqlH.WaitCategories).Methods("GET")
	publicAPI.HandleFunc("/timescale/status", timescaleH.Status).Methods("GET")
	publicAPI.HandleFunc("/timescale/mssql/metrics", timescaleH.MssqlMetrics).Methods("GET")
	publicAPI.HandleFunc("/timescale/mssql/top-queries", timescaleH.MssqlTopQueries).Methods("GET")
	publicAPI.HandleFunc("/timescale/mssql/long-running-queries", timescaleH.MssqlLongRunningQueries).Methods("GET")
	publicAPI.HandleFunc("/timescale/postgres/throughput", timescaleH.PostgresThroughput).Methods("GET")
	publicAPI.HandleFunc("/timescale/postgres/connections", timescaleH.PostgresConnections).Methods("GET")
	publicAPI.HandleFunc("/live/kpis", liveH.KPIs).Methods("GET")
	publicAPI.HandleFunc("/live/running-queries", liveH.RunningQueries).Methods("GET")
	publicAPI.HandleFunc("/live/blocking", liveH.Blocking).Methods("GET")
	publicAPI.HandleFunc("/live/io-latency", liveH.IOLatency).Methods("GET")
	publicAPI.HandleFunc("/live/tempdb", liveH.TempDB).Methods("GET")
	publicAPI.HandleFunc("/live/waits", liveH.Waits).Methods("GET")
	publicAPI.HandleFunc("/live/connections", liveH.Connections).Methods("GET")
	publicAPI.HandleFunc("/health/score", healthH.Score).Methods("GET")
	publicAPI.HandleFunc("/health/anomalies", healthH.Anomalies).Methods("GET")
	publicAPI.HandleFunc("/health/regressed-queries", healthH.RegressedQueries).Methods("GET")
	publicAPI.HandleFunc("/health/wait-spikes", healthH.WaitSpikes).Methods("GET")
	publicAPI.HandleFunc("/health/metrics-history", healthH.MetricsHistory).Methods("GET")
	publicAPI.HandleFunc("/incidents/timeline", healthH.IncidentsTimeline).Methods("GET")
	publicAPI.HandleFunc("/queries/bottlenecks", queryH.Bottlenecks).Methods("GET")

	// Protected API routes - auth required
	protectedAPI.Use(requireAuthMiddleware)

	protectedAPI.HandleFunc("/auth/me", authH.Me).Methods("GET")

	protectedAPI.HandleFunc("/admin/users", adminH.CreateUser).Methods("POST")
	protectedAPI.HandleFunc("/admin/users", adminH.ListUsers).Methods("GET")
	protectedAPI.HandleFunc("/admin/users/{id}", adminH.DeleteUser).Methods("DELETE")
	protectedAPI.HandleFunc("/admin/users/{id}/role", adminH.UpdateUserRole).Methods("PUT")

	protectedAPI.HandleFunc("/admin/widgets/{id}", widgetAdminH.UpdateWidget).Methods("PUT")
	protectedAPI.HandleFunc("/admin/widgets/{id}/restore", widgetAdminH.RestoreWidget).Methods("POST")
	protectedAPI.HandleFunc("/admin/widgets/{id}", widgetAdminH.GetWidget).Methods("GET")
	protectedAPI.HandleFunc("/admin/widgets", widgetAdminH.ListWidgets).Methods("GET")

	// /health/*, /incidents/* - defined above without auth

	// (All postgres GET endpoints are public; keep only mutating actions protected.)
	protectedAPI.HandleFunc("/postgres/kill-session", postgresH.KillSession).Methods("POST")
	protectedAPI.HandleFunc("/postgres/reset-queries", postgresH.ResetQueries).Methods("POST")
	protectedAPI.HandleFunc("/postgres/backups/report", postgresH.BackupReport).Methods("POST")
	protectedAPI.HandleFunc("/postgres/logs/report", postgresH.LogsReport).Methods("POST")

	// /mssql/*, /live/*, /timescale/*, /health/*, /incidents/*, /queries/* - defined above without auth
	protectedAPI.HandleFunc("/mssql/xevents", mssqlH.XEvents).Methods("GET")

	protectedAPI.HandleFunc("/dashboard/widgets", dashboardH.Widgets).Methods("GET")
	protectedAPI.HandleFunc("/dashboard/query/execute", dashboardH.ExecuteQuery).Methods("POST")

	var rulesH *handlers.RulesHandler
	var rulesHErr error
	publicAPI.HandleFunc("/rules/best-practices", func(w http.ResponseWriter, r *http.Request) {
		if rulesH == nil {
			rulesH, rulesHErr = handlers.NewRulesHandlerFromConfig(cfg)
			if rulesHErr != nil {
				log.Printf("[Router] RulesHandler initialization error: %v", rulesHErr)
			}
		}
		if rulesH != nil {
			rulesH.BestPractices(w, r)
		} else {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]string{"error": "rule engine not available"})
		}
	}).Methods("GET")
}
