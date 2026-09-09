// Package api provides HTTP API endpoints for the monitoring dashboard.
// It handles routing, authentication, and response formatting for all REST endpoints.
// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Defines API handlers and routes for all monitoring endpoints. Registers health routes, authentication, and monitoring handlers for both SQLSERVER and PostgreSQL databases.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"

	"github.com/gorilla/mux"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rsharma155/sql_optima/internal/api/handlers"
	"github.com/rsharma155/sql_optima/internal/config"
	pg_backup_api "github.com/rsharma155/sql_optima/internal/domain/postgres_backup_dr/api"
	pg_obs_api "github.com/rsharma155/sql_optima/internal/domain/postgres_observability/api"
	pg_security_api "github.com/rsharma155/sql_optima/internal/domain/postgres_security/api"
	ha_api "github.com/rsharma155/sql_optima/internal/domain/sqlserver_ha_replication/api"
	sqlbackup_api "github.com/rsharma155/sql_optima/internal/domain/sqlserver_backup_recovery/api"
	"github.com/rsharma155/sql_optima/internal/middleware"
	"github.com/rsharma155/sql_optima/internal/repository"
	"github.com/rsharma155/sql_optima/internal/service"
	"github.com/rsharma155/sql_optima/internal/storage/cold"
)


func RegisterHealthRoutes(r *mux.Router, cfg *config.Config, metricsSvc *service.MetricsService, loginLimiter *middleware.LoginRateLimiter, sec config.Security) {
	r.HandleFunc("/api/health", HandleHealthLiveness).Methods("GET")
	r.HandleFunc("/api/health/ready", func(w http.ResponseWriter, req *http.Request) {
		HandleHealthReadiness(w, req, cfg, metricsSvc)
	}).Methods("GET")

	r.HandleFunc("/api/auth/status", HandleAuthStatus(sec)).Methods("GET")

	// SQL Server Intelligence Report — declared here so the /api/config closure below can reference it.
	var irSvc *service.IntelligenceReportService
	var irH *handlers.IntelligenceReportHandlers

	if !sec.AuthRequired {
		r.HandleFunc("/api/config", func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			// Lazy-reload: if startup failed to decrypt/load instances, recover on first request.
			if len(cfg.Instances) == 0 && metricsSvc != nil && metricsSvc.RegistryReload != nil {
				metricsSvc.RegistryReload()
			}
			statuses := map[string]string{}
			if metricsSvc != nil {
				statuses = metricsSvc.GetAllInstanceStatuses()
			}
			instances := make([]config.Instance, 0, len(cfg.Instances))
			for _, inst := range cfg.Instances {
				copyInst := inst
				if status, ok := statuses[inst.Name]; ok {
					copyInst.Available = status == "online"
				} else {
					copyInst.Available = true
				}
				instances = append(instances, copyInst)
			}

			intelActive := false
			if irSvc != nil {
				intelActive = irSvc.CheckStatus(req.Context())
			}

			coldCfg := cold.ConfigFromEnv()
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"instances":                    instances,
				"feature_flags":                cfg.FeatureFlags,
				"intelligence_active":          intelActive,
				"cold_storage_enabled":         coldCfg.Enabled,
				"cold_storage_query_available": coldCfg.TrinoURL != "",
				"max_dashboard_range_days":     int(handlers.MaxDashboardTimeRange().Hours() / 24),
			})
		}).Methods("GET")
	}

	authH := handlers.NewAuthHandlers(metricsSvc, loginLimiter)
	adminH := handlers.NewAdminHandlers(metricsSvc)
	adminServersH := handlers.NewAdminServerHandlers(metricsSvc)
	adminCollectorH := handlers.NewAdminCollectorHandlers(metricsSvc)
	adminSqlServerDiagH := handlers.NewAdminSqlServerDiagnosticsHandlers(metricsSvc, cfg)
	widgetAdminH := handlers.NewWidgetAdminHandlers(metricsSvc)
	sqlserverH := handlers.NewSqlServerHandlers(metricsSvc, cfg)
	postgresH := handlers.NewPostgresHandlers(metricsSvc, cfg)
	coldQueryH := handlers.NewColdQueryHandlers() // nil when COLD_STORAGE_TRINO_URL is not set
	timescaleH := handlers.NewTimescaleHandlers(metricsSvc, coldQueryH)
	pgSihH := handlers.NewPgStorageIndexHealthHandlers(metricsSvc, cfg)
	msSihH := handlers.NewSqlServerStorageHandlers(metricsSvc, cfg)
	unifiedSihH := handlers.NewStorageIndexHealthTimescaleHandlers(metricsSvc, cfg)
	healthH := handlers.NewHealthHandlers(metricsSvc, cfg)
	dashboardH := handlers.NewDashboardHandlers(metricsSvc)
	queryH := handlers.NewQueryHandlers(metricsSvc)
	sqlserverQAH := handlers.NewSqlServerQueryAnalysisHandlers(metricsSvc, cfg)
	sqlserverWQH := handlers.NewWatchedQueryHandlers(metricsSvc, cfg)
	sqlserverWLH := handlers.NewSqlServerWorkloadHandlers(metricsSvc, cfg)
	sqlserverPAH := handlers.NewSqlServerPlanHandlers(metricsSvc, cfg)
	osMetricsH := handlers.NewOSMetricsHandlers(metricsSvc)
	osCollectorH := handlers.NewOSCollectorHandlers(metricsSvc)
	sqlserverLockH := handlers.NewSqlServerLockHandlers(metricsSvc, cfg)
	sqlserverWaitStatsH := handlers.NewSqlServerWaitStatsHandlers(metricsSvc, cfg)

	var coldStorageH *handlers.ColdStorageHandlers
	if pool := metricsSvc.GetTimescaleDBPool(); pool != nil {
		coldStorageH = handlers.NewColdStorageHandlers(pool)
		revokeStore := repository.NewOSAgentRevokeStore(pool)
		osCollectorH.SetOSAgentRevoker(revokeStore)
		middleware.SetOSAgentRevokeChecker(revokeStore)
	}

	// SQL Server Intelligence Report — initialize with nil service if pool is missing to ensure routes are registered.
	if pool := metricsSvc.GetTimescaleDBPool(); pool != nil {
		irSvc = service.NewIntelligenceReportService(pool)
	}
	irH = handlers.NewIntelligenceReportHandlers(irSvc, cfg)

	// New Postgres Domain Handlers
	pgObsH := pg_obs_api.NewPostgresObservabilityHandler(metricsSvc)
	pgBackupH := pg_backup_api.NewPostgresBackupHandler(metricsSvc)
	pgSecurityH := pg_security_api.NewPostgresSecurityHandler(metricsSvc)

	// SQL Server HA & Replication Domain Handler
	sqlServerHAH := ha_api.NewHAReplicationHandler(metricsSvc)
	sqlServerBackupH := sqlbackup_api.NewSQLServerBackupHandler(metricsSvc)

	mon := &monitoringHandlers{
		SqlServer: sqlserverH, Postgres: postgresH, Timescale: timescaleH,
		Health: healthH, Dashboard: dashboardH, Query: queryH,
		PgSIH: pgSihH, SqlServerSIH: msSihH, UnifiedSIH: unifiedSihH,
		SqlServerQueryAnalysis: sqlserverQAH, SqlServerWatchedQuery: sqlserverWQH,
		SqlServerWorkload: sqlserverWLH, SqlServerPlanAnalyzer: sqlserverPAH,
		SqlServerLocks: sqlserverLockH, SqlServerWaitStats: sqlserverWaitStatsH,
		IntelligenceReport: irH,
		ColdStorage: coldStorageH,
		ColdQuery:   coldQueryH,
		PgObservability: pgObsH, PgBackup: pgBackupH, PgSecurity: pgSecurityH,
		SQLServerHA: sqlServerHAH, SQLServerBackup: sqlServerBackupH,
		}

	// ── Shared notifier (config loaded from DB at startup, hot-reloadable via admin panel) ──
	sharedNotifier := service.NewNotifier(service.NotifierConfig{
		WebhookURL:          os.Getenv("ALERT_WEBHOOK_URL"),
		SlackWebhookURL:     os.Getenv("SLACK_WEBHOOK_URL"),
		PagerDutyRoutingKey: os.Getenv("PAGERDUTY_ROUTING_KEY"),
		EmailSMTPJSON:       os.Getenv("ALERT_SMTP_JSON"),
	}, slog.Default())

	// ── Admin notification handler (wired to shared notifier) ──────────────────
	var adminNotifH *handlers.AdminNotificationHandlers
	if tsPool := metricsSvc.GetTimescaleDBPool(); tsPool != nil {
		notifRepo := repository.NewNotificationConfigRepository(tsPool)
		adminNotifH = handlers.NewAdminNotificationHandlers(notifRepo, sharedNotifier, metricsSvc)
		sharedNotifier.LoadFromDB(context.Background(), notifRepo)
	}

	// ── Alert engine wiring ────────────────────────────────────
	var alertH *handlers.AlertHandlers
	getAlertH := func() *handlers.AlertHandlers {
		if alertH == nil {
			if tsPool := metricsSvc.GetTimescaleDBPool(); tsPool != nil {
				alertRepo := repository.NewTimescaleAlertStore(tsPool)
				maintRepo := repository.NewAlertMaintenanceStore(tsPool)

				evaluators := []service.AlertEvaluator{
					service.NewSqlServerBlockingEvaluator(tsPool),
					service.NewSqlServerFailedJobsEvaluator(tsPool),
					service.NewSqlServerDiskSpaceEvaluator(tsPool),
					service.NewSqlServerLongRunningQueryEvaluator(tsPool),
					service.NewSqlServerConnectionPressureEvaluator(tsPool),
					service.NewPgReplicationLagEvaluator(tsPool),
					service.NewPgBlockingEvaluator(tsPool),
					service.NewPgBackupFreshnessEvaluator(tsPool),
					service.NewPgDiskSpaceEvaluator(tsPool),
					service.NewPgConnectionSaturationEvaluator(tsPool),
					service.NewPgWalSlotRetentionEvaluator(tsPool),
					service.NewPgBouncerWaitEvaluator(tsPool),
				}

				alertSvc := service.NewAlertService(
					alertRepo,
					maintRepo,
					evaluators,
					sharedNotifier,
				)
				alertH = handlers.NewAlertHandlers(alertSvc, cfg)
			}
		}
		return alertH
	}

	var rulesH *handlers.RulesHandler
	rulesBP := func(w http.ResponseWriter, req *http.Request) {
		if rulesH == nil {
			var pool *pgxpool.Pool
			if metricsSvc != nil {
				pool = metricsSvc.GetTimescaleDBPool()
			}
			var osIngest func(context.Context) bool
			if metricsSvc != nil {
				osIngest = metricsSvc.IsOSMetricsIngestEnabled
			}
			var pgRepo *repository.PgRepository
			if metricsSvc != nil {
				pgRepo = metricsSvc.PgRepo
			}
			rulesH = handlers.NewRulesHandler(pool, cfg, osIngest, pgRepo)
		}
		if rulesH != nil {
			rulesH.BestPractices(w, req)
		} else {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "rule engine not available"})
		}
	}

	// --- Public / open /api branch (no JWT)
	openAPI := r.PathPrefix("/api").Subrouter()
	if loginLimiter != nil {
		loginHF := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			authH.Login(w, req)
		})
		limitedLogin := middleware.LoginRateLimitMiddleware(loginLimiter, loginHF)
		openAPI.Handle("/login", limitedLogin).Methods("POST")
		openAPI.Handle("/auth/login", limitedLogin).Methods("POST")
	}
	openAPI.HandleFunc("/logout", authH.Logout).Methods("POST")
	openAPI.HandleFunc("/auth/logout", authH.Logout).Methods("POST")

	if sec.AuthRequired {
		// Strict: only auth endpoints stay public; config is moved behind JWT below.
	} else {
		registerMonitoringReadRoutes(openAPI, mon, rulesBP)
		registerMonitoringElevatedRoutes(openAPI, sqlserverH, handlers.NewPgExplainAnalyzeHandler(metricsSvc), handlers.PgExplainOptimize, handlers.PgExplainIndexAdvisor(cfg))
		registerAlertReadRoutes(openAPI, getAlertH)
		openAPI.HandleFunc("/os-collector/status", osCollectorH.Status).Methods("GET")
		openAPI.HandleFunc("/os-collector/bundle", osCollectorH.DownloadBundle).Methods("GET")
		openAPI.HandleFunc("/os-collector/ingest", osCollectorH.GetIngestConfig).Methods("GET")
		openAPI.HandleFunc("/os-collector/ingest", osCollectorH.SetIngestConfig).Methods("PUT")
		if mon.ColdQuery != nil {
			openAPI.HandleFunc("/cold-storage/query", mon.ColdQuery.RunQuery).Methods("POST")
			openAPI.HandleFunc("/cold-storage/history", mon.ColdQuery.RunHistoryQuery).Methods("POST")
		}
		// Alert mutations and destructive PG actions require JWT even in legacy dev mode (see legacyAuthed below).
	}

	// --- Any authenticated user (JWT or OIDC)
	authed := r.PathPrefix("/api").Subrouter()
	authed.Use(middleware.RequireAuth(""))
	authed.Use(middleware.CSRFProtect)
	authed.HandleFunc("/auth/me", authH.Me).Methods("GET")

	// OS collector ingest toggle at /api/os-collector/ingest (DBA/Admin when AUTH_REQUIRED=1).
	osCollectorIngestAPI := r.PathPrefix("/api").Subrouter()
	osCollectorIngestAPI.Use(middleware.RequireAuth(""))
	osCollectorIngestAPI.Use(middleware.RequireAnyRole("dba", "admin"))
	osCollectorIngestAPI.Use(middleware.CSRFProtect)
	osCollectorIngestAPI.HandleFunc("/os-collector/ingest", osCollectorH.GetIngestConfig).Methods("GET")
	osCollectorIngestAPI.HandleFunc("/os-collector/ingest", osCollectorH.SetIngestConfig).Methods("PUT")

	if sec.AuthRequired {
		configProtected := r.PathPrefix("/api").Subrouter()
		configProtected.Use(middleware.RequireAuth(""))
		configProtected.Use(middleware.RequireAnyRole("viewer", "dba", "admin"))
		configProtected.HandleFunc("/config", func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			// Lazy-reload: if startup failed to decrypt/load instances, recover on first request.
			if len(cfg.Instances) == 0 && metricsSvc != nil && metricsSvc.RegistryReload != nil {
				metricsSvc.RegistryReload()
			}
			statuses := map[string]string{}
			if metricsSvc != nil {
				statuses = metricsSvc.GetAllInstanceStatuses()
			}
			instances := make([]config.Instance, 0, len(cfg.Instances))
			for _, inst := range cfg.Instances {
				copyInst := inst
				if status, ok := statuses[inst.Name]; ok {
					copyInst.Available = status == "online"
				} else {
					copyInst.Available = true
				}
				instances = append(instances, copyInst)
			}

			intelActive := false
			if irSvc != nil {
				intelActive = irSvc.CheckStatus(req.Context())
			}

			coldCfg := cold.ConfigFromEnv()
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"instances":                    instances,
				"feature_flags":                cfg.FeatureFlags,
				"intelligence_active":          intelActive,
				"cold_storage_enabled":         coldCfg.Enabled,
				"cold_storage_query_available": coldCfg.TrinoURL != "",
				"max_dashboard_range_days":     int(handlers.MaxDashboardTimeRange().Hours() / 24),
			})
		}).Methods("GET")

		readAPI := r.PathPrefix("/api").Subrouter()
		readAPI.Use(middleware.RequireAuth(""))
		readAPI.Use(middleware.RequireAnyRole("viewer", "dba", "admin"))
		registerMonitoringReadRoutes(readAPI, mon, rulesBP)
		registerAlertReadRoutes(readAPI, getAlertH)
		readAPI.HandleFunc("/os-collector/status", osCollectorH.Status).Methods("GET")

		dbaAPI := r.PathPrefix("/api").Subrouter()
		dbaAPI.Use(middleware.RequireAuth(""))
		dbaAPI.Use(middleware.RequireAnyRole("dba", "admin"))
		dbaAPI.Use(middleware.CSRFProtect)
		registerMonitoringElevatedRoutes(dbaAPI, sqlserverH, handlers.NewPgExplainAnalyzeHandler(metricsSvc), handlers.PgExplainOptimize, handlers.PgExplainIndexAdvisor(cfg))
		dbaAPI.HandleFunc("/os-collector/bundle", osCollectorH.DownloadBundle).Methods("GET")
		dbaAPI.HandleFunc("/os-collector/token", osCollectorH.MintOSAgentToken).Methods("POST")
		dbaAPI.HandleFunc("/os-collector/token/revoke", osCollectorH.RevokeOSAgentToken).Methods("POST")
		dbaAPI.HandleFunc("/postgres/dr-policy", pgBackupH.PutDRPolicy).Methods("PUT")
		registerPostgresDBAMutations(dbaAPI, postgresH)
		registerDashboardWidgetRoutes(dbaAPI, dashboardH)
		registerAlertMutationRoutes(dbaAPI, getAlertH)
		if mon.ColdQuery != nil {
			dbaAPI.HandleFunc("/cold-storage/query", mon.ColdQuery.RunQuery).Methods("POST")
			dbaAPI.HandleFunc("/cold-storage/history", mon.ColdQuery.RunHistoryQuery).Methods("POST")
		}
	}

	if !sec.AuthRequired {
		legacyAuthed := r.PathPrefix("/api").Subrouter()
		legacyAuthed.Use(middleware.RequireAuth(""))
		legacyAuthed.Use(middleware.CSRFProtect)
		legacyAuthed.HandleFunc("/os-collector/ingest", osCollectorH.GetIngestConfig).Methods("GET")
		legacyAuthed.HandleFunc("/os-collector/ingest", osCollectorH.SetIngestConfig).Methods("PUT")
		legacyAuthed.HandleFunc("/sqlserver/xevents", sqlserverH.XEvents).Methods("GET")
		registerPostgresDBAMutations(legacyAuthed, postgresH)
		registerDashboardWidgetRoutes(legacyAuthed, dashboardH)
		registerAlertMutationRoutes(legacyAuthed, getAlertH)
	}

	// --- Admin-only
	adminAPI := r.PathPrefix("/api/admin").Subrouter()
	adminAPI.Use(middleware.RequireAuth("admin"))
	adminAPI.Use(middleware.CSRFProtect)
	osMetricsAPI := r.PathPrefix("/api").Subrouter()
	osMetricsAPI.Use(middleware.RequireOSMetricsAuth())
	osMetricsAPI.HandleFunc("/os/metrics", osMetricsH.ReceiveMetrics).Methods("POST")
	adminAPI.HandleFunc("/users", adminH.CreateUser).Methods("POST")
	adminAPI.HandleFunc("/users", adminH.ListUsers).Methods("GET")
	adminAPI.HandleFunc("/users/{id}", adminH.DeleteUser).Methods("DELETE")
	adminAPI.HandleFunc("/users/{id}/role", adminH.UpdateUserRole).Methods("PUT")
	adminAPI.HandleFunc("/servers", adminServersH.AddServer).Methods("POST")
	adminAPI.HandleFunc("/servers", adminServersH.ListServers).Methods("GET")
	adminAPI.HandleFunc("/servers/test-draft", adminServersH.TestServerDraft).Methods("POST")
	adminAPI.HandleFunc("/servers/check-permissions-draft", adminServersH.CheckPermissionsDraft).Methods("POST")
	adminAPI.HandleFunc("/servers/{id}/test", adminServersH.TestServer).Methods("POST")
	adminAPI.HandleFunc("/servers/{id}/check-permissions", adminServersH.CheckPermissions).Methods("POST")
	adminAPI.HandleFunc("/servers/{id}/rotate", adminServersH.RotateServer).Methods("POST")
	adminAPI.HandleFunc("/servers/{id}", adminServersH.GetServer).Methods("GET")
	adminAPI.HandleFunc("/servers/{id}", adminServersH.UpdateServer).Methods("PUT")
	adminAPI.HandleFunc("/servers/{id}", adminServersH.PatchServer).Methods("PATCH")
	adminAPI.HandleFunc("/servers/{id}", adminServersH.DeleteServer).Methods("DELETE")
	adminAPI.HandleFunc("/os-collector/bundle", osCollectorH.DownloadBundle).Methods("GET")
	adminAPI.HandleFunc("/os-collector/token", osCollectorH.MintOSAgentToken).Methods("POST")
	adminAPI.HandleFunc("/os-collector/token/revoke", osCollectorH.RevokeOSAgentToken).Methods("POST")
	adminAPI.HandleFunc("/os-collector/ingest", osCollectorH.GetIngestConfig).Methods("GET")
	adminAPI.HandleFunc("/os-collector/ingest", osCollectorH.SetIngestConfig).Methods("PUT")
	adminAPI.HandleFunc("/widgets/{id}", widgetAdminH.UpdateWidget).Methods("PUT")
	adminAPI.HandleFunc("/widgets/{id}/restore", widgetAdminH.RestoreWidget).Methods("POST")
	adminAPI.HandleFunc("/widgets/{id}", widgetAdminH.GetWidget).Methods("GET")
	adminAPI.HandleFunc("/widgets", widgetAdminH.ListWidgets).Methods("GET")
	adminAPI.HandleFunc("/collector-configs", adminCollectorH.ListConfigs).Methods("GET")
	adminAPI.HandleFunc("/collector-configs/{id}", adminCollectorH.UpdateConfig).Methods("PUT")
	adminAPI.HandleFunc("/diagnostics/sqlserver", adminSqlServerDiagH.GetSqlServerDiagnostics).Methods("GET")
	adminAPI.HandleFunc("/diagnostics/sqlserver/{instance}", adminSqlServerDiagH.GetSqlServerDiagnostics).Methods("GET")
	if adminNotifH != nil {
		adminAPI.HandleFunc("/notifications/config", adminNotifH.GetConfig).Methods("GET")
		adminAPI.HandleFunc("/notifications/config", adminNotifH.UpdateConfig).Methods("PUT")
		adminAPI.HandleFunc("/notifications/test", adminNotifH.TestNotification).Methods("POST")
	}
}
