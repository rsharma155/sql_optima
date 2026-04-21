//go:build integration

// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Integration tests for the PGSS dashboard pipeline using Testcontainers.
//
//	Tests the full write → read path: UpsertPgssQueryDim, ComputeAndStorePgssDelta1m,
//	GetPgssWorkloadTimeSeries, GetPgssTopQueries, GetPgssLatencyTimeSeries, GetPgssRegressions.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/rsharma155/sql_optima/internal/storage/hot"
)

// startTimescaleDB launches a TimescaleDB container and returns a pgxpool and cleanup func.
func startTimescaleDB(t *testing.T) (*pgxpool.Pool, func()) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	req := testcontainers.ContainerRequest{
		Image:        "timescale/timescaledb:latest-pg16",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     "test",
			"POSTGRES_PASSWORD": "test",
			"POSTGRES_DB":       "test",
		},
		WaitingFor: wait.ForListeningPort("5432/tcp").WithStartupTimeout(90 * time.Second),
	}

	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("start timescaledb container: %v", err)
	}

	host, _ := c.Host(ctx)
	port, _ := c.MappedPort(ctx, "5432")
	dsn := fmt.Sprintf("postgres://test:test@%s:%s/test?sslmode=disable", host, port.Port())

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		_ = c.Terminate(context.Background())
		t.Fatalf("connect to timescaledb: %v", err)
	}

	cleanup := func() {
		pool.Close()
		_ = c.Terminate(context.Background())
	}
	return pool, cleanup
}

// bootstrapPgssSchema creates the minimal tables needed for the PGSS pipeline.
func bootstrapPgssSchema(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	ddl := []string{
		`CREATE EXTENSION IF NOT EXISTS timescaledb CASCADE`,
		`CREATE TABLE IF NOT EXISTS postgres_query_stats (
			capture_timestamp    TIMESTAMPTZ NOT NULL,
			server_instance_name TEXT NOT NULL,
			query_id             BIGINT NOT NULL,
			query_text           TEXT,
			calls                BIGINT DEFAULT 0,
			total_time_ms        DOUBLE PRECISION DEFAULT 0,
			mean_time_ms         DOUBLE PRECISION DEFAULT 0,
			rows                 BIGINT DEFAULT 0,
			shared_blks_hit      BIGINT DEFAULT 0,
			shared_blks_read     BIGINT DEFAULT 0,
			shared_blks_dirtied  BIGINT DEFAULT 0,
			shared_blks_written  BIGINT DEFAULT 0,
			temp_blks_written    BIGINT DEFAULT 0,
			wal_bytes            BIGINT DEFAULT 0,
			wal_records          BIGINT DEFAULT 0,
			wal_fpi              BIGINT DEFAULT 0,
			total_plan_time      DOUBLE PRECISION DEFAULT 0,
			mean_plan_time       DOUBLE PRECISION DEFAULT 0,
			plans                BIGINT DEFAULT 0
		)`,
		`SELECT create_hypertable('postgres_query_stats', 'capture_timestamp', if_not_exists => TRUE)`,
		`CREATE TABLE IF NOT EXISTS pgss_query_dim (
			server_instance_name TEXT NOT NULL,
			query_id             BIGINT NOT NULL,
			query_text           TEXT,
			PRIMARY KEY (server_instance_name, query_id)
		)`,
		`CREATE TABLE IF NOT EXISTS pgss_delta_1m (
			capture_timestamp    TIMESTAMPTZ NOT NULL,
			server_instance_name TEXT NOT NULL,
			query_id             BIGINT NOT NULL,
			calls                BIGINT DEFAULT 0,
			total_exec_time      DOUBLE PRECISION DEFAULT 0,
			rows                 BIGINT DEFAULT 0,
			shared_blks_hit      BIGINT DEFAULT 0,
			shared_blks_read     BIGINT DEFAULT 0,
			temp_blks_written    BIGINT DEFAULT 0,
			wal_bytes            BIGINT DEFAULT 0,
			total_plan_time      DOUBLE PRECISION DEFAULT 0,
			mean_exec_time       DOUBLE PRECISION DEFAULT 0
		)`,
		`SELECT create_hypertable('pgss_delta_1m', 'capture_timestamp', if_not_exists => TRUE)`,
	}

	for _, stmt := range ddl {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			t.Fatalf("bootstrap DDL: %v\nSQL: %s", err, stmt)
		}
	}
}

// seedSnapshots inserts two consecutive query stats snapshots with known deltas.
func seedSnapshots(t *testing.T, pool *pgxpool.Pool, instance string, t0, t1 time.Time) {
	t.Helper()
	ctx := context.Background()

	// Snapshot at t0 (base)
	snap0 := []struct {
		qid   int64
		calls int64
		total float64
		rows  int64
		hit   int64
		read  int64
		temp  int64
		wal   int64
		plan  float64
		mean  float64
	}{
		{qid: 1001, calls: 100, total: 500, rows: 1000, hit: 5000, read: 200, temp: 10, wal: 2048, plan: 50, mean: 5.0},
		{qid: 1002, calls: 50, total: 300, rows: 500, hit: 3000, read: 100, temp: 5, wal: 1024, plan: 20, mean: 6.0},
	}
	for _, s := range snap0 {
		_, err := pool.Exec(ctx,
			`INSERT INTO postgres_query_stats
				(capture_timestamp, server_instance_name, query_id, calls, total_time_ms, mean_time_ms, rows,
				 shared_blks_hit, shared_blks_read, temp_blks_written, wal_bytes, total_plan_time)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
			t0, instance, s.qid, s.calls, s.total, s.mean, s.rows,
			s.hit, s.read, s.temp, s.wal, s.plan)
		if err != nil {
			t.Fatalf("seed snap0 qid=%d: %v", s.qid, err)
		}
	}

	// Snapshot at t1 (current) — cumulative counters increased
	snap1 := []struct {
		qid   int64
		calls int64
		total float64
		rows  int64
		hit   int64
		read  int64
		temp  int64
		wal   int64
		plan  float64
		mean  float64
	}{
		{qid: 1001, calls: 200, total: 1100, rows: 2200, hit: 10000, read: 400, temp: 20, wal: 4096, plan: 110, mean: 5.5},
		{qid: 1002, calls: 80, total: 600, rows: 800, hit: 5000, read: 200, temp: 8, wal: 2048, plan: 40, mean: 7.5},
	}
	for _, s := range snap1 {
		_, err := pool.Exec(ctx,
			`INSERT INTO postgres_query_stats
				(capture_timestamp, server_instance_name, query_id, calls, total_time_ms, mean_time_ms, rows,
				 shared_blks_hit, shared_blks_read, temp_blks_written, wal_bytes, total_plan_time)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
			t1, instance, s.qid, s.calls, s.total, s.mean, s.rows,
			s.hit, s.read, s.temp, s.wal, s.plan)
		if err != nil {
			t.Fatalf("seed snap1 qid=%d: %v", s.qid, err)
		}
	}
}

func TestPgssPipeline_Integration(t *testing.T) {
	pool, cleanup := startTimescaleDB(t)
	defer cleanup()

	bootstrapPgssSchema(t, pool)

	tl := hot.NewTimescaleLogger(pool)
	ctx := context.Background()
	instance := "pg-integration-test"

	t0 := time.Now().UTC().Truncate(time.Minute).Add(-2 * time.Minute)
	t1 := t0.Add(time.Minute)

	// ── Step 1: Upsert query dimension ──
	dimRows := []hot.PostgresQueryStatsSnapRow{
		{QueryID: 1001, QueryText: "SELECT * FROM orders WHERE id = $1"},
		{QueryID: 1002, QueryText: "UPDATE inventory SET qty = qty - $1 WHERE sku = $2"},
	}
	if err := tl.UpsertPgssQueryDim(ctx, instance, dimRows); err != nil {
		t.Fatalf("UpsertPgssQueryDim: %v", err)
	}

	// Idempotent — second call should not error
	if err := tl.UpsertPgssQueryDim(ctx, instance, dimRows); err != nil {
		t.Fatalf("UpsertPgssQueryDim idempotent: %v", err)
	}

	// ── Step 2: Seed snapshots and compute delta ──
	seedSnapshots(t, pool, instance, t0, t1)

	if err := tl.ComputeAndStorePgssDelta1m(ctx, instance, t1); err != nil {
		t.Fatalf("ComputeAndStorePgssDelta1m: %v", err)
	}

	// ── Step 3: Read workload time-series ──
	from := t0.Add(-time.Minute)
	to := t1.Add(time.Minute)

	workload, err := tl.GetPgssWorkloadTimeSeries(ctx, instance, from, to)
	if err != nil {
		t.Fatalf("GetPgssWorkloadTimeSeries: %v", err)
	}
	if len(workload) == 0 {
		t.Fatal("expected at least 1 workload point")
	}
	wp := workload[0]
	if wp.QPS <= 0 {
		t.Errorf("expected positive QPS, got %f", wp.QPS)
	}
	if wp.CacheHitRatio <= 0 || wp.CacheHitRatio > 100 {
		t.Errorf("cache hit ratio out of range: %f", wp.CacheHitRatio)
	}

	// ── Step 4: Read top queries ──
	top, err := tl.GetPgssTopQueries(ctx, instance, from, to, "total_time", 50)
	if err != nil {
		t.Fatalf("GetPgssTopQueries: %v", err)
	}
	if len(top) == 0 {
		t.Fatal("expected at least 1 top query")
	}
	// Verify query text was joined from dim table
	found := false
	for _, q := range top {
		if q.Query != "" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected query_text to be joined from pgss_query_dim")
	}

	// ── Step 5: Read latency time-series ──
	latency, err := tl.GetPgssLatencyTimeSeries(ctx, instance, from, to)
	if err != nil {
		t.Fatalf("GetPgssLatencyTimeSeries: %v", err)
	}
	if len(latency) == 0 {
		t.Fatal("expected at least 1 latency point")
	}
	lp := latency[0]
	if lp.P50 <= 0 {
		t.Errorf("expected positive P50, got %f", lp.P50)
	}
	if lp.P99 < lp.P50 {
		t.Errorf("P99 (%f) should be >= P50 (%f)", lp.P99, lp.P50)
	}

	// ── Step 6: Regressions (should be empty — single delta window) ──
	regressions, err := tl.GetPgssRegressions(ctx, instance)
	if err != nil {
		t.Fatalf("GetPgssRegressions: %v", err)
	}
	// With only 1 minute of deltas, there should be no regressions
	_ = regressions // may be nil or empty — both are fine
}
