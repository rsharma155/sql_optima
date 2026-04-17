// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: TimescaleDB connection pool management and configuration for time-series metric persistence.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package hot

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Metric struct {
	CaptureTimestamp time.Time              `json:"capture_timestamp"`
	ServerName       string                 `json:"server_name"`
	MetricName       string                 `json:"metric_name"`
	MetricValue      float64                `json:"metric_value"`
	Tags             map[string]interface{} `json:"tags,omitempty"`
}

type Config struct {
	Host     string
	Port     string
	User     string
	Password string
	Database string
	SSLMode  string
	MaxConns int32
}

func (c *Config) connString() string {
	sslMode := c.SSLMode
	if sslMode == "" {
		sslMode = "disable"
	}
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(c.User, c.Password),
		Host:   net.JoinHostPort(c.Host, c.Port),
		Path:   "/" + strings.TrimPrefix(c.Database, "/"),
	}
	q := url.Values{}
	q.Set("sslmode", sslMode)
	u.RawQuery = q.Encode()
	return u.String()
}

func DefaultConfig() *Config {
	// Support both the newer TIMESCALEDB_* variables and the docker-compose DB_* variables.
	host := getEnv("TIMESCALEDB_HOST", "")
	if host == "" {
		host = getEnv("DB_HOST", "localhost")
	}
	port := getEnv("TIMESCALEDB_PORT", "")
	if port == "" {
		port = getEnv("DB_PORT", "5432")
	}
	return &Config{
		Host:     host,
		Port:     port,
		User:     getEnv("DB_USER", "dbmonitor"),
		Password: getEnv("DB_PASSWORD", ""),
		Database: getEnv("DB_NAME", "dbmonitor_metrics"),
		SSLMode:  getEnv("TIMESCALEDB_SSLMODE", "disable"),
		MaxConns: 50,
	}
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

type HotStorage struct {
	pool   *pgxpool.Pool
	config *Config
	mu     sync.RWMutex
}

func New(cfg *Config) (*HotStorage, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	log.Printf("[TimescaleDB] Attempting to connect (host=%s port=%s db=%s user_set=%v sslmode=%s)...",
		cfg.Host, cfg.Port, cfg.Database, strings.TrimSpace(cfg.User) != "", cfg.SSLMode)

	poolConfig, err := pgxpool.ParseConfig(cfg.connString())
	if err != nil {
		return nil, fmt.Errorf("failed to parse connection string: %w", err)
	}

	poolConfig.MaxConns = cfg.MaxConns
	poolConfig.MinConns = 5
	poolConfig.MaxConnLifetime = 30 * time.Minute
	poolConfig.MaxConnIdleTime = 10 * time.Minute
	poolConfig.HealthCheckPeriod = 30 * time.Second

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	hs := &HotStorage{pool: pool, config: cfg}
	// Runtime migrations are intentionally disabled.
	// TimescaleDB schema should be provisioned externally using `infrastructure/sql_scripts/00_timescale_schema.sql`.
	// This avoids schema changes on API startup and prevents surprise load on shared Timescale instances.
	log.Printf("[TimescaleDB] Connected (schema provisioned externally; runtime migrations disabled)")

	return hs, nil
}

func (s *HotStorage) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pool != nil {
		s.pool.Close()
		s.pool = nil
	}
}

func (s *HotStorage) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

func (s *HotStorage) Pool() *pgxpool.Pool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.pool
}

func (s *HotStorage) Stats() *pgxpool.Stat {
	return s.pool.Stat()
}

// NOTE: Runtime migrations were removed in favor of external schema provisioning.

func (s *HotStorage) GetMetricsForArchive(ctx context.Context, cutoff time.Time, limit int) ([]*Metric, string, error) {
	var metrics []*Metric
	var servers []string
	serverSet := make(map[string]bool)

	tables := []string{
		"sqlserver_ag_health",
		"sqlserver_database_throughput",
		"sqlserver_query_store_stats",
		"sqlserver_top_queries",
		"postgres_bgwriter_stats",
		"postgres_archiver_stats",
	}

	for _, table := range tables {
		query := fmt.Sprintf(`
			SELECT capture_timestamp, server_instance_name, 
				   COALESCE($1::text, 'unknown_metric') as metric_name,
				   COALESCE($2::float, 0) as metric_value,
				   '{}'::jsonb as tags
			FROM %s
			WHERE capture_timestamp < $3
			LIMIT $4`,
			table)

		rows, err := s.pool.Query(ctx, query, table, 0, cutoff, limit/len(tables))
		if err != nil {
			log.Printf("[Archiver] Warning: failed to query %s: %v", table, err)
			continue
		}

		for rows.Next() {
			var m Metric
			if err := rows.Scan(&m.CaptureTimestamp, &m.ServerName, &m.MetricName, &m.MetricValue, nil); err != nil {
				continue
			}
			metrics = append(metrics, &m)
			if !serverSet[m.ServerName] {
				serverSet[m.ServerName] = true
				servers = append(servers, m.ServerName)
			}
		}
		rows.Close()
	}

	return metrics, strings.Join(servers, ","), nil
}

func (s *HotStorage) DeleteChunksOlderThan(ctx context.Context, duration time.Duration) error {
	tables := []string{
		"sqlserver_ag_health",
		"sqlserver_database_throughput",
		"sqlserver_query_store_stats",
		"sqlserver_top_queries",
		"postgres_bgwriter_stats",
		"postgres_archiver_stats",
	}

	for _, table := range tables {
		query := fmt.Sprintf(`SELECT drop_chunks('%s', older_than => INTERVAL '%d seconds')`,
			table, int(duration.Seconds()))
		if _, err := s.pool.Exec(ctx, query); err != nil {
			log.Printf("[Archiver] Warning: failed to drop chunks for %s: %v", table, err)
		}
	}
	return nil
}
