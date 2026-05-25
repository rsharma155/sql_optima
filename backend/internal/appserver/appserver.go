// Package appserver wires HTTP server, background jobs, and observability (shared by cmd/server and cmd/api).
// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Wires HTTP server, background jobs, and observability (shared by cmd/server and cmd/api). Initializes config, repositories, TimescaleDB, telemetry, and routes.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package appserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	"github.com/rsharma155/sql_optima/internal/api"
	"github.com/rsharma155/sql_optima/internal/api/handlers"
	"github.com/rsharma155/sql_optima/internal/collectors/application"
	"github.com/rsharma155/sql_optima/internal/collectors/domain/ruleengine"
	"github.com/rsharma155/sql_optima/internal/collectors/domain/scheduler"
	"github.com/rsharma155/sql_optima/internal/collectors/infrastructure/postgres"
	"github.com/rsharma155/sql_optima/internal/collectors/infrastructure/sqlserver"
	"github.com/rsharma155/sql_optima/internal/collectors/infrastructure/timescaledb"
	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/domain/servers"
	"github.com/rsharma155/sql_optima/internal/middleware"
	"github.com/rsharma155/sql_optima/internal/queue"
	"github.com/rsharma155/sql_optima/internal/repository"
	"github.com/rsharma155/sql_optima/internal/security"
	"github.com/rsharma155/sql_optima/internal/service"
	"github.com/rsharma155/sql_optima/internal/storage/cold"
	"github.com/rsharma155/sql_optima/internal/storage/hot"
	"github.com/rsharma155/sql_optima/internal/telemetry"
)

var (
	errorLog  *log.Logger
	errorFile *os.File
)

func initErrorLogger() {
	logDir := "logs"
	if err := os.MkdirAll(logDir, 0755); err != nil {
		slog.Error("Failed to create log directory", "err", err)
		os.Exit(1)
	}
	errorLogPath := filepath.Join(logDir, "error.log")

	// Basic log rotation: if file exists and is > 10MB, truncate it.
	if info, err := os.Stat(errorLogPath); err == nil {
		if info.Size() > 10*1024*1024 {
			_ = os.Truncate(errorLogPath, 0)
		}
	}

	var err error
	errorFile, err = os.OpenFile(errorLogPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		slog.Error("Failed to open error log file", "err", err)
		os.Exit(1)
	}
	// Write errors to both the file and stderr so they appear in `docker compose logs`.
	multiW := io.MultiWriter(os.Stderr, errorFile)
	errorLog = log.New(multiW, "[ERROR] ", log.Ldate|log.Ltime|log.Lshortfile)
	slog.Info("error log initialized", "path", errorLogPath)
}

func parseEnvInt(key string, defaultVal int) int {
	s := strings.TrimSpace(os.Getenv(key))
	if s == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return defaultVal
	}
	return n
}

// Main starts the SQL Optima API and static UI.
func Main() {
	_ = godotenv.Load()
	initErrorLogger()
	defer errorFile.Close()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	if err := config.MergeViperConfigs(); err != nil {
		slog.Info("[config] viper merge", "err", err)
	}

	configPath, frontendDir := config.ResolveDataPaths()

	sec := config.LoadSecurity()

	jwtSecret, err := config.ResolveJWTSecret(configPath)
	if err != nil {
		slog.Error("JWT secret initialization failed", "err", err)
	os.Exit(1)
	}
	if strings.TrimSpace(os.Getenv("JWT_SECRET")) == "" {
		slog.Info("[auth] JWT_SECRET not set; using persisted local secret from data/ for this environment")
	}
	middleware.SetJWTSecret(jwtSecret)

	if sec.AuthMode == "oidc" && sec.OIDCIssuerURL != "" && sec.OIDCAudience != "" {
		octx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		if _, err := middleware.InitOIDC(octx, sec.OIDCIssuerURL, sec.OIDCAudience); err != nil {
			cancel()
			slog.Error("OIDC init failed", "err", err)
	os.Exit(1)
		}
		cancel()
		// Avoid logging issuer URL (often contains internal hostnames).
		slog.Info("[auth] OIDC verifier enabled")
	}

	tctx, tcancel := context.WithTimeout(context.Background(), 10*time.Second)
	tracerShutdown, err := telemetry.InitTracer(tctx, "sql-optima")
	tcancel()
	if err != nil {
		slog.Info("[telemetry] OpenTelemetry init", "err", err)
	}
	defer func() {
		_ = tracerShutdown(context.Background())
	}()

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		slog.Error("Fatal Error loading", "target", configPath, "err", err)
	os.Exit(1)
	}

	var tsHotStorage *hot.HotStorage
	var usingEnvTimescale bool
	var kms servers.KeyManagementService
	var usingLocalKMS bool
	tsHotStorage, usingEnvTimescale, err = hot.ConnectMetricsTimescale(configPath, jwtSecret)
	if err != nil {
		errMsg := fmt.Sprintf("TimescaleDB (env fallback): %v", err)
		slog.Warn("[WARNING]", "err", errMsg)
		errorLog.Print(errMsg)
		tsHotStorage = nil
		usingEnvTimescale = false
	}

	if tsHotStorage != nil {
		kms, usingLocalKMS = config.InitServerRegistryKMS(jwtSecret)

		if usingLocalKMS {
			slog.Info("[kms] using local envelope key derived from JWT_SECRET (set VAULT_ADDR for Vault Transit in production)")
		}
	}

	// Without Timescale there is no server registry; do not use config.yaml instances until DB is configured.
	if tsHotStorage == nil {
		cfg.Instances = nil
	} else if kms != nil {
		if loaded, lerr := repository.LoadInstancesFromServerRegistry(context.Background(), tsHotStorage.Pool(), kms, security.NewEnvelopeSecretBox()); lerr == nil && len(loaded) > 0 {
			cfg.Instances = loaded
			slog.Info("[config] loaded %d instance(s) from server registry", "val", len(cfg.Instances))
		} else if !usingEnvTimescale && !config.DeploymentIsDocker() {
			cfg.Instances = nil
			slog.Warn("[config] no active servers in registry; config.yaml instances ignored (dedicated UI mode — use onboarding or Admin to register targets)")
		}
	}

	slog.Info("Booting Environment: Loaded %d Instances...", "val", len(cfg.Instances))

	pgRepo := repository.NewPgRepository(context.Background(), cfg)
	msRepo := repository.NewSqlServerRepository(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start background health checks to ensure offline instances are retried
	msRepo.StartBackgroundHealthCheck(ctx)
	pgRepo.StartBackgroundHealthCheck(ctx)

	// Wire LocalPool for staging access
	if tsHotStorage != nil {
		pgRepo.LocalPool = tsHotStorage.Pool()
		msRepo.LocalPool = tsHotStorage.Pool()
	}

	metricsSvc := service.NewMetricsService(pgRepo, msRepo, cfg, tsHotStorage)
	metricsSvc.ServerKMS = kms
	if tsHotStorage != nil {
		metricsSvc.BindTimescaleRepos(tsHotStorage.Pool())
	}

	if sec.AuthRequired && sec.AuthMode == "local" && metricsSvc.UserRepo == nil {
		if metricsSvc.IsTimescaleConnected() {
			slog.Error("AUTH_REQUIRED with AUTH_MODE=local requires Timescale user tables (optima_users). Check schema or use AUTH_MODE=oidc.")
	os.Exit(1)
		}
		slog.Info("[auth] AUTH_REQUIRED with local mode: Timescale not connected — use /setup to add the metrics DB first; login stays unavailable until the")
	}

	// 1. Start the new Pulse Service (Handles Tier 1, 2, 3)
	// This replaces most ad-hoc DMV collectors started below.
	if metricsSvc.PulseSvc != nil {
		metricsSvc.PulseSvc.Start(ctx)
	}

	// 2. Start specialized and Tier 4 collectors
	// Note: Sessions, Activity, and Basic Perf are now handled by PulseService.
	// We keep collectors that handle specialized logic (e.g., Log Parsing, Enterprise-specific deltas)

	// Always start background collector; it handles table-driven frequency checks for Tier 4
	go metricsSvc.StartBackgroundCollector(ctx)

	redisAddr := strings.TrimSpace(os.Getenv("REDIS_ADDR"))
	var asynqSch *asynq.Scheduler
	if redisAddr != "" {
		asynqSch, err = queue.StartScheduler(redisAddr, metricsSvc)
		if err != nil {
			slog.Error("asynq scheduler", "err", err)
	os.Exit(1)
		}
		srv, mux := queue.NewServerWithMux(redisAddr, metricsSvc)
		go func() {
			if err := srv.Run(mux); err != nil {
				slog.Info("[asynq] server", "err", err)
			}
		}()
		slog.Info("[asynq] Redis-backed collector queue enabled (tasks can be offloaded); StartBackgroundCollector will coordinate ticks")
	}

	// Specialized Collectors (to be progressively merged into Pulses)
	go metricsSvc.StartPerformanceDebtCollector(ctx)
	go metricsSvc.StartQueryAnalysisCollector(ctx)
	go metricsSvc.StartWatchedQueryCollector(ctx)
	go metricsSvc.StartSqlServerStorageHistoryCollector(ctx)
	go metricsSvc.StartPostgresNewDashboardsCollectors(ctx)
	go metricsSvc.StartPostgresStorageIndexHealthCollector(ctx)
	go metricsSvc.StartPostgresQueryStatsCollector(ctx)
	go metricsSvc.StartPgLocksBlockingCollector(ctx)
	go metricsSvc.StartQueryStoreCollector(ctx)
	go metricsSvc.StartPostgresEnterpriseCollector(ctx)
	go metricsSvc.StartSqlServerHealthCollector(ctx)
	go metricsSvc.StartSqlServerHAReplicationCollector(ctx)
	go metricsSvc.StartSqlServerBackupCollector(ctx)
	go metricsSvc.StartSqlServerDatabaseCatalogCollector(ctx)
	go metricsSvc.StartPerfCountersCollector(ctx)
	go metricsSvc.StartSessionCollector(ctx)
	go metricsSvc.StartIndexCollector(ctx)
	go metricsSvc.StartXEFileTargetWorker(ctx)

	// SQL Server Wait Stats V2 & Shared Collectors
	go metricsSvc.StartSharedWaitStatsCollector(ctx)
	go metricsSvc.StartActiveWaitSessionsCollector(ctx)
	go metricsSvc.StartFileIOLatencyCollector(ctx)
	go metricsSvc.StartMemoryIntelligenceCollector(ctx)

	// Legacy Collectors DISABLED (Now handled by PulseService or Shared Collectors)

	// go metricsSvc.StartPgLocksBlockingCollector(ctx)
	// go metricsSvc.StartSQLServerHealthKPICollector(ctx)
	// go metricsSvc.StartEnterpriseCollector(ctx)
	// go metricsSvc.StartEnterpriseMetricsCollector(ctx)

	if tsHotStorage != nil {
		go startQueryV2Collector(ctx, tsHotStorage.Pool(), cfg, msRepo, pgRepo)
		go startColdStorageExporter(ctx, tsHotStorage.Pool())
	}

	// ── Alert evaluation loop ──────────────────────────────────
	if tsPool := metricsSvc.GetTimescaleDBPool(); tsPool != nil {
		alertRepo := repository.NewTimescaleAlertStore(tsPool)
		maintRepo := repository.NewAlertMaintenanceStore(tsPool)
		evaluators := []service.AlertEvaluator{
			service.NewSqlServerBlockingEvaluator(tsPool),
			service.NewSqlServerFailedJobsEvaluator(tsPool),
			service.NewSqlServerDiskSpaceEvaluator(tsPool),
			service.NewPgReplicationLagEvaluator(tsPool),
			service.NewPgBlockingEvaluator(tsPool),
			service.NewPgBackupFreshnessEvaluator(tsPool),
			service.NewPgDiskSpaceEvaluator(tsPool),
		}
		alertSvc := service.NewAlertService(
			alertRepo,
			maintRepo,
			evaluators,
			service.NewNotifier(service.NotifierConfig{
				WebhookURL:      os.Getenv("ALERT_WEBHOOK_URL"),
				SlackWebhookURL: os.Getenv("SLACK_WEBHOOK_URL"),
			}, slog.Default()),
		)
		alertInterval := metricsSvc.GetCollectorInterval(ctx, "Alert Evaluation Loop", 60*time.Second)
		go service.StartAlertEvaluationLoop(ctx, tsPool, cfg, alertSvc, alertInterval)
	}

	r := mux.NewRouter()
	r.Use(telemetry.PrometheusMiddleware) // registered on the router so route templates are available for path labels
	r.Handle("/metrics", telemetry.MetricsHandler()).Methods("GET")

	loginLimit := middleware.NewLoginRateLimiter(parseEnvInt("LOGIN_RATE_LIMIT_PER_MIN", 20))
	api.RegisterHealthRoutes(r, cfg, metricsSvc, loginLimit, sec)

	disablePublicSetup := strings.TrimSpace(os.Getenv("DISABLE_PUBLIC_SETUP")) == "1"
	if !disablePublicSetup {
		slog.Warn("DISABLE_PUBLIC_SETUP is not set to 1 — /api/setup/* endpoints are publicly reachable. Set DISABLE_PUBLIC_SETUP=1 in production after first-run bootstrap is complete.")
	}
	if !sec.AuthRequired {
		slog.Warn("AUTH_REQUIRED is not enabled — all monitoring read API endpoints are publicly accessible without a token. Set AUTH_REQUIRED=1 in production environments.")
	}
	allowTSReconf := strings.TrimSpace(os.Getenv("ALLOW_TIMESCALE_RECONFIG")) == "1"
	reloadFromRegistry := func() {
		ctx := context.Background()
		if metricsSvc.GetTimescaleDBPool() == nil || metricsSvc.ServerKMS == nil {
			return
		}
		loaded, lerr := repository.LoadInstancesFromServerRegistry(ctx, metricsSvc.GetTimescaleDBPool(), metricsSvc.ServerKMS, security.NewEnvelopeSecretBox())
		if lerr != nil {
			slog.Error("[config] registry reload failed", "err", lerr)
			return
		}
		if loaded == nil {
			loaded = []config.Instance{}
		}
		cfg.Instances = loaded
		metricsSvc.ReplaceInstanceRepositories(repository.NewPgRepository(context.Background(), cfg), repository.NewSqlServerRepository(cfg))
		slog.Info("[config] registry reload: %d instance(s)", "val", len(cfg.Instances))

		// Immediately trigger a one-time collection for new instances across all workers
		go func() {
			tctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			// Core metrics (CPU, Memory, Historical Log)
			metricsSvc.RunLiveCollectorOnce(tctx)
			metricsSvc.RunHistoricalCollectorOnce(tctx)
			// Enterprise and Query Store metrics
			metricsSvc.TriggerBackgroundCollectorsOnce()
		}()
	}
	metricsSvc.RegistryReload = reloadFromRegistry
	api.RegisterSetupRoutes(r, middleware.NewSetupRateLimiter(parseEnvInt("SETUP_RATE_LIMIT_PER_MIN", 12), time.Minute), &handlers.SetupDeps{
		Metrics:              metricsSvc,
		Cfg:                  cfg,
		ConfigPath:           configPath,
		JWTSecret:            jwtSecret,
		ReloadFromRegistry:   reloadFromRegistry,
		VaultAddrSet:         strings.TrimSpace(os.Getenv("VAULT_ADDR")) != "",
		UsingLocalKMS:        usingLocalKMS,
		DisablePublicSetup:   disablePublicSetup,
		AllowTimescaleReconf: allowTSReconf,
	})

	noCacheFS := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
			w.Header().Set("Pragma", "no-cache")
			w.Header().Set("Expires", "0")
			next.ServeHTTP(w, req)
		})
	}
	r.PathPrefix("/assets/css/").Handler(noCacheFS(http.StripPrefix("/assets/css/", http.FileServer(http.Dir(filepath.Join(frontendDir, "assets", "css"))))))
	r.PathPrefix("/js/").Handler(noCacheFS(http.StripPrefix("/js/", http.FileServer(http.Dir(filepath.Join(frontendDir, "js"))))))
	r.PathPrefix("/pages/").Handler(noCacheFS(http.StripPrefix("/pages/", http.FileServer(http.Dir(filepath.Join(frontendDir, "pages"))))))

	r.PathPrefix("/").HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// If an /api/* path reaches the SPA fallback, it means the API route wasn't registered
		// in this running server binary. Return a JSON 404 to avoid confusing "200 HTML" responses.
		if strings.HasPrefix(req.URL.Path, "/api/") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "api route not found (server may be outdated; restart after rebuild)"})
			return
		}
		http.ServeFile(w, req, filepath.Join(frontendDir, "index.html"))
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = config.DefaultPort
	}
	if !strings.HasPrefix(port, ":") {
		port = ":" + port
	}

	inner := middleware.RequestIDMiddleware(
		middleware.AccessLogMiddleware(logger,
			middleware.CORSMiddleware(
				middleware.SecurityHeadersMiddleware(r))))

	httpHandler := telemetry.WrapOTelHTTP(inner)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Bind the listener first so we know the port is available before printing the banner.
	ln, err := net.Listen("tcp", port)
	if err != nil {
		slog.Error("[FATAL] Failed to bind port", "target", port, "err", err)
	os.Exit(1)
	}

	server := &http.Server{
		Handler:      httpHandler,
		ReadTimeout:  2 * time.Minute,
		// Best-practices evaluation can run many detection queries sequentially (up to ~6 min).
		WriteTimeout: 8 * time.Minute,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		if err := server.Serve(ln); err != nil && err != http.ErrServerClosed {
			errMsg := fmt.Sprintf("Server failed: %v", err)
			slog.Info("[FATAL]", "err", errMsg)
			errorLog.Print(errMsg)
		}
	}()

	// Resolve the actual address (handles ":0" or ":8080" → "localhost:8080").
	host := "localhost"
	if h := strings.TrimSpace(os.Getenv("HOSTNAME")); h != "" {
		host = h
	}
	addr := ln.Addr().String()
	if strings.HasPrefix(addr, "[::]") || strings.HasPrefix(addr, "0.0.0.0") {
		addr = host + ":" + strings.Split(addr, ":")[len(strings.Split(addr, ":"))-1]
	}

	// Print startup banner to both stdout and stderr so it appears in
	// docker compose logs regardless of stream routing.
	banner := fmt.Sprintf(`
======================================================
  SQL Optima is up and running!

  Local:   http://localhost%s
  Network: http://%s

  Open the URL above in your browser to get started.
  Press Ctrl+C to stop the server.
======================================================
`, port, addr)
	_, _ = fmt.Fprint(os.Stdout, banner)

	sig := <-sigChan
	slog.Info("Received signal: %v, shutting down gracefully...", "val", sig)

	cancel()
	if asynqSch != nil {
		asynqSch.Shutdown()
	}

	if tsHotStorage != nil {
		tsHotStorage.Close()
		slog.Info("[Info] TimescaleDB connection closed")
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Info("Server forced to shutdown", "err", err)
	}

	slog.Info("Server exited")
}

func startQueryV2Collector(ctx context.Context, pool *pgxpool.Pool, cfg *config.Config, msRepo *repository.SqlServerRepository, pgRepo *repository.PgRepository) {
	// Query V2 pipeline is always enabled (hash-based query deltas for SQL Server and PostgreSQL).
	slog.Info("[collector-v2] starting lightweight query metrics pipeline")

	// Repositories
	configRepo := repository.NewCollectorConfigRepository(pool)
	ruleRepo := timescaledb.NewRuleRepository(pool)
	writer := timescaledb.NewTimescaleWriter(pool)

	// Ensure query-V2 jobs exist in optima_collector_configs so existing deployments
	// that pre-date the schema seed addition still pick them up at runtime.
	for _, seed := range []struct {
		name   string
		module string
		freqS  int
	}{
		{"sqlserver_query_snapshot", "SQLSERVER", 60},
		{"sqlserver_session_enrichment", "SQLSERVER", 30},
		{"pg_queries_v2", "Postgres", 60},
	} {
		if err := configRepo.InsertDefault(ctx, seed.name, seed.module, seed.freqS); err != nil {
			slog.Info("[collector-v2] seed config", "target", seed.name, "err", err)
		}
	}

	// Fetch rules
	mssqlRules, _ := ruleRepo.GetMSSQLIgnoreRules(ctx)
	pgRules, _ := ruleRepo.GetPGIgnoreRules(ctx)
	filter := ruleengine.NewFilterService(mssqlRules, pgRules)

	// Scheduler Adapter
	schedulerRepo := &schedulerRepoAdapter{repo: configRepo}
	jobScheduler := scheduler.NewJobSchedulerService(schedulerRepo)

	// One app per instance type (or one app that handles all? Let's use one app that coordinates)
	// We need a way to get DB connections for each instance.
	// For simplicity, we'll create the app and then in each cycle, iterate over instances.

	// But wait, our CollectorApp currently takes single repos.
	// Let's refactor CollectorApp to take a Factory or similar, or just handle multiple instances.

	// Actually, let's keep it simple: Create a map of serverID -> App
	mssqlApps := make(map[uuid.UUID]*application.CollectorApp)
	pgApps := make(map[uuid.UUID]*application.CollectorApp)

	ticker := time.NewTicker(5 * time.Second)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Ensure all current instances from cfg have a collector app
				for _, inst := range cfg.Instances {
					if inst.Type == "sqlserver" {
						if _, ok := mssqlApps[inst.ServerID]; !ok {
							db, ok := msRepo.GetConn(inst.Name)
							if ok {
								repo := sqlserver.NewSQLServerSnapshotRepository(db)
								mssqlApps[inst.ServerID] = application.NewCollectorApp(jobScheduler, repo, nil, writer, filter)
								slog.Info("[collector-v2] Dynamic start: SQL Server collector for", "val", inst.Name)
							}
						}
					} else if inst.Type == "postgres" {
						if _, ok := pgApps[inst.ServerID]; !ok {
							db, ok := pgRepo.GetConn(inst.Name)
							if ok {
								repo := postgres.NewPGSnapshotRepository(db)
								pgApps[inst.ServerID] = application.NewCollectorApp(jobScheduler, nil, repo, writer, filter)
								slog.Info("[collector-v2] Dynamic start: Postgres collector for", "val", inst.Name)
							}
						}
					}
				}

				for sid, app := range mssqlApps {
					// Drop collector if instance is no longer online in repository
					online := false
					for _, inst := range cfg.Instances {
						if inst.ServerID == sid {
							if msRepo.GetInstanceStatus(inst.Name) == "online" {
								online = true
							}
							break
						}
					}
					if !online {
						delete(mssqlApps, sid)
						slog.Info("[collector-v2] Dynamic stop: SQL Server collector offline", "target", sid)
						continue
					}
					app.RunCycle(ctx, sid)
				}
				for sid, app := range pgApps {
					// Drop collector if instance is no longer online in repository
					online := false
					for _, inst := range cfg.Instances {
						if inst.ServerID == sid {
							if pgRepo.GetInstanceStatus(inst.Name) == "online" {
								online = true
							}
							break
						}
					}
					if !online {
						delete(pgApps, sid)
						slog.Info("[collector-v2] Dynamic stop: Postgres collector offline", "target", sid)
						continue
					}
					app.RunCycle(ctx, sid)
				}
			}
		}
	}()
}

type schedulerRepoAdapter struct {
	repo *repository.CollectorConfigRepository
}

func (a *schedulerRepoAdapter) GetActiveConfigs(ctx context.Context) ([]scheduler.Config, error) {
	cfgs, err := a.repo.GetActiveConfigs(ctx)
	if err != nil {
		return nil, err
	}
	var res []scheduler.Config
	for _, c := range cfgs {
		res = append(res, scheduler.Config{
			Name:             c.CollectorName,
			FrequencySeconds: c.FrequencySeconds,
		})
	}
	return res, nil
}

func startColdStorageExporter(ctx context.Context, pool *pgxpool.Pool) {
	cfg := cold.ConfigFromEnv()
	if !cfg.Enabled {
		slog.Info("[ColdExporter] disabled via COLD_STORAGE_ENABLED=false")
		return
	}

	uploader, err := cold.NewS3Uploader(ctx, cfg)
	if err != nil {
		slog.Error("[ColdExporter] failed to initialize uploader", "err", err)
		return
	}

	if err := uploader.EnsureBucket(ctx); err != nil {
		slog.Warn("[ColdExporter] could not ensure bucket", "err", err)
	}

	watermark := cold.NewWatermarkStore(pool)
	exporter := cold.NewExporter(pool, uploader, watermark, cfg)

	// Register core tables
	cold.RegisterCoreTables(exporter)

	slog.Info("[ColdExporter] initialized and scheduled (2am UTC daily)")

	// Schedule nightly at 02:00 UTC
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		now := time.Now().UTC()
		if now.Hour() == 2 {
			if err := exporter.Run(ctx); err != nil {
				slog.Error("[ColdExporter] run failed", "err", err)
			}
			// Sleep for an hour to avoid multiple runs within the same hour window
			time.Sleep(1 * time.Hour)
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// check again
		}
	}
}
