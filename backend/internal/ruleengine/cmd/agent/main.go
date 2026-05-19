// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Rule engine agent main entry point.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package main

import (
	"log/slog"
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/rsharma155/sql_optima/internal/ruleengine/collectors"
	"github.com/rsharma155/sql_optima/internal/ruleengine/engine"
	"github.com/rsharma155/sql_optima/internal/ruleengine/postgres"
)

type Config struct {
	PostgresConnStr  string
	SQLServerConnStr string
	PgCollectorStr   string
	ServerID         uuid.UUID
	InstanceType     string
	WorkerCount      int
	PollInterval     time.Duration
}

func main() {
	cfg := parseFlags()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	slog.Info("[Agent] Starting Rule Engine Agent for server_id=", "val", cfg.ServerID)

	pgClient, err := postgres.NewPGClient(ctx, cfg.PostgresConnStr)
	if err != nil {
		slog.Error("[Agent] Failed to create PG client", "err", err)
	os.Exit(1)
	}
	defer pgClient.Close()
	slog.Info("[Agent] PostgreSQL client connected")

	if cfg.InstanceType == "sqlserver" && cfg.SQLServerConnStr != "" {
		sqlCol, err := collectors.NewSQLServerCollector(cfg.SQLServerConnStr)
		if err != nil {
			slog.Error("[Agent] Failed to create SQL Server collector", "err", err)
	os.Exit(1)
		}
		defer sqlCol.Close()
		slog.Info("[Agent] SQL Server collector connected")
	} else if cfg.InstanceType == "postgres" && cfg.PgCollectorStr != "" {
		pgCol, err := collectors.NewPostgresCollector(cfg.PgCollectorStr)
		if err != nil {
			slog.Error("[Agent] Failed to create PostgreSQL collector", "err", err)
	os.Exit(1)
		}
		defer pgCol.Close()
		slog.Info("[Agent] PostgreSQL collector connected")
	} else {
		slog.Error("[Agent] Invalid configuration: instance_type=", "val", cfg.InstanceType)
	os.Exit(1)
	}

	runner := engine.NewRunner(pgClient, cfg.WorkerCount)

	if cfg.InstanceType == "sqlserver" {
		sqlCol, _ := collectors.NewSQLServerCollector(cfg.SQLServerConnStr)
		runner.SetSQLServerCollector(sqlCol)
	} else {
		pgCol, _ := collectors.NewPostgresCollector(cfg.PgCollectorStr)
		runner.SetPostgresCollector(pgCol)
	}

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	ticker := time.NewTicker(cfg.PollInterval)
	defer ticker.Stop()

	run := func() {
		err := runner.Start(ctx, cfg.ServerID, cfg.InstanceType)
		if err != nil {
			slog.Error("[Agent] Run failed", "err", err)
		}
	}

	slog.Info("[Agent] Starting initial run...")
	run()

	slog.Info("[Agent] Polling every", "val", cfg.PollInterval)

	for {
		select {
		case <-ticker.C:
			slog.Info("[Agent] Starting scheduled run...")
			run()
		case <-signalChan:
			slog.Info("[Agent] Shutting down...")
			runner.Stop()
			return
		}
	}
}

func parseFlags() *Config {
	cfg := &Config{
		WorkerCount:  5,
		PollInterval: 5 * time.Minute,
	}

	var serverIDStr string
	flag.StringVar(&cfg.PostgresConnStr, "postgres", "", "PostgreSQL connection string")
	flag.StringVar(&cfg.SQLServerConnStr, "sqlserver", "", "SQL Server connection string")
	flag.StringVar(&cfg.PgCollectorStr, "pg-collector", "", "PostgreSQL target connection string")
	flag.StringVar(&serverIDStr, "server-id", "", "Server ID (UUID) for rule engine")
	flag.StringVar(&cfg.InstanceType, "instance-type", "sqlserver", "Instance type: sqlserver or postgres")
	flag.IntVar(&cfg.WorkerCount, "workers", 5, "Number of worker goroutines")
	flag.DurationVar(&cfg.PollInterval, "interval", 5*time.Minute, "Polling interval")

	flag.Parse()

	if cfg.PostgresConnStr == "" {
		fmt.Println("-postgres flag is required")
		os.Exit(1)
	}

	if cfg.InstanceType == "" {
		fmt.Println("-instance-type flag is required")
		os.Exit(1)
	}

	if serverIDStr != "" {
		id, err := uuid.Parse(serverIDStr)
		if err != nil {
			fmt.Printf("Invalid server-id (must be UUID): %v\n", err)
			os.Exit(1)
		}
		cfg.ServerID = id
	}

	return cfg
}
