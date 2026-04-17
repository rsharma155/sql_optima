// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Background goroutine that periodically runs alert evaluation
//
//	for every configured server instance.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package service

import (
	"context"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/domain/alerts"
)

// advisoryLockID is a fixed int64 used with pg_try_advisory_xact_lock to
// ensure only one API process evaluates alerts in a given tick.
// Chosen arbitrarily; collisions with application-level advisory locks are
// extremely unlikely given the key space.
const advisoryLockID int64 = 0x53514C5F414C5254 // "SQL_ALRT" as hex

// engineForInstanceType maps config.Instance.Type to an alerts.Engine.
func engineForInstanceType(typ string) (alerts.Engine, bool) {
	switch typ {
	case "sqlserver":
		return alerts.EngineSQLServer, true
	case "postgres":
		return alerts.EnginePostgres, true
	default:
		return "", false
	}
}

// StartAlertEvaluationLoop runs alert evaluation for all configured instances
// at the given interval. It blocks until ctx is cancelled.
//
// A PostgreSQL advisory lock (pg_try_advisory_xact_lock) is acquired at the
// start of each tick so that only one process evaluates in scaled deployments.
func StartAlertEvaluationLoop(ctx context.Context, pool *pgxpool.Pool, cfg *config.Config, alertSvc *AlertService, interval time.Duration) {
	if alertSvc == nil || cfg == nil || pool == nil {
		return
	}
	if interval <= 0 {
		interval = 60 * time.Second
	}

	log.Printf("[alerts] evaluation loop started (interval=%s, instances=%d)", interval, len(cfg.Instances))

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("[alerts] evaluation loop stopped")
			return
		case <-ticker.C:
			runOnceWithLock(ctx, pool, cfg, alertSvc)
		}
	}
}

// runOnceWithLock attempts to acquire an advisory lock inside a transaction.
// If another process already holds it, this tick is skipped silently.
func runOnceWithLock(ctx context.Context, pool *pgxpool.Pool, cfg *config.Config, alertSvc *AlertService) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		log.Printf("[alerts] failed to begin tx for advisory lock: %v", err)
		return
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var acquired bool
	if err := tx.QueryRow(ctx, "SELECT pg_try_advisory_xact_lock($1)", advisoryLockID).Scan(&acquired); err != nil {
		log.Printf("[alerts] advisory lock query failed: %v", err)
		return
	}
	if !acquired {
		return // another process is already evaluating
	}

	runOnce(ctx, cfg, alertSvc)
	_ = tx.Commit(ctx) // release the lock
}

func runOnce(ctx context.Context, cfg *config.Config, alertSvc *AlertService) {
	for _, inst := range cfg.Instances {
		engine, ok := engineForInstanceType(inst.Type)
		if !ok {
			continue
		}
		n, err := alertSvc.RunEvaluation(ctx, inst.Name, engine)
		if err != nil {
			log.Printf("[alerts] evaluation error instance=%s engine=%s: %v", inst.Name, engine, err)
			continue
		}
		if n > 0 {
			log.Printf("[alerts] %d new/updated alerts for %s (%s)", n, inst.Name, engine)
		}
	}
}
