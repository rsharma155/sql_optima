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

	"github.com/gorilla/mux"
	"github.com/hibiken/asynq"

	"github.com/rsharma155/sql_optima/internal/api"
	"github.com/rsharma155/sql_optima/internal/api/handlers"
	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/domain/servers"
	"github.com/rsharma155/sql_optima/internal/middleware"
	"github.com/rsharma155/sql_optima/internal/queue"
	"github.com/rsharma155/sql_optima/internal/repository"
	"github.com/rsharma155/sql_optima/internal/security"
	"github.com/rsharma155/sql_optima/internal/service"
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
		log.Fatalf("Failed to create log directory: %v", err)
	}
	errorLogPath := filepath.Join(logDir, "error.log")
	var err error
	errorFile, err = os.OpenFile(errorLogPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("Failed to open error log file: %v", err)
	}
	// Write errors to both the file and stderr so they appear in `docker compose logs`.
	multiW := io.MultiWriter(os.Stderr, errorFile)
	errorLog = log.New(multiW, "[ERROR] ", log.Ldate|log.Ltime|log.Lshortfile)
	log.Printf("Error log file: %s", errorLogPath)
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
	initErrorLogger()
	defer errorFile.Close()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	if err := config.MergeViperConfigs(); err != nil {
		log.Printf("[config] viper merge: %v", err)
	}

	configPath, frontendDir := config.ResolveDataPaths()

	sec := config.LoadSecurity()

	jwtSecret, err := config.ResolveJWTSecret(configPath)
	if err != nil {
		log.Fatalf("JWT secret initialization failed: %v", err)
	}
	if strings.TrimSpace(os.Getenv("JWT_SECRET")) == "" {
		log.Printf("[auth] JWT_SECRET not set; using persisted local secret from data/ for this environment")
	}
	middleware.SetJWTSecret(jwtSecret)

	if sec.AuthMode == "oidc" && sec.OIDCIssuerURL != "" && sec.OIDCAudience != "" {
		octx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		if _, err := middleware.InitOIDC(octx, sec.OIDCIssuerURL, sec.OIDCAudience); err != nil {
			cancel()
			log.Fatalf("OIDC init failed: %v", err)
		}
		cancel()
		// Avoid logging issuer URL (often contains internal hostnames).
		log.Printf("[auth] OIDC verifier enabled")
	}

	tctx, tcancel := context.WithTimeout(context.Background(), 10*time.Second)
	tracerShutdown, err := telemetry.InitTracer(tctx, "sql-optima")
	tcancel()
	if err != nil {
		log.Printf("[telemetry] OpenTelemetry init: %v", err)
	}
	defer func() {
		_ = tracerShutdown(context.Background())
	}()

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("Fatal Error loading %s: %v", configPath, err)
	}

	var tsHotStorage *hot.HotStorage
	var usingEnvTimescale bool
	tsHotStorage, usingEnvTimescale, err = config.ConnectMetricsTimescale(configPath, jwtSecret)
	if err != nil {
		errMsg := fmt.Sprintf("TimescaleDB (env fallback): %v", err)
		log.Printf("[WARNING] %s", errMsg)
		errorLog.Print(errMsg)
		tsHotStorage = nil
		usingEnvTimescale = false
	}

	var kms servers.KeyManagementService
	usingLocalKMS := false
	if tsHotStorage != nil {
		kms, usingLocalKMS = config.InitServerRegistryKMS(jwtSecret)
		if usingLocalKMS {
			log.Printf("[kms] using local envelope key derived from JWT_SECRET (set VAULT_ADDR for Vault Transit in production)")
		}
	}

	// Without Timescale there is no server registry; do not use config.yaml instances until DB is configured.
	if tsHotStorage == nil {
		cfg.Instances = nil
	} else if kms != nil {
		if loaded, lerr := repository.LoadInstancesFromServerRegistry(context.Background(), tsHotStorage.Pool(), kms, security.NewEnvelopeSecretBox()); lerr == nil && len(loaded) > 0 {
			cfg.Instances = loaded
			log.Printf("[config] loaded %d instance(s) from server registry", len(cfg.Instances))
		} else if !usingEnvTimescale && !config.DeploymentIsDocker() {
			cfg.Instances = nil
			log.Println("[config] no active servers in registry; config.yaml instances ignored (dedicated UI mode — use onboarding or Admin to register targets)")
		}
	}

	log.Printf("Booting Environment: Loaded %d Instances...", len(cfg.Instances))

	pgRepo := repository.NewPgRepository(cfg)
	msRepo := repository.NewMssqlRepository(cfg)

	metricsSvc := service.NewMetricsService(pgRepo, msRepo, cfg, tsHotStorage)
	metricsSvc.ServerKMS = kms

	if sec.AuthRequired && sec.AuthMode == "local" && metricsSvc.UserRepo == nil {
		if metricsSvc.IsTimescaleConnected() {
			log.Fatal("AUTH_REQUIRED with AUTH_MODE=local requires Timescale user tables (optima_users). Check schema or use AUTH_MODE=oidc.")
		}
		log.Printf("[auth] AUTH_REQUIRED with local mode: Timescale not connected — use /setup to add the metrics DB first; login stays unavailable until then")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Postgres locks/blocking incidents are lightweight and useful even when Redis-backed
	// collectors are enabled. Start this adaptive watcher whenever Timescale is connected.
	go metricsSvc.StartPgLocksBlockingCollector(ctx)

	redisAddr := strings.TrimSpace(os.Getenv("REDIS_ADDR"))
	var asynqSch *asynq.Scheduler
	if redisAddr != "" {
		asynqSch, err = queue.StartScheduler(redisAddr)
		if err != nil {
			log.Fatalf("asynq scheduler: %v", err)
		}
		srv, mux := queue.NewServerWithMux(redisAddr, metricsSvc)
		go func() {
			if err := srv.Run(mux); err != nil {
				log.Printf("[asynq] server: %v", err)
			}
		}()
		log.Println("[asynq] Redis-backed collector queue enabled (live/historical); in-process ticker disabled")
	} else {
		go metricsSvc.StartBackgroundCollector(ctx)
	}

	go metricsSvc.StartQueryStoreCollector(ctx)
	go metricsSvc.StartEnterpriseCollector(ctx)
	go metricsSvc.StartEnterpriseMetricsCollector(ctx)
	go metricsSvc.StartPerformanceDebtCollector(ctx)
	go metricsSvc.StartPostgresEnterpriseCollector(ctx)

	// ── Alert evaluation loop ──────────────────────────────────
	if tsPool := metricsSvc.GetTimescaleDBPool(); tsPool != nil {
		alertRepo := repository.NewAlertRepository(tsPool)
		maintRepo := repository.NewAlertMaintenanceRepository(tsPool)
		evaluators := []service.AlertEvaluator{
			service.NewMssqlBlockingEvaluator(tsPool),
			service.NewMssqlFailedJobsEvaluator(tsPool),
			service.NewMssqlDiskSpaceEvaluator(tsPool),
			service.NewPgReplicationLagEvaluator(tsPool),
			service.NewPgBlockingEvaluator(tsPool),
			service.NewPgBackupFreshnessEvaluator(tsPool),
			service.NewPgDiskSpaceEvaluator(tsPool),
		}
		alertSvc := service.NewAlertService(alertRepo, maintRepo, evaluators)
		go service.StartAlertEvaluationLoop(ctx, tsPool, cfg, alertSvc, 60*time.Second)
	}

	r := mux.NewRouter()
	r.Use(telemetry.PrometheusMiddleware) // registered on the router so route templates are available for path labels
	r.Handle("/metrics", telemetry.MetricsHandler()).Methods("GET")

	loginLimit := middleware.NewLoginRateLimiter(parseEnvInt("LOGIN_RATE_LIMIT_PER_MIN", 20))
	api.RegisterHealthRoutes(r, cfg, metricsSvc, loginLimit, sec)

	disablePublicSetup := strings.TrimSpace(os.Getenv("DISABLE_PUBLIC_SETUP")) == "1"
	if !disablePublicSetup {
		log.Printf("[SECURITY WARNING] DISABLE_PUBLIC_SETUP is not set to 1 — /api/setup/* endpoints are publicly reachable. " +
			"Set DISABLE_PUBLIC_SETUP=1 in production after first-run bootstrap is complete.")
	}
	if !sec.AuthRequired {
		log.Printf("[SECURITY WARNING] AUTH_REQUIRED is not enabled — all monitoring read API endpoints are publicly accessible without a token. " +
			"Set AUTH_REQUIRED=1 in production environments.")
	}
	allowTSReconf := strings.TrimSpace(os.Getenv("ALLOW_TIMESCALE_RECONFIG")) == "1"
	reloadFromRegistry := func() {
		ctx := context.Background()
		if metricsSvc.GetTimescaleDBPool() == nil || metricsSvc.ServerKMS == nil {
			return
		}
		loaded, lerr := repository.LoadInstancesFromServerRegistry(ctx, metricsSvc.GetTimescaleDBPool(), metricsSvc.ServerKMS, security.NewEnvelopeSecretBox())
		if lerr != nil {
			log.Printf("[config] registry reload failed: %v", lerr)
			return
		}
		if loaded == nil {
			loaded = []config.Instance{}
		}
		cfg.Instances = loaded
		metricsSvc.ReplaceInstanceRepositories(repository.NewPgRepository(cfg), repository.NewMssqlRepository(cfg))
		log.Printf("[config] registry reload: %d instance(s)", len(cfg.Instances))
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

	r.PathPrefix("/assets/css/").Handler(http.StripPrefix("/assets/css/", http.FileServer(http.Dir(filepath.Join(frontendDir, "assets", "css")))))
	r.PathPrefix("/js/").Handler(http.StripPrefix("/js/", http.FileServer(http.Dir(filepath.Join(frontendDir, "js")))))
	r.PathPrefix("/pages/").Handler(http.StripPrefix("/pages/", http.FileServer(http.Dir(filepath.Join(frontendDir, "pages")))))

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
		log.Fatalf("[FATAL] Failed to bind port %s: %v", port, err)
	}

	server := &http.Server{
		Handler:      httpHandler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		if err := server.Serve(ln); err != nil && err != http.ErrServerClosed {
			errMsg := fmt.Sprintf("Server failed: %v", err)
			log.Printf("[FATAL] %s", errMsg)
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
	fmt.Fprint(os.Stdout, banner)
	fmt.Fprint(os.Stderr, banner)

	sig := <-sigChan
	log.Printf("Received signal: %v, shutting down gracefully...", sig)

	cancel()
	if asynqSch != nil {
		asynqSch.Shutdown()
	}

	if tsHotStorage != nil {
		tsHotStorage.Close()
		log.Println("[Info] TimescaleDB connection closed")
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited")
}
