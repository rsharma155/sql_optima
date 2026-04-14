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
	"log"
	"log/slog"
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
	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/middleware"
	"github.com/rsharma155/sql_optima/internal/queue"
	"github.com/rsharma155/sql_optima/internal/repository"
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
	errorLog = log.New(errorFile, "[ERROR] ", log.Ldate|log.Ltime|log.Lshortfile)
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

	sec := config.LoadSecurity()

	jwtSecret := []byte(os.Getenv("JWT_SECRET"))
	if len(jwtSecret) == 0 {
		jwtSecret = []byte("your-256-bit-secret-key-change-this-in-production")
		log.Printf("[WARNING] Using default JWT secret - set JWT_SECRET environment variable in production")
	}
	if sec.AuthRequired && len(jwtSecret) < 32 {
		log.Fatal("AUTH_REQUIRED=1 requires JWT_SECRET with at least 32 characters")
	}
	middleware.SetJWTSecret(jwtSecret)

	if sec.AuthMode == "oidc" && sec.OIDCIssuerURL != "" && sec.OIDCAudience != "" {
		octx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		if _, err := middleware.InitOIDC(octx, sec.OIDCIssuerURL, sec.OIDCAudience); err != nil {
			cancel()
			log.Fatalf("OIDC init failed: %v", err)
		}
		cancel()
		log.Printf("[auth] OIDC verifier enabled for issuer %s", sec.OIDCIssuerURL)
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

	configPath, queriesPath, frontendDir := config.ResolveDataPaths()
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("Fatal Error loading %s: %v", configPath, err)
	}
	log.Printf("Booting Environment: Loaded %d Instances...", len(cfg.Instances))

	queriesLoaded := true
	if err := config.LoadQueries(queriesPath); err != nil {
		queriesLoaded = false
	}

	pgRepo := repository.NewPgRepository(cfg)
	msRepo := repository.NewMssqlRepository(cfg)

	var tsHotStorage *hot.HotStorage
	tsHotStorage, err = hot.New(nil)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to connect to TimescaleDB: %v. Metrics will not be persisted.", err)
		log.Printf("[WARNING] %s", errMsg)
		errorLog.Print(errMsg)
		tsHotStorage = nil
	} else {
		log.Println("[Info] Connected to TimescaleDB for metrics persistence")
	}

	metricsSvc := service.NewMetricsService(pgRepo, msRepo, cfg, tsHotStorage)

	if sec.AuthRequired && sec.AuthMode == "local" && metricsSvc.UserRepo == nil {
		log.Fatal("AUTH_REQUIRED with AUTH_MODE=local requires TimescaleDB (optima_users). Configure Timescale or use AUTH_MODE=oidc.")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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

	r := mux.NewRouter()
	r.Handle("/metrics", telemetry.MetricsHandler()).Methods("GET")

	loginLimit := middleware.NewLoginRateLimiter(parseEnvInt("LOGIN_RATE_LIMIT_PER_MIN", 20))
	api.RegisterHealthRoutes(r, cfg, metricsSvc, queriesLoaded, loginLimit, sec)

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
			telemetry.PrometheusMiddleware(
				middleware.CORSMiddleware(
					middleware.SecurityHeadersMiddleware(r)))))

	httpHandler := telemetry.WrapOTelHTTP(inner)

	log.Printf("Starting Dual-Engine API & Static Server on http://localhost%s", port)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	server := &http.Server{
		Addr:         port,
		Handler:      httpHandler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errMsg := fmt.Sprintf("Server failed to start: %v", err)
			log.Printf("[FATAL] %s", errMsg)
			errorLog.Print(errMsg)
		}
	}()

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
