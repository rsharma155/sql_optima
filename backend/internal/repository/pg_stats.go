// Package repository provides data access layer for database operations.
// It handles connections and queries for both PostgreSQL and SQL Server databases.
// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL statistics collector for query performance and statement statistics.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/models"
)

func init() {
	// Explicitly register the pgx driver as "postgres"
	sql.Register("postgres", stdlib.GetDefaultDriver())
	log.Println("[POSTGRES] Driver explicitly registered")
}

// PgRepository manages PostgreSQL database connections and provides methods for querying metrics.
// It supports connection pooling, automatic database discovery, and thread-safe operations.
type PgRepository struct {
	conns  map[string]*sql.DB // Connection pool per instance
	status map[string]string  // Instance status: "online", "offline", "error"
	mutex  sync.RWMutex       // Thread-safe access to connections
	cfg    *config.Config     // Application configuration

	// Lightweight in-memory cache for size deltas (growth estimation).
	lastDbSizeBytes map[string]int64
	lastDbSizeAt    map[string]time.Time
}

// GetConn returns a live PostgreSQL connection for an instance name.
func (c *PgRepository) GetConn(instanceName string) (*sql.DB, bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	db, ok := c.conns[instanceName]
	return db, ok
}

// HasConnection returns true if the instance has an active connection in the pool.
func (c *PgRepository) HasConnection(instanceName string) bool {
	c.mutex.RLock()
	_, ok := c.conns[instanceName]
	c.mutex.RUnlock()
	return ok
}

// NewPgRepository creates a new PostgreSQL repository and initializes connections to all configured instances.
// It supports environment variable overrides for credentials and automatic database discovery.
func NewPgRepository(cfg *config.Config) *PgRepository {
	c := &PgRepository{
		conns:  make(map[string]*sql.DB),
		status: make(map[string]string),
		cfg:    cfg,
		lastDbSizeBytes: make(map[string]int64),
		lastDbSizeAt:    make(map[string]time.Time),
	}

	for i, inst := range cfg.Instances {
		if inst.Type == "postgres" {
			port := inst.Port
			if port == 0 {
				port = 5432
			}

			// Support environment variable overrides for credentials
			user := inst.User
			password := inst.Password

			envPrefix := fmt.Sprintf("DB_%s", strings.ToUpper(strings.ReplaceAll(inst.Name, "-", "_")))
			if user == "" {
				user = os.Getenv(envPrefix + "_USER")
			}
			if password == "" {
				password = os.Getenv(envPrefix + "_PASSWORD")
			}

			// Default to postgres user if not specified
			if user == "" {
				user = "postgres"
			}

			sslmode := inst.SSLMode
			if sslmode == "" {
				sslmode = "disable"
			}

			// Build connection string with optional SSL certificates
			connStr := fmt.Sprintf("host=%s port=%d user=%s dbname=postgres sslmode=%s", inst.Host, port, user, sslmode)

			if password != "" {
				connStr += fmt.Sprintf(" password=%s", password)
			}
			if inst.SSLCert != "" && inst.SSLKey != "" {
				connStr += fmt.Sprintf(" sslcert=%s sslkey=%s", inst.SSLCert, inst.SSLKey)
			}
			if inst.SSLRootCert != "" {
				connStr += fmt.Sprintf(" sslrootcert=%s", inst.SSLRootCert)
			}

			db, err := sql.Open("postgres", connStr)
			if err != nil {
				log.Printf("[POSTGRES] DSN Parse Error %s: %v", inst.Name, err)
				c.status[inst.Name] = "error"
				continue
			}

			// Test connection
			if err := db.Ping(); err != nil {
				log.Printf("[POSTGRES] Connection Failed %s: %v", inst.Name, err)
				c.status[inst.Name] = "error"
				continue
			}

			log.Printf("[POSTGRES] Connected to %s (%s:%d)", inst.Name, inst.Host, port)

			// Configure connection pool for optimal resource usage
			db.SetMaxOpenConns(5)
			db.SetMaxIdleConns(2)
			db.SetConnMaxLifetime(time.Minute * 10)

			c.conns[inst.Name] = db
			c.status[inst.Name] = "unknown"
			log.Printf("[POSTGRES] DEBUG: Added connection to pool for %s, total: %d, conns map: %v", inst.Name, len(c.conns), c.conns)

			// Auto-discover databases if not configured
			if len(inst.Databases) == 0 {
				go func(instName string, db *sql.DB, idx int) {
					defer func() {
						if r := recover(); r != nil {
							log.Printf("[POSTGRES] Panic during database discovery for %s: %v", instName, r)
						}
					}()

					query := "SELECT datname FROM pg_database WHERE datistemplate = false AND datname NOT IN ('postgres')"
					rows, err := db.Query(query)
					if err == nil {
						var discoverDbs []string
						for rows.Next() {
							var dbName string
							rows.Scan(&dbName)
							discoverDbs = append(discoverDbs, dbName)
						}
						rows.Close()
						cfg.Instances[idx].Databases = discoverDbs
						log.Printf("[POSTGRES] Auto-discovered %d databases for %s", len(discoverDbs), instName)
					} else {
						log.Printf("[POSTGRES] Dynamic Database Binding failure %s: %v", instName, err)
					}
				}(inst.Name, db, i)
			}
		}
	}
	return c
}

// PingAll tests connectivity to all configured PostgreSQL instances concurrently.
// Updates instance status based on ping results.
func (c *PgRepository) PingAll() {
	var wg sync.WaitGroup
	for name, db := range c.conns {
		wg.Add(1)
		go func(n string, connection *sql.DB) {
			defer wg.Done()
			err := connection.Ping()
			c.mutex.Lock()
			if err != nil {
				c.status[n] = "offline"
				log.Printf("[POSTGRES] Connection failed to %s: %v", n, err)
			} else {
				c.status[n] = "online"
				log.Printf("[POSTGRES] Connection successful to %s", n)
			}
			c.mutex.Unlock()
		}(name, db)
	}
	wg.Wait()
}

// reconnectInstance attempts to reestablish a connection to a specific PostgreSQL instance.
// Used when existing connection becomes stale or disconnected.
func (c *PgRepository) reconnectInstance(instanceName string) bool {
	if c.cfg == nil {
		return false
	}

	var inst config.Instance
	found := false
	for _, instance := range c.cfg.Instances {
		if instance.Name == instanceName {
			inst = instance
			found = true
			break
		}
	}

	if !found {
		log.Printf("[POSTGRES] reconnectInstance: instance %s not found in config", instanceName)
		return false
	}

	c.mutex.Lock()
	if oldDb, ok := c.conns[instanceName]; ok && oldDb != nil {
		oldDb.Close()
	}
	c.mutex.Unlock()

	port := inst.Port
	if port == 0 {
		port = 5432
	}

	user := inst.User
	password := inst.Password

	envPrefix := fmt.Sprintf("DB_%s", strings.ToUpper(strings.ReplaceAll(inst.Name, "-", "_")))
	if user == "" {
		user = os.Getenv(envPrefix + "_USER")
	}
	if password == "" {
		password = os.Getenv(envPrefix + "_PASSWORD")
	}

	if user == "" {
		user = "postgres"
	}

	sslmode := inst.SSLMode
	if sslmode == "" {
		sslmode = "disable"
	}

	connStr := fmt.Sprintf("host=%s port=%d user=%s dbname=postgres sslmode=%s", inst.Host, port, user, sslmode)
	if password != "" {
		connStr += fmt.Sprintf(" password=%s", password)
	}
	if inst.SSLCert != "" && inst.SSLKey != "" {
		connStr += fmt.Sprintf(" sslcert=%s sslkey=%s", inst.SSLCert, inst.SSLKey)
	}
	if inst.SSLRootCert != "" {
		connStr += fmt.Sprintf(" sslrootcert=%s", inst.SSLRootCert)
	}

	newDb, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Printf("[POSTGRES] reconnectInstance: failed to open connection for %s: %v", instanceName, err)
		c.mutex.Lock()
		c.status[instanceName] = "error"
		c.mutex.Unlock()
		return false
	}

	newDb.SetMaxOpenConns(5)
	newDb.SetMaxIdleConns(2)
	newDb.SetConnMaxLifetime(time.Minute * 10)

	if err := newDb.Ping(); err != nil {
		log.Printf("[POSTGRES] reconnectInstance: ping failed for %s: %v", instanceName, err)
		newDb.Close()
		c.mutex.Lock()
		c.status[instanceName] = "offline"
		c.mutex.Unlock()
		return false
	}

	c.mutex.Lock()
	c.conns[instanceName] = newDb
	c.status[instanceName] = "online"
	c.mutex.Unlock()

	log.Printf("[POSTGRES] Successfully reconnected to %s", instanceName)
	return true
}

// OpenConnForDatabase creates a short-lived connection to a specific database on a configured instance.
// This is used for per-database collectors (e.g. pg_stat_user_* is scoped to the connected database).
func (c *PgRepository) OpenConnForDatabase(ctx context.Context, instanceName, dbName string) (*sql.DB, error) {
	if c == nil || c.cfg == nil {
		return nil, fmt.Errorf("postgres repo not configured")
	}
	dbName = strings.TrimSpace(dbName)
	if dbName == "" {
		return nil, fmt.Errorf("dbName is required")
	}

	var inst config.Instance
	found := false
	for _, instance := range c.cfg.Instances {
		if instance.Name == instanceName {
			inst = instance
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("instance not found: %s", instanceName)
	}

	port := inst.Port
	if port == 0 {
		port = 5432
	}

	user := inst.User
	password := inst.Password
	envPrefix := fmt.Sprintf("DB_%s", strings.ToUpper(strings.ReplaceAll(inst.Name, "-", "_")))
	if user == "" {
		user = os.Getenv(envPrefix + "_USER")
	}
	if password == "" {
		password = os.Getenv(envPrefix + "_PASSWORD")
	}
	if user == "" {
		user = "postgres"
	}

	sslmode := inst.SSLMode
	if sslmode == "" {
		sslmode = "disable"
	}

	connStr := fmt.Sprintf("host=%s port=%d user=%s dbname=%s sslmode=%s", inst.Host, port, user, dbName, sslmode)
	if password != "" {
		connStr += fmt.Sprintf(" password=%s", password)
	}
	if inst.SSLCert != "" && inst.SSLKey != "" {
		connStr += fmt.Sprintf(" sslcert=%s sslkey=%s", inst.SSLCert, inst.SSLKey)
	}
	if inst.SSLRootCert != "" {
		connStr += fmt.Sprintf(" sslrootcert=%s", inst.SSLRootCert)
	}

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(2)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(5 * time.Minute)

	// Keep per-db connects from hanging the collector tick.
	pingCtx := ctx
	var cancel context.CancelFunc
	if pingCtx == nil {
		pingCtx = context.Background()
	}
	pingCtx, cancel = context.WithTimeout(pingCtx, 5*time.Second)
	defer cancel()

	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

// GetInstanceStatus returns the current connection status of a PostgreSQL instance.
// Returns: "online", "offline", "error", or "unknown"
func (c *PgRepository) GetInstanceStatus(instanceName string) string {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	if status, exists := c.status[instanceName]; exists {
		return status
	}
	return "unknown"
}

// GetAllInstanceStatuses returns the connection status of all configured PostgreSQL instances.
func (c *PgRepository) GetAllInstanceStatuses() map[string]string {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	statuses := make(map[string]string)
	for name, status := range c.status {
		statuses[name] = status
	}
	return statuses
}

// UpdateInstanceStatus performs a ping to check and update the status of an instance.
func (c *PgRepository) UpdateInstanceStatus(instanceName string) {
	c.mutex.RLock()
	db, ok := c.conns[instanceName]
	c.mutex.RUnlock()

	if !ok || db == nil {
		c.mutex.Lock()
		c.status[instanceName] = "error"
		c.mutex.Unlock()
		return
	}

	err := db.Ping()
	c.mutex.Lock()
	if err != nil {
		c.status[instanceName] = "offline"
	} else {
		c.status[instanceName] = "online"
	}
	c.mutex.Unlock()
}

// GetGlobalMetric returns global instance metrics for the global dashboard overview.
// Includes connection status and basic resource usage estimates.
func (c *PgRepository) GetGlobalMetric(name string, base models.GlobalInstanceMetric) models.GlobalInstanceMetric {
	c.mutex.RLock()
	db, ok := c.conns[name]
	c.mutex.RUnlock()

	if !ok || db == nil {
		base.Status = 2
		base.Error = "Connection Context Lost"
		return base
	}

	if err := db.Ping(); err != nil {
		base.Status = 2
		base.Error = err.Error()
		return base
	}

	base.Status = 0

	// PostgreSQL doesn't provide direct OS CPU/memory metrics without extensions
	// These are set to 0 to indicate unavailable data
	base.CPUUsage = 0
	base.MemoryPct = 0

	return base
}

// GetConnectionStats returns active, idle, and total connection counts for a PostgreSQL instance.
// Used for connection pool monitoring and capacity planning.
func (c *PgRepository) GetConnectionStats(instanceName string) (active int, idle int, total int, err error) {
	c.mutex.RLock()
	db, ok := c.conns[instanceName]
	c.mutex.RUnlock()

	if !ok || db == nil {
		log.Printf("[POSTGRES] GetConnectionStats: connection not found for %s, attempting reconnect", instanceName)
		if c.reconnectInstance(instanceName) {
			c.mutex.RLock()
			db, ok = c.conns[instanceName]
			c.mutex.RUnlock()
			if !ok || db == nil {
				return 0, 0, 0, fmt.Errorf("connection not found after reconnect")
			}
		} else {
			return 0, 0, 0, fmt.Errorf("connection not found")
		}
	}

	query := `
		SELECT 
			COUNT(*) as total,
			COUNT(*) FILTER (WHERE state = 'active') as active,
			COUNT(*) FILTER (WHERE state = 'idle') as idle
		FROM pg_stat_activity
		WHERE datname IS NOT NULL
	`
	err = db.QueryRow(query).Scan(&total, &active, &idle)
	return
}

// GetReplicationLag returns replication lag in MB for standby servers.
// Returns status: "primary", "standby", or "unknown"
func (c *PgRepository) GetReplicationLag(instanceName string) (lagMB float64, status string, err error) {
	c.mutex.RLock()
	db, ok := c.conns[instanceName]
	c.mutex.RUnlock()

	if !ok || db == nil {
		return 0, "unknown", fmt.Errorf("connection not found")
	}

	// Check if this is a standby using pg_is_in_recovery()
	var isStandby bool
	err = db.QueryRow("SELECT pg_is_in_recovery()").Scan(&isStandby)
	if err != nil {
		return 0, "unknown", err
	}

	if !isStandby {
		return 0, "primary", nil
	}

	// Calculate lag on standby using LSN difference
	query := `
		SELECT 
			CASE 
				WHEN pg_last_wal_replay_lsn() IS NOT NULL AND pg_last_wal_receive_lsn() IS NOT NULL 
				THEN EXTRACT(EPOCH FROM (pg_last_wal_receive_lsn() - pg_last_wal_replay_lsn())) / 1024 / 1024
				ELSE 0 
			END as lag_mb
	`
	err = db.QueryRow(query).Scan(&lagMB)
	if err != nil {
		return 0, "standby", err
	}

	status = "standby"
	return
}

// GetServerInfo returns PostgreSQL version and uptime information.
// Uptime is calculated from pg_postmaster_start_time().
func (c *PgRepository) GetServerInfo(instanceName string) (version string, uptime string, err error) {
	c.mutex.RLock()
	db, ok := c.conns[instanceName]
	c.mutex.RUnlock()

	if !ok || db == nil {
		log.Printf("[POSTGRES] GetServerInfo: connection not found for %s, attempting reconnect", instanceName)
		if c.reconnectInstance(instanceName) {
			c.mutex.RLock()
			db, ok = c.conns[instanceName]
			c.mutex.RUnlock()
			if !ok || db == nil {
				return "", "", fmt.Errorf("connection not found after reconnect")
			}
		} else {
			return "", "", fmt.Errorf("connection not found")
		}
	}

	// Get version string and extract version number
	err = db.QueryRow("SELECT version()").Scan(&version)
	if err != nil {
		return "", "", err
	}

	// Extract version number from full version string
	if strings.Contains(version, "PostgreSQL ") {
		parts := strings.Split(version, " ")
		if len(parts) > 1 {
			version = parts[1]
		}
	}

	// Get server start time and calculate uptime
	var startTime time.Time
	err = db.QueryRow("SELECT pg_postmaster_start_time()").Scan(&startTime)
	if err != nil {
		return version, "", err
	}

	duration := time.Since(startTime)
	days := int(duration.Hours() / 24)
	hours := int(duration.Hours()) % 24
	minutes := int(duration.Minutes()) % 60
	seconds := int(duration.Seconds()) % 60

	uptime = fmt.Sprintf("%d Days, %02d:%02d:%02d", days, hours, minutes, seconds)

	return version, uptime, nil
}

// GetSystemStats returns estimated CPU and memory usage metrics.
// Note: These are approximations based on PostgreSQL internals, not actual OS metrics.
func (c *PgRepository) GetSystemStats(instanceName string) (cpuUsage float64, memoryUsage float64, err error) {
	c.mutex.RLock()
	db, ok := c.conns[instanceName]
	c.mutex.RUnlock()

	if !ok || db == nil {
		return 0, 0, fmt.Errorf("connection not found")
	}

	// Estimate CPU based on active connections vs max connections
	var activeConnections int
	err = db.QueryRow("SELECT count(*) FROM pg_stat_activity WHERE state = 'active'").Scan(&activeConnections)
	if err != nil {
		return 0, 0, err
	}

	var maxConnections int
	err = db.QueryRow("SELECT setting::int FROM pg_settings WHERE name = 'max_connections'").Scan(&maxConnections)
	if err != nil {
		maxConnections = 100 // fallback
	}

	cpuUsage = float64(activeConnections) / float64(maxConnections) * 100
	if cpuUsage > 100 {
		cpuUsage = 100
	}

	// Estimate memory based on shared buffers
	var sharedBuffersMB int
	err = db.QueryRow(`
		SELECT (setting::bigint * 8192) / 1024 / 1024
		FROM pg_settings
		WHERE name = 'shared_buffers'
	`).Scan(&sharedBuffersMB)
	if err != nil {
		sharedBuffersMB = 128 // fallback
	}

	memoryUsage = float64(sharedBuffersMB) / 1024 * 100
	if memoryUsage > 100 {
		memoryUsage = 85 // cap at reasonable level
	}

	return cpuUsage, memoryUsage, nil
}

// GetDatabases returns the list of user databases (excludes template databases and 'postgres').
func (c *PgRepository) GetDatabases(instanceName string) ([]string, error) {
	c.mutex.RLock()
	db, ok := c.conns[instanceName]
	c.mutex.RUnlock()

	if !ok || db == nil {
		log.Printf("[POSTGRES] GetDatabases: connection not found for %s, attempting reconnect", instanceName)
		if c.reconnectInstance(instanceName) {
			c.mutex.RLock()
			db, ok = c.conns[instanceName]
			c.mutex.RUnlock()
			if !ok || db == nil {
				return nil, fmt.Errorf("connection not found after reconnect")
			}
		} else {
			return nil, fmt.Errorf("connection not found")
		}
	}

	// List all non-template databases including postgres (needed for CNPG)
	query := "SELECT datname FROM pg_database WHERE datistemplate = false AND datallowconn = true ORDER BY datname"
	rows, err := db.Query(query)
	if err != nil {
		log.Printf("[POSTGRES] GetDatabases: query failed for %s: %v", instanceName, err)
		return nil, err
	}
	defer rows.Close()

	var databases []string
	for rows.Next() {
		var dbName string
		if err := rows.Scan(&dbName); err != nil {
			return nil, err
		}
		databases = append(databases, dbName)
	}

	log.Printf("[POSTGRES] GetDatabases: found %d databases for %s: %v", len(databases), instanceName, databases)
	return databases, nil
}

// GetLongRunningQueries returns queries running longer than specified duration.
// Used to identify problematic long-running queries.
func (c *PgRepository) GetLongRunningQueries(instanceName string, minDurationSeconds int) ([]models.PgSession, error) {
	c.mutex.RLock()
	db, ok := c.conns[instanceName]
	c.mutex.RUnlock()

	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}

	query := `
		SELECT
			pid,
			COALESCE(usename::text, '') AS usename,
			COALESCE(datname::text, '') AS datname,
			COALESCE(client_addr::text, '') AS client_addr,
			client_port,
			backend_start,
			query_start,
			state_change,
			wait_event_type,
			wait_event,
			state,
			query
		FROM pg_stat_activity
		WHERE pid <> pg_backend_pid()
		AND state = 'active'
		AND COALESCE(query_start, xact_start) IS NOT NULL
		AND extract(epoch FROM (now() - COALESCE(query_start, xact_start))) > $1
		AND query NOT ILIKE '%pg_stat_activity%'
		AND query NOT ILIKE 'autovacuum:%'
		ORDER BY COALESCE(query_start, xact_start) ASC NULLS LAST
		LIMIT 50
	`

	rows, err := db.Query(query, minDurationSeconds)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []models.PgSession
	for rows.Next() {
		var s models.PgSession
		err := rows.Scan(
			&s.PID,
			&s.UserName,
			&s.Database,
			&s.ClientAddr,
			&s.ClientPort,
			&s.BackendStart,
			&s.QueryStart,
			&s.StateChange,
			&s.WaitEventType,
			&s.WaitEvent,
			&s.State,
			&s.Query,
		)
		if err != nil {
			continue
		}
		sessions = append(sessions, s)
	}

	return sessions, nil
}

// GetActiveQueries returns all currently active queries.
func (c *PgRepository) GetActiveQueries(instanceName string) ([]models.PgSession, error) {
	c.mutex.RLock()
	db, ok := c.conns[instanceName]
	c.mutex.RUnlock()

	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}

	query := `
		SELECT
			pid,
			COALESCE(usename::text, '') AS usename,
			COALESCE(datname::text, '') AS datname,
			COALESCE(client_addr::text, '') AS client_addr,
			client_port,
			backend_start,
			query_start,
			state_change,
			wait_event_type,
			wait_event,
			state,
			query
		FROM pg_stat_activity
		WHERE pid <> pg_backend_pid()
		AND state = 'active'
		AND COALESCE(query_start, xact_start) IS NOT NULL
		AND query NOT ILIKE '%pg_stat_activity%'
		AND query NOT ILIKE 'autovacuum:%'
		ORDER BY COALESCE(query_start, xact_start) ASC NULLS LAST
		LIMIT 100
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []models.PgSession
	for rows.Next() {
		var s models.PgSession
		err := rows.Scan(
			&s.PID,
			&s.UserName,
			&s.Database,
			&s.ClientAddr,
			&s.ClientPort,
			&s.BackendStart,
			&s.QueryStart,
			&s.StateChange,
			&s.WaitEventType,
			&s.WaitEvent,
			&s.State,
			&s.Query,
		)
		if err != nil {
			continue
		}
		sessions = append(sessions, s)
	}

	return sessions, nil
}

// PgSession represents a PostgreSQL session/connection with query details.
type PgSession struct {
	PID        int        `json:"pid"`
	User       string     `json:"user"`
	Database   string     `json:"database"`
	AppName    string     `json:"app_name"`
	State      string     `json:"state"`
	Duration   string     `json:"duration"`
	WaitEvent  string     `json:"wait_event"`
	BlockedBy  *int       `json:"blocked_by"`
	Query      string     `json:"query"`
	QueryStart *time.Time `json:"-"`
}

// GetSessions returns active sessions with blocking information.
func (c *PgRepository) GetSessions(instanceName string) ([]PgSession, error) {
	c.mutex.RLock()
	db, ok := c.conns[instanceName]
	c.mutex.RUnlock()

	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}

	query := `
		SELECT 
			pid,
			usename,
			datname,
			application_name,
			state,
			EXTRACT(EPOCH FROM (now() - state_change)) as duration_seconds,
			CASE
				WHEN wait_event_type IS NULL AND wait_event IS NULL THEN ''
				ELSE COALESCE(wait_event_type, '') || ':' || COALESCE(wait_event, '')
			END as wait_event,
			pg_blocking_pids(pid) as blocked_by,
			query
		FROM pg_stat_activity 
		WHERE pid <> pg_backend_pid()
		ORDER BY state_change DESC
		LIMIT 100
	`
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []PgSession
	for rows.Next() {
		var s PgSession
		var durationSeconds float64
		var blockedByArr []int
		err := rows.Scan(&s.PID, &s.User, &s.Database, &s.AppName, &s.State, &durationSeconds, &s.WaitEvent, &blockedByArr, &s.Query)
		if err != nil {
			continue
		}

		// Format duration as HH:MM:SS
		duration := time.Duration(durationSeconds) * time.Second
		hours := int(duration.Hours())
		minutes := int(duration.Minutes()) % 60
		seconds := int(duration.Seconds()) % 60
		s.Duration = fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)

		// Set blocked_by if any
		if len(blockedByArr) > 0 {
			s.BlockedBy = &blockedByArr[0]
		}

		sessions = append(sessions, s)
	}

	return sessions, nil
}

// PgLock represents a PostgreSQL lock with metadata.
type PgLock struct {
	PID        int    `json:"pid"`
	LockType   string `json:"lock_type"`
	Relation   string `json:"relation"`
	Mode       string `json:"mode"`
	Granted    bool   `json:"granted"`
	WaitingFor string `json:"waiting_for"`
}

// GetLocks returns all current locks with waiting time for ungranted locks.
func (c *PgRepository) GetLocks(instanceName string) ([]PgLock, error) {
	c.mutex.RLock()
	db, ok := c.conns[instanceName]
	c.mutex.RUnlock()

	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}

	query := `
		SELECT 
			l.pid,
			l.locktype,
			COALESCE(r.relname || ' (' || r.oid || ')', 'virtual') as relation,
			l.mode,
			l.granted,
			CASE WHEN l.granted = false THEN EXTRACT(EPOCH FROM (now() - a.state_change)) ELSE 0 END as waiting_seconds
		FROM pg_locks l
		LEFT JOIN pg_class r ON l.relation = r.oid
		LEFT JOIN pg_stat_activity a ON l.pid = a.pid
		WHERE l.pid <> pg_backend_pid()
		ORDER BY l.granted, waiting_seconds DESC
		LIMIT 100
	`
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var locks []PgLock
	for rows.Next() {
		var l PgLock
		var waitingSeconds float64
		err := rows.Scan(&l.PID, &l.LockType, &l.Relation, &l.Mode, &l.Granted, &waitingSeconds)
		if err != nil {
			continue
		}

		if !l.Granted && waitingSeconds > 0 {
			duration := time.Duration(waitingSeconds) * time.Second
			minutes := int(duration.Minutes())
			seconds := int(duration.Seconds()) % 60
			l.WaitingFor = fmt.Sprintf("%dm %ds", minutes, seconds)
		} else {
			l.WaitingFor = "-"
		}

		locks = append(locks, l)
	}

	return locks, nil
}

// PgBlockingNode represents a node in the blocking tree visualization.
type PgBlockingNode struct {
	PID        int              `json:"pid"`
	User       string           `json:"user,omitempty"`
	Database   string           `json:"database,omitempty"`
	State      string           `json:"state"`
	QueryStart *time.Time       `json:"query_start,omitempty"`
	Duration   string           `json:"duration"`
	WaitEvent  string           `json:"wait_event"`
	Query      string           `json:"query"`
	BlockedBy  []PgBlockingNode `json:"blocked_by"`
}

// GetBlockingTree returns hierarchical blocking relationships between sessions.
func (c *PgRepository) GetBlockingTree(instanceName string) ([]PgBlockingNode, error) {
	c.mutex.RLock()
	db, ok := c.conns[instanceName]
	c.mutex.RUnlock()

	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}

	// Use pg_locks to discover blocked->blocking pairs.
	// This is more reliable than filtering pg_stat_activity by state and is less sensitive to
	// limited visibility of other sessions.
	pairsQ := `
		WITH waiting AS (
			SELECT *
			FROM pg_locks
			WHERE NOT granted
		),
		holding AS (
			SELECT *
			FROM pg_locks
			WHERE granted
		)
		SELECT
			w.pid  AS blocked_pid,
			h.pid  AS blocking_pid
		FROM waiting w
		JOIN holding h
		  ON h.locktype IS NOT DISTINCT FROM w.locktype
		 AND h.database IS NOT DISTINCT FROM w.database
		 AND h.relation IS NOT DISTINCT FROM w.relation
		 AND h.page IS NOT DISTINCT FROM w.page
		 AND h.tuple IS NOT DISTINCT FROM w.tuple
		 AND h.virtualxid IS NOT DISTINCT FROM w.virtualxid
		 AND h.transactionid IS NOT DISTINCT FROM w.transactionid
		 AND h.classid IS NOT DISTINCT FROM w.classid
		 AND h.objid IS NOT DISTINCT FROM w.objid
		 AND h.objsubid IS NOT DISTINCT FROM w.objsubid
		 AND h.pid <> w.pid
	`
	pairRows, err := db.Query(pairsQ)
	if err != nil {
		return nil, err
	}
	defer pairRows.Close()

	type pair struct{ blocked, blocking int }
	var pairs []pair
	pidSet := make(map[int]struct{})
	for pairRows.Next() {
		var bpid, spid int
		if err := pairRows.Scan(&bpid, &spid); err != nil {
			continue
		}
		pairs = append(pairs, pair{blocked: bpid, blocking: spid})
		pidSet[bpid] = struct{}{}
		pidSet[spid] = struct{}{}
	}
	if err := pairRows.Err(); err != nil {
		return nil, err
	}
	if len(pairs) == 0 {
		return []PgBlockingNode{}, nil
	}

	// Load pg_stat_activity rows for involved PIDs.
	var pidList []int
	for pid := range pidSet {
		pidList = append(pidList, pid)
	}

	// Build a dynamic IN clause safely.
	args := make([]any, 0, len(pidList))
	ph := make([]string, 0, len(pidList))
	for i, pid := range pidList {
		args = append(args, pid)
		ph = append(ph, fmt.Sprintf("$%d", i+1))
	}

	actQ := fmt.Sprintf(`
		SELECT
			a.pid,
			a.usename,
			a.datname,
			a.state,
			a.query_start,
			EXTRACT(EPOCH FROM (now() - a.state_change)) as duration_seconds,
			COALESCE(a.wait_event_type || ':' || a.wait_event, '') as wait_event,
			LEFT(a.query, 400) as query
		FROM pg_stat_activity a
		WHERE a.pid IN (%s)
	`, strings.Join(ph, ","))

	actRows, err := db.Query(actQ, args...)
	if err != nil {
		return nil, err
	}
	defer actRows.Close()

	sessionMap := make(map[int]PgBlockingNode)
	for actRows.Next() {
		var node PgBlockingNode
		var durationSeconds float64
		if err := actRows.Scan(&node.PID, &node.User, &node.Database, &node.State, &node.QueryStart, &durationSeconds, &node.WaitEvent, &node.Query); err != nil {
			continue
		}
		duration := time.Duration(durationSeconds) * time.Second
		minutes := int(duration.Minutes())
		seconds := int(duration.Seconds()) % 60
		node.Duration = fmt.Sprintf("%dm %ds", minutes, seconds)
		sessionMap[node.PID] = node
	}

	// Ensure we keep placeholders for PIDs we couldn't read (permissions/race).
	for pid := range pidSet {
		if _, ok := sessionMap[pid]; ok {
			continue
		}
		sessionMap[pid] = PgBlockingNode{
			PID:       pid,
			State:     "unknown",
			Duration:  "-",
			WaitEvent: "",
			Query:     "",
		}
	}

	// Build parent->children map.
	roots := make(map[int]struct{})
	for _, p := range pairs {
		roots[p.blocked] = struct{}{}
	}
	for _, p := range pairs {
		blocker := sessionMap[p.blocking]
		blocked := sessionMap[p.blocked]
		blocker.BlockedBy = append(blocker.BlockedBy, blocked)
		sessionMap[p.blocking] = blocker
		delete(roots, p.blocked)
	}

	var out []PgBlockingNode
	for pid := range roots {
		out = append(out, sessionMap[pid])
	}
	return out, nil
}

// GetBlockingTreeFast returns a blocking tree using pg_blocking_pids(), which is the same signal used by
// the incident collector (monitor.pg_blocking_pairs). This is typically more consistent for "live" UI.
func (c *PgRepository) GetBlockingTreeFast(instanceName string) ([]PgBlockingNode, error) {
	c.mutex.RLock()
	db, ok := c.conns[instanceName]
	c.mutex.RUnlock()
	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}

	// Build pairs directly from pg_blocking_pids().
	type pair struct{ blocked, blocking int }
	var pairs []pair
	pidSet := make(map[int]struct{})

	rows, err := db.Query(`
		SELECT a.pid AS blocked_pid, unnest(pg_blocking_pids(a.pid)) AS blocking_pid
		FROM pg_stat_activity a
		WHERE cardinality(pg_blocking_pids(a.pid)) > 0
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var bpid, spid int
		if err := rows.Scan(&bpid, &spid); err != nil {
			continue
		}
		pairs = append(pairs, pair{blocked: bpid, blocking: spid})
		pidSet[bpid] = struct{}{}
		pidSet[spid] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(pairs) == 0 {
		return []PgBlockingNode{}, nil
	}

	// Load pg_stat_activity rows for involved PIDs.
	var pidList []int
	for pid := range pidSet {
		pidList = append(pidList, pid)
	}
	args := make([]any, 0, len(pidList))
	ph := make([]string, 0, len(pidList))
	for i, pid := range pidList {
		args = append(args, pid)
		ph = append(ph, fmt.Sprintf("$%d", i+1))
	}

	actQ := fmt.Sprintf(`
		SELECT
			a.pid,
			COALESCE(a.usename,''),
			COALESCE(a.datname,''),
			COALESCE(a.state,''),
			a.query_start,
			EXTRACT(EPOCH FROM (now() - a.state_change)) as duration_seconds,
			COALESCE(a.wait_event_type || ':' || a.wait_event, '') as wait_event,
			LEFT(a.query, 400) as query
		FROM pg_stat_activity a
		WHERE a.pid IN (%s)
	`, strings.Join(ph, ","))

	actRows, err := db.Query(actQ, args...)
	if err != nil {
		return nil, err
	}
	defer actRows.Close()

	sessionMap := make(map[int]PgBlockingNode)
	for actRows.Next() {
		var node PgBlockingNode
		var durationSeconds float64
		if err := actRows.Scan(&node.PID, &node.User, &node.Database, &node.State, &node.QueryStart, &durationSeconds, &node.WaitEvent, &node.Query); err != nil {
			continue
		}
		duration := time.Duration(durationSeconds) * time.Second
		minutes := int(duration.Minutes())
		seconds := int(duration.Seconds()) % 60
		node.Duration = fmt.Sprintf("%dm %ds", minutes, seconds)
		sessionMap[node.PID] = node
	}

	for pid := range pidSet {
		if _, ok := sessionMap[pid]; ok {
			continue
		}
		sessionMap[pid] = PgBlockingNode{
			PID:       pid,
			State:     "unknown",
			Duration:  "-",
			WaitEvent: "",
			Query:     "",
		}
	}

	// Determine roots: blockers that are not blocked.
	blockedSet := make(map[int]struct{})
	blockerSet := make(map[int]struct{})
	for _, p := range pairs {
		blockedSet[p.blocked] = struct{}{}
		blockerSet[p.blocking] = struct{}{}
	}
	var rootPIDs []int
	for pid := range blockerSet {
		if _, isBlocked := blockedSet[pid]; !isBlocked {
			rootPIDs = append(rootPIDs, pid)
		}
	}
	sort.Ints(rootPIDs)

	children := make(map[int][]int)
	for _, p := range pairs {
		children[p.blocking] = append(children[p.blocking], p.blocked)
	}

	var build func(pid int, seen map[int]struct{}) PgBlockingNode
	build = func(pid int, seen map[int]struct{}) PgBlockingNode {
		n := sessionMap[pid]
		if seen == nil {
			seen = map[int]struct{}{}
		}
		if _, ok := seen[pid]; ok {
			return n
		}
		seen2 := make(map[int]struct{}, len(seen)+1)
		for k := range seen {
			seen2[k] = struct{}{}
		}
		seen2[pid] = struct{}{}
		for _, ch := range children[pid] {
			n.BlockedBy = append(n.BlockedBy, build(ch, seen2))
		}
		return n
	}

	out := make([]PgBlockingNode, 0, len(rootPIDs))
	for _, pid := range rootPIDs {
		out = append(out, build(pid, map[int]struct{}{}))
	}
	return out, nil
}

// PgQueryStat represents query performance statistics from pg_stat_statements.
type PgQueryStat struct {
	QueryID         int64   `json:"query_id"`
	Query           string  `json:"query"`
	UserName        string  `json:"user,omitempty"`
	Calls           int64   `json:"calls"`
	TotalTime       float64 `json:"total_time"`
	MeanTime        float64 `json:"mean_time"`
	Rows            int64   `json:"rows"`
	TempBlksRead    int64   `json:"temp_blks_read"`
	TempBlksWritten int64   `json:"temp_blks_written"`
	BlkReadTime     float64 `json:"blk_read_time"`
	BlkWriteTime    float64 `json:"blk_write_time"`
	SharedBlksRead  int64   `json:"shared_blks_read,omitempty"`
	SharedBlksHit   int64   `json:"shared_blks_hit,omitempty"`
	WalBytes        int64   `json:"wal_bytes,omitempty"`
}

// GetQueryStats returns top queries by total execution time from pg_stat_statements.
// Stats are cumulative since the last pg_stat_statements_reset() (not a time window).
// Requires pg_stat_statements extension to be installed.
func (c *PgRepository) GetQueryStats(instanceName string) ([]PgQueryStat, error) {
	return c.GetQueryStatsWithLimit(instanceName, 50)
}

// GetQueryStatsWithLimit is like GetQueryStats but allows a higher LIMIT for Timescale snapshots (delta windows).
func (c *PgRepository) GetQueryStatsWithLimit(instanceName string, limit int) ([]PgQueryStat, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 5000 {
		limit = 5000
	}

	c.mutex.RLock()
	db, ok := c.conns[instanceName]
	c.mutex.RUnlock()

	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}

	// Check if pg_stat_statements is available
	var exists bool
	err := db.QueryRow("SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'pg_stat_statements')").Scan(&exists)
	if err != nil || !exists {
		return nil, fmt.Errorf("pg_stat_statements extension not available")
	}

	query := `SELECT 
			s.queryid,
			LEFT(s.query, 400) as query,
			COALESCE(r.rolname, '') as user_name,
			s.calls,
			s.total_exec_time,
			s.mean_exec_time,
			s.rows,
			s.temp_blks_read,
			s.temp_blks_written,
			s.blk_read_time,
			s.blk_write_time,
			s.shared_blks_read,
			s.shared_blks_hit,
			COALESCE(s.wal_bytes,0)
		FROM pg_stat_statements s
		LEFT JOIN pg_roles r ON r.oid = s.userid
		WHERE ` + buildPgStatStatementsFilters() + fmt.Sprintf(`
		ORDER BY s.total_exec_time DESC
		LIMIT %d
	`, limit)
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []PgQueryStat
	for rows.Next() {
		var s PgQueryStat
		err := rows.Scan(&s.QueryID, &s.Query, &s.UserName, &s.Calls, &s.TotalTime, &s.MeanTime, &s.Rows, &s.TempBlksRead, &s.TempBlksWritten, &s.BlkReadTime, &s.BlkWriteTime, &s.SharedBlksRead, &s.SharedBlksHit, &s.WalBytes)
		if err != nil {
			continue
		}
		stats = append(stats, s)
	}

	return stats, nil
}

// GetQueryStatsForSnapshot returns all matching pg_stat_statements rows (no LIMIT) for Timescale delta snapshots.
func (c *PgRepository) GetQueryStatsForSnapshot(instanceName string) ([]PgQueryStat, error) {
	c.mutex.RLock()
	db, ok := c.conns[instanceName]
	c.mutex.RUnlock()

	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}

	var exists bool
	err := db.QueryRow("SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'pg_stat_statements')").Scan(&exists)
	if err != nil || !exists {
		return nil, fmt.Errorf("pg_stat_statements extension not available")
	}

	query := `SELECT 
			s.queryid,
			LEFT(s.query, 400) as query,
			COALESCE(r.rolname, '') as user_name,
			s.calls,
			s.total_exec_time,
			s.mean_exec_time,
			s.rows,
			s.temp_blks_read,
			s.temp_blks_written,
			s.blk_read_time,
			s.blk_write_time,
			s.shared_blks_read,
			s.shared_blks_hit,
			COALESCE(s.wal_bytes,0)
		FROM pg_stat_statements s
		LEFT JOIN pg_roles r ON r.oid = s.userid
		WHERE ` + buildPgStatStatementsFilters() + `
		ORDER BY s.queryid
	`
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []PgQueryStat
	for rows.Next() {
		var s PgQueryStat
		err := rows.Scan(&s.QueryID, &s.Query, &s.UserName, &s.Calls, &s.TotalTime, &s.MeanTime, &s.Rows, &s.TempBlksRead, &s.TempBlksWritten, &s.BlkReadTime, &s.BlkWriteTime, &s.SharedBlksRead, &s.SharedBlksHit, &s.WalBytes)
		if err != nil {
			continue
		}
		stats = append(stats, s)
	}

	return stats, rows.Err()
}

// PgTableStat represents table storage and vacuum statistics.
type PgTableStat struct {
	Schema      string  `json:"schema"`
	Table       string  `json:"table"`
	TotalSize   string  `json:"total_size"`
	DeadTuples  int64   `json:"dead_tuples"`
	BloatPct    float64 `json:"bloat_pct"`
	SeqScans    int64   `json:"seq_scans"`
	IdxScans    int64   `json:"idx_scans"`
	LastVacuum  *string `json:"last_vacuum"`
	LastAnalyze *string `json:"last_analyze"`
}

// GetTableStats returns table statistics including size, bloat, and vacuum information.
func (c *PgRepository) GetTableStats(instanceName string) ([]PgTableStat, error) {
	c.mutex.RLock()
	db, ok := c.conns[instanceName]
	c.mutex.RUnlock()

	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}

	query := `
		SELECT 
			schemaname,
			relname,
			pg_size_pretty(pg_total_relation_size(quote_ident(schemaname)||'.'||quote_ident(relname))) as total_size,
			n_dead_tup,
			CASE WHEN n_live_tup + n_dead_tup > 0 THEN (n_dead_tup::float / (n_live_tup + n_dead_tup)) * 100 ELSE 0 END as bloat_pct,
			seq_scan,
			idx_scan,
			last_vacuum,
			last_analyze
		FROM pg_stat_user_tables
		ORDER BY pg_total_relation_size(quote_ident(schemaname)||'.'||quote_ident(relname)) DESC
		LIMIT 20
	`
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []PgTableStat
	for rows.Next() {
		var s PgTableStat
		err := rows.Scan(&s.Schema, &s.Table, &s.TotalSize, &s.DeadTuples, &s.BloatPct, &s.SeqScans, &s.IdxScans, &s.LastVacuum, &s.LastAnalyze)
		if err != nil {
			continue
		}
		stats = append(stats, s)
	}

	return stats, nil
}

// PgIndexStat represents index usage statistics.
type PgIndexStat struct {
	IndexName string `json:"index_name"`
	TableName string `json:"table_name"`
	Size      string `json:"size"`
	Scans     int64  `json:"scans"`
	Reason    string `json:"reason"`
}

// GetIndexStats returns potentially unused indexes (idx_scan = 0).
func (c *PgRepository) GetIndexStats(instanceName string) ([]PgIndexStat, error) {
	c.mutex.RLock()
	db, ok := c.conns[instanceName]
	c.mutex.RUnlock()

	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}

	query := `
		SELECT 
			indexrelname,
			relname,
			pg_size_pretty(pg_relation_size(indexrelid)) as size,
			idx_scan,
			CASE 
				WHEN idx_scan = 0 THEN 'Unused'
				ELSE 'OK'
			END as reason
		FROM pg_stat_user_indexes
		WHERE idx_scan = 0
		ORDER BY pg_relation_size(indexrelid) DESC
		LIMIT 10
	`
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []PgIndexStat
	for rows.Next() {
		var s PgIndexStat
		err := rows.Scan(&s.IndexName, &s.TableName, &s.Size, &s.Scans, &s.Reason)
		if err != nil {
			continue
		}
		stats = append(stats, s)
	}

	return stats, nil
}

// GetReplicationStats returns detailed replication information including standby lag.
func (c *PgRepository) GetReplicationStats(instanceName string) (*models.PgReplicationStats, error) {
	c.mutex.RLock()
	db, ok := c.conns[instanceName]
	c.mutex.RUnlock()

	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}

	stats := &models.PgReplicationStats{}

	// Best-effort HA "cluster_name" signal (often set by operators).
	var clusterName string
	_ = db.QueryRow("SELECT COALESCE(setting,'') FROM pg_settings WHERE name='cluster_name'").Scan(&clusterName)
	var appNames []string

	// Step 1: Determine Node Role
	var isStandby bool
	err := db.QueryRow("SELECT pg_is_in_recovery() AS is_standby").Scan(&isStandby)
	if err != nil {
		return nil, fmt.Errorf("failed to determine node role: %w", err)
	}
	stats.IsPrimary = !isStandby

	if stats.IsPrimary {
		// Step 2 (Primary): Fetch Connected CNPG Replicas from pg_stat_replication
		stats.ClusterState = "primary"

		rows, err := db.Query(`
			SELECT 
				application_name AS replica_pod_name,
				client_addr AS pod_ip,
				state,
				sync_state,
				COALESCE(pg_wal_lsn_diff(pg_current_wal_lsn(), replay_lsn) / 1024.0 / 1024.0, 0) AS replay_lag_mb
			FROM pg_stat_replication
			ORDER BY application_name
		`)
		if err != nil {
			log.Printf("[POSTGRES] GetReplicationStats: pg_stat_replication query failed for %s: %v", instanceName, err)
			stats.Standbys = []models.PgReplicationStat{}
		} else {
			defer rows.Close()

			var maxLagMB float64
			for rows.Next() {
				var stat models.PgReplicationStat
				if err := rows.Scan(&stat.ReplicaPodName, &stat.PodIP, &stat.State, &stat.SyncState, &stat.ReplayLagMB); err != nil {
					log.Printf("[POSTGRES] GetReplicationStats: scan error for %s: %v", instanceName, err)
					continue
				}
				appNames = append(appNames, stat.ReplicaPodName)
				stats.Standbys = append(stats.Standbys, stat)
				if stat.ReplayLagMB > maxLagMB {
					maxLagMB = stat.ReplayLagMB
				}
			}
			stats.MaxLagMB = maxLagMB
			if stats.Standbys == nil {
				stats.Standbys = []models.PgReplicationStat{}
			}
		}
	} else {
		// Step 3 (Standby): Fetch Local Replay Lag
		stats.ClusterState = "standby"

		var localLagMB float64
		err = db.QueryRow(`
			SELECT 
				CASE 
					WHEN pg_last_wal_receive_lsn() = pg_last_wal_replay_lsn() THEN 0
					ELSE COALESCE(pg_wal_lsn_diff(pg_last_wal_receive_lsn(), pg_last_wal_replay_lsn()) / 1024.0 / 1024.0, 0)
				END AS local_replay_lag_mb
		`).Scan(&localLagMB)
		if err != nil {
			log.Printf("[POSTGRES] GetReplicationStats: local lag query failed for %s: %v", instanceName, err)
			stats.LocalLagMB = 0
		} else {
			stats.LocalLagMB = localLagMB
			stats.MaxLagMB = localLagMB
		}
		stats.Standbys = []models.PgReplicationStat{}
	}

	// clusterName/appNames are used by the API handler to determine HA provider.
	_ = clusterName
	_ = appNames

	// Get WAL generation rate (approximation)
	var walRate float64
	err = db.QueryRow(`
		SELECT 
			CASE 
				WHEN pg_is_in_recovery() = false AND pg_current_wal_lsn() IS NOT NULL 
				THEN 0
				ELSE 0
			END
	`).Scan(&walRate)
	if err == nil {
		stats.WalGenRateMBps = walRate
	}

	// Get BGWriter efficiency
	var buffersBackend, maxwrittenClean int64
	err = db.QueryRow("SELECT buffers_backend, maxwritten_clean FROM pg_stat_bgwriter").Scan(&buffersBackend, &maxwrittenClean)
	if err == nil && (buffersBackend+maxwrittenClean) > 0 {
		stats.BgWriterEffPct = float64(buffersBackend) / float64(buffersBackend+maxwrittenClean) * 100
	}

	return stats, nil
}

// GetConfig returns PostgreSQL configuration settings for key categories.
func (c *PgRepository) GetConfig(instanceName string) ([]models.PgConfigSetting, error) {
	c.mutex.RLock()
	db, ok := c.conns[instanceName]
	c.mutex.RUnlock()

	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}

	query := `
		SELECT
			name,
			setting,
			unit,
			category,
			source,
			boot_val,
			reset_val
		FROM pg_settings
		WHERE category IN ('Autovacuum', 'Client Connection Defaults', 'Connections and Authentication', 'Resource Usage', 'Write-Ahead Log')
		ORDER BY category, name
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var settings []models.PgConfigSetting
	for rows.Next() {
		var s models.PgConfigSetting
		err := rows.Scan(&s.Name, &s.Value, &s.Unit, &s.Category, &s.Source, &s.BootVal, &s.ResetVal)
		if err != nil {
			continue
		}
		settings = append(settings, s)
	}

	return settings, nil
}

// GetAlerts returns potential issues and alerts based on current metrics.
func (c *PgRepository) GetAlerts(instanceName string) ([]models.PgAlert, error) {
	c.mutex.RLock()
	db, ok := c.conns[instanceName]
	c.mutex.RUnlock()

	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}

	var alerts []models.PgAlert

	// Check for long-running idle-in-transaction queries
	longTxnQuery := `
		SELECT
			pid,
			EXTRACT(EPOCH FROM (now() - xact_start)) / 60 as duration_minutes,
			query
		FROM pg_stat_activity
		WHERE xact_start IS NOT NULL
		AND state = 'idle in transaction'
		AND EXTRACT(EPOCH FROM (now() - xact_start)) > 900
		ORDER BY xact_start
		LIMIT 5
	`

	rows, err := db.Query(longTxnQuery)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var pid int
			var duration float64
			var query string
			rows.Scan(&pid, &duration, &query)
			alerts = append(alerts, models.PgAlert{
				Severity:   "CRITICAL",
				Metric:     fmt.Sprintf("Idle in Transaction (PID %d)", pid),
				Threshold:  "> 15 mins",
				CurrentVal: fmt.Sprintf("%.0f mins", duration),
				Timestamp:  time.Now().Format("2006-01-02 15:04:05"),
				Status:     "ACTIVE",
			})
		}
	}

	// Check connection count threshold
	var connCount int
	db.QueryRow("SELECT count(*) FROM pg_stat_activity").Scan(&connCount)

	var maxConn int
	db.QueryRow("SELECT setting::int FROM pg_settings WHERE name = 'max_connections'").Scan(&maxConn)

	if maxConn > 0 && float64(connCount)/float64(maxConn) > 0.8 {
		alerts = append(alerts, models.PgAlert{
			Severity:   "WARNING",
			Metric:     "Connections HWM",
			Threshold:  fmt.Sprintf("> 80%% (%d)", int(float64(maxConn)*0.8)),
			CurrentVal: fmt.Sprintf("%d (%.0f%%)", connCount, float64(connCount)/float64(maxConn)*100),
			Timestamp:  time.Now().Format("2006-01-02 15:04:05"),
			Status:     "LOGGED",
		})
	}

	// Check replication lag on standby
	lag, status, err := c.GetReplicationLag(instanceName)
	if err == nil && status == "standby" && lag > 10 {
		alerts = append(alerts, models.PgAlert{
			Severity:   "CRITICAL",
			Metric:     "Replication Lag",
			Threshold:  "> 10 MB",
			CurrentVal: fmt.Sprintf("%.1f MB", lag),
			Timestamp:  time.Now().Format("2006-01-02 15:04:05"),
			Status:     "ACTIVE",
		})
	}

	// Check for tables with high bloat
	bloatQuery := `
		SELECT
			schemaname || '.' || tablename as table_name,
			n_dead_tup as dead_tuples,
			CASE WHEN n_live_tup > 0 THEN (n_dead_tup::float / n_live_tup) * 100 ELSE 0 END as bloat_pct
		FROM pg_stat_user_tables
		WHERE n_dead_tup > 1000
		ORDER BY n_dead_tup DESC
		LIMIT 3
	`

	bloatRows, err := db.Query(bloatQuery)
	if err == nil {
		defer bloatRows.Close()
		for bloatRows.Next() {
			var tableName string
			var deadTuples int
			var bloatPct float64
			bloatRows.Scan(&tableName, &deadTuples, &bloatPct)
			if bloatPct > 20 {
				alerts = append(alerts, models.PgAlert{
					Severity:   "WARNING",
					Metric:     fmt.Sprintf("Table Bloat (%s)", tableName),
					Threshold:  "> 20%",
					CurrentVal: fmt.Sprintf("%.1f%% (%d dead tuples)", bloatPct, deadTuples),
					Timestamp:  time.Now().Format("2006-01-02 15:04:05"),
					Status:     "LOGGED",
				})
			}
		}
	}

	return alerts, nil
}

// BGWriterStats represents PostgreSQL background writer metrics from pg_stat_bgwriter.
type BGWriterStats struct {
	CheckpointsTimed    int64   // Number of timed checkpoints
	CheckpointsReq      int64   // Number of requested checkpoints
	CheckpointWriteTime float64 // Time spent writing checkpoint files (ms)
	CheckpointSyncTime  float64 // Time spent syncing checkpoint files (ms)
	BuffersCheckpoint   int64   // Buffers written by checkpoints
	BuffersClean        int64   // Buffers written by background writer
	MaxwrittenClean     int64   // Times bgwriter stopped due to full buffers
	BuffersBackend      int64   // Buffers written directly by backends
	BuffersAlloc        int64   // Buffers allocated by backends
}

// FetchBGWriterStats retrieves background writer and checkpointer statistics from pg_stat_bgwriter.
// These metrics are collected and stored in TimescaleDB for historical analysis.
func (c *PgRepository) FetchBGWriterStats(instanceName string) (*BGWriterStats, error) {
	c.mutex.RLock()
	db, ok := c.conns[instanceName]
	c.mutex.RUnlock()

	if !ok || db == nil {
		log.Printf("[POSTGRES] FetchBGWriterStats: connection not found for %s, attempting reconnect", instanceName)
		if c.reconnectInstance(instanceName) {
			c.mutex.RLock()
			db, ok = c.conns[instanceName]
			c.mutex.RUnlock()
			if !ok || db == nil {
				return nil, fmt.Errorf("connection not found after reconnect")
			}
		} else {
			return nil, fmt.Errorf("connection not found")
		}
	}

	stats := &BGWriterStats{}
	query := `
		SELECT 
			checkpoints_timed,
			checkpoints_req,
			checkpoint_write_time,
			checkpoint_sync_time,
			buffers_checkpoint,
			buffers_clean,
			maxwritten_clean,
			buffers_backend,
			buffers_alloc
		FROM pg_stat_bgwriter
	`

	err := db.QueryRow(query).Scan(
		&stats.CheckpointsTimed,
		&stats.CheckpointsReq,
		&stats.CheckpointWriteTime,
		&stats.CheckpointSyncTime,
		&stats.BuffersCheckpoint,
		&stats.BuffersClean,
		&stats.MaxwrittenClean,
		&stats.BuffersBackend,
		&stats.BuffersAlloc,
	)

	if err != nil {
		return nil, err
	}

	return stats, nil
}

// ArchiverStats represents PostgreSQL WAL archiver metrics from pg_stat_archiver.
type ArchiverStats struct {
	ArchivedCount   int64          // Number of WAL files successfully archived
	FailedCount     int64          // Number of WAL file archive failures
	LastArchivedWal sql.NullString // Name of last successfully archived WAL
	LastFailedWal   sql.NullString // Name of last failed WAL file
}

// FetchArchiverStats retrieves WAL archiver statistics from pg_stat_archiver.
func (c *PgRepository) FetchArchiverStats(instanceName string) (*ArchiverStats, error) {
	c.mutex.RLock()
	db, ok := c.conns[instanceName]
	c.mutex.RUnlock()

	if !ok || db == nil {
		log.Printf("[POSTGRES] FetchArchiverStats: connection not found for %s, attempting reconnect", instanceName)
		if c.reconnectInstance(instanceName) {
			c.mutex.RLock()
			db, ok = c.conns[instanceName]
			c.mutex.RUnlock()
			if !ok || db == nil {
				return nil, fmt.Errorf("connection not found after reconnect")
			}
		} else {
			return nil, fmt.Errorf("connection not found")
		}
	}

	stats := &ArchiverStats{}
	query := `
		SELECT 
			archived_count,
			failed_count,
			last_archived_wal,
			last_failed_wal
		FROM pg_stat_archiver
	`

	err := db.QueryRow(query).Scan(
		&stats.ArchivedCount,
		&stats.FailedCount,
		&stats.LastArchivedWal,
		&stats.LastFailedWal,
	)

	if err != nil {
		return nil, err
	}

	return stats, nil
}

// NormalizedQueryStat represents normalized query statistics with query ID.
type NormalizedQueryStat struct {
	QueryID   int64
	QueryText string
	Calls     int64
	TotalTime float64
	MeanTime  float64
	Rows      int64
}

// FetchNormalizedQueryStats retrieves query statistics from pg_stat_statements.
// Returns query ID, text, and execution metrics. Requires pg_stat_statements extension.
func (c *PgRepository) FetchNormalizedQueryStats(instanceName string) ([]NormalizedQueryStat, error) {
	c.mutex.RLock()
	db, ok := c.conns[instanceName]
	c.mutex.RUnlock()

	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}

	// Check if pg_stat_statements is available
	var exists bool
	err := db.QueryRow("SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'pg_stat_statements')").Scan(&exists)
	if err != nil || !exists {
		return nil, fmt.Errorf("pg_stat_statements extension not available")
	}

	query := `SELECT 
			s.queryid,
			s.query,
			s.calls,
			s.total_exec_time,
			s.mean_exec_time,
			s.rows
		FROM pg_stat_statements s
		LEFT JOIN pg_roles r ON r.oid = s.userid
		WHERE ` + buildPgStatStatementsFilters() + `
		ORDER BY s.total_exec_time DESC
		LIMIT 100
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []NormalizedQueryStat
	for rows.Next() {
		var s NormalizedQueryStat
		if err := rows.Scan(&s.QueryID, &s.QueryText, &s.Calls, &s.TotalTime, &s.MeanTime, &s.Rows); err != nil {
			continue
		}
		stats = append(stats, s)
	}

	return stats, rows.Err()
}

// DBObservationMetrics holds critical DBA-focused health metrics for PostgreSQL
type DBObservationMetrics struct {
	XIDAge               int64   // max age(datfrozenxid) across user DBs
	XIDWraparoundPct     float64 // Percentage toward autovacuum_freeze_max_age (0-100+)
	IndexHitPct          float64 // Index lookup vs sequential scan ratio (0-100)
	IdleInTransactionCnt int     // Count of dangerous idle-in-transaction connections
	WALFails             int64   // WAL archive failures
	MaxTableBloatPct     float64 // Highest table bloat percentage
}

// FetchDBObservationMetrics retrieves critical DBA health metrics
func (c *PgRepository) FetchDBObservationMetrics(instanceName string) (*DBObservationMetrics, error) {
	metrics := &DBObservationMetrics{}

	c.mutex.RLock()
	db, ok := c.conns[instanceName]
	c.mutex.RUnlock()

	if !ok || db == nil {
		log.Printf("[POSTGRES] FetchDBObservationMetrics: connection not found for %s, attempting reconnect", instanceName)
		if c.reconnectInstance(instanceName) {
			c.mutex.RLock()
			db, ok = c.conns[instanceName]
			c.mutex.RUnlock()
			if !ok || db == nil {
				return nil, fmt.Errorf("connection not found after reconnect")
			}
		} else {
			return nil, fmt.Errorf("connection not found")
		}
	}

	// Query 1: XID Wraparound %
	var xidAge int64
	var xidPct float64
	err := db.QueryRow(`
		SELECT 
			COALESCE(MAX(age(datfrozenxid)), 0),
			COALESCE((MAX(age(datfrozenxid))::float / NULLIF(current_setting('autovacuum_freeze_max_age')::float, 0)) * 100, 0)
		FROM pg_database 
		WHERE datistemplate = false
	`).Scan(&xidAge, &xidPct)
	if err != nil {
		log.Printf("[POSTGRES] FetchDBObservationMetrics: XID query failed for %s: %v", instanceName, err)
	} else {
		metrics.XIDAge = xidAge
		metrics.XIDWraparoundPct = xidPct
	}

	// Query 2: Index Hit Rate
	err = db.QueryRow(`
		SELECT CASE WHEN (SUM(idx_tup_fetch) + SUM(seq_tup_read)) > 0 
			THEN (SUM(idx_tup_fetch)::float / NULLIF((SUM(idx_tup_fetch) + SUM(seq_tup_read)), 0)) * 100 
			ELSE 0 END
		FROM pg_stat_user_tables
	`).Scan(&metrics.IndexHitPct)
	if err != nil {
		log.Printf("[POSTGRES] FetchDBObservationMetrics: Index Hit Rate query failed for %s: %v", instanceName, err)
	}

	// Query 3: Idle in Transaction Count
	err = db.QueryRow(`
		SELECT COUNT(*) FROM pg_stat_activity
		WHERE state IN ('idle in transaction', 'idle in transaction (aborted)')
	`).Scan(&metrics.IdleInTransactionCnt)
	if err != nil {
		log.Printf("[POSTGRES] FetchDBObservationMetrics: Idle in Transaction query failed for %s: %v", instanceName, err)
	}

	// Get WAL failed count from ArchiverStats
	archiverStats, err := c.FetchArchiverStats(instanceName)
	if err == nil && archiverStats != nil {
		metrics.WALFails = archiverStats.FailedCount
	}

	// Get max table bloat from table stats
	tableStats, err := c.GetTableStats(instanceName)
	if err == nil && len(tableStats) > 0 {
		for _, t := range tableStats {
			if t.BloatPct > metrics.MaxTableBloatPct {
				metrics.MaxTableBloatPct = t.BloatPct
			}
		}
	}

	return metrics, nil
}

// TerminateSession kills a PostgreSQL backend process by PID.
func (c *PgRepository) TerminateSession(instanceName string, pid int) error {
	c.mutex.RLock()
	db, ok := c.conns[instanceName]
	c.mutex.RUnlock()

	if !ok || db == nil {
		return fmt.Errorf("connection not found")
	}

	_, err := db.Exec("SELECT pg_terminate_backend($1)", pid)
	if err != nil {
		return fmt.Errorf("failed to terminate session %d: %w", pid, err)
	}

	log.Printf("[POSTGRES] Terminated session PID %d on %s", pid, instanceName)
	return nil
}

// ResetQueryStats resets pg_stat_statements statistics.
func (c *PgRepository) ResetQueryStats(instanceName string) error {
	c.mutex.RLock()
	db, ok := c.conns[instanceName]
	c.mutex.RUnlock()

	if !ok || db == nil {
		return fmt.Errorf("connection not found")
	}

	_, err := db.Exec("SELECT pg_stat_statements_reset()")
	if err != nil {
		return fmt.Errorf("failed to reset query stats: %w", err)
	}

	log.Printf("[POSTGRES] Reset pg_stat_statements on %s", instanceName)
	return nil
}
