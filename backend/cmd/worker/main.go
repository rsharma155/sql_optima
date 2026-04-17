// Worker process: runs Asynq consumers for Redis-scheduled collector tasks.
// Requires REDIS_ADDR. Run the API with the same REDIS_ADDR to enqueue via the embedded scheduler, or use this binary alone if another process schedules tasks.
// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Background worker process that runs Asynq consumers for Redis-scheduled collector tasks. Requires REDIS_ADDR environment variable for task queue management.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/domain/servers"
	"github.com/rsharma155/sql_optima/internal/queue"
	"github.com/rsharma155/sql_optima/internal/repository"
	"github.com/rsharma155/sql_optima/internal/security"
	"github.com/rsharma155/sql_optima/internal/service"
)

func main() {
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		log.Fatal("REDIS_ADDR is required for cmd/worker")
	}

	configPath, _ := config.ResolveDataPaths()
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	jwtSecret, err := config.ResolveJWTSecret(configPath)
	if err != nil {
		log.Fatalf("JWT secret initialization failed: %v", err)
	}
	if os.Getenv("JWT_SECRET") == "" {
		log.Printf("[worker/auth] JWT_SECRET not set; using persisted local secret from data/")
	}

	tsHotStorage, usingEnvTimescale, err := config.ConnectMetricsTimescale(configPath, jwtSecret)
	if err != nil {
		log.Printf("[worker] Timescale: %v", err)
		tsHotStorage = nil
		usingEnvTimescale = false
	}

	var kms servers.KeyManagementService
	if tsHotStorage != nil {
		kms, _ = config.InitServerRegistryKMS(jwtSecret)
	}

	if tsHotStorage == nil {
		cfg.Instances = nil
	} else if kms != nil {
		loaded, lerr := repository.LoadInstancesFromServerRegistry(context.Background(), tsHotStorage.Pool(), kms, security.NewEnvelopeSecretBox())
		if lerr != nil {
			log.Printf("[worker] registry load: %v", lerr)
			cfg.Instances = nil
		} else if len(loaded) > 0 {
			cfg.Instances = loaded
			log.Printf("[worker] loaded %d instance(s) from server registry", len(loaded))
		} else if !usingEnvTimescale && !config.DeploymentIsDocker() {
			cfg.Instances = nil
			log.Println("[worker] no active servers in registry; config.yaml instances ignored (same as API)")
		}
	} else {
		cfg.Instances = nil
		log.Println("[worker] KMS unavailable; instance list cleared")
	}

	pgRepo := repository.NewPgRepository(cfg)
	msRepo := repository.NewMssqlRepository(cfg)
	metricsSvc := service.NewMetricsService(pgRepo, msRepo, cfg, tsHotStorage)
	metricsSvc.ServerKMS = kms

	srv, mux := queue.NewServerWithMux(redisAddr, metricsSvc)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		srv.Shutdown()
	}()

	log.Printf("Asynq worker listening on Redis %s", redisAddr)
	if err := srv.Run(mux); err != nil {
		log.Fatalf("worker: %v", err)
	}
}
