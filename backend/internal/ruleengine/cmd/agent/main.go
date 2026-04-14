// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Rule engine agent main entry point.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rsharma155/sql_optima/internal/ruleengine/collectors"
	"github.com/rsharma155/sql_optima/internal/ruleengine/engine"
	"github.com/rsharma155/sql_optima/internal/ruleengine/postgres"
)

type Config struct {
	PostgresConnStr  string
	SQLServerConnStr string
	PgCollectorStr   string
	ServerID         int
	InstanceType     string
	WorkerCount      int
	PollInterval     time.Duration
}

func main() {
	cfg := parseFlags()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	log.Printf("[Agent] Starting Rule Engine Agent for server_id=%d", cfg.ServerID)

	pgClient, err := postgres.NewPGClient(ctx, cfg.PostgresConnStr)
	if err != nil {
		log.Fatalf("[Agent] Failed to create PG client: %v", err)
	}
	defer pgClient.Close()
	log.Printf("[Agent] PostgreSQL client connected")

	if cfg.InstanceType == "sqlserver" && cfg.SQLServerConnStr != "" {
		sqlCol, err := collectors.NewSQLServerCollector(cfg.SQLServerConnStr)
		if err != nil {
			log.Fatalf("[Agent] Failed to create SQL Server collector: %v", err)
		}
		defer sqlCol.Close()
		log.Printf("[Agent] SQL Server collector connected")
	} else if cfg.InstanceType == "postgres" && cfg.PgCollectorStr != "" {
		pgCol, err := collectors.NewPostgresCollector(cfg.PgCollectorStr)
		if err != nil {
			log.Fatalf("[Agent] Failed to create PostgreSQL collector: %v", err)
		}
		defer pgCol.Close()
		log.Printf("[Agent] PostgreSQL collector connected")
	} else {
		log.Fatalf("[Agent] Invalid configuration: instance_type=%s", cfg.InstanceType)
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
			log.Printf("[Agent] Run failed: %v", err)
		}
	}

	log.Printf("[Agent] Starting initial run...")
	run()

	log.Printf("[Agent] Polling every %v", cfg.PollInterval)

	for {
		select {
		case <-ticker.C:
			log.Printf("[Agent] Starting scheduled run...")
			run()
		case <-signalChan:
			log.Printf("[Agent] Shutting down...")
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

	flag.StringVar(&cfg.PostgresConnStr, "postgres", "", "PostgreSQL connection string")
	flag.StringVar(&cfg.SQLServerConnStr, "sqlserver", "", "SQL Server connection string")
	flag.StringVar(&cfg.PgCollectorStr, "pg-collector", "", "PostgreSQL target connection string")
	flag.IntVar(&cfg.ServerID, "server-id", 1, "Server ID for rule engine")
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

	return cfg
}
