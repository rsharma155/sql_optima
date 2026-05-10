// Package repository provides data access layer for database operations.
// It handles connections and queries for both PostgreSQL and SQL Server databases.
// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Core PostgreSQL repository for connection management and pool handling.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"database/sql"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/rsharma155/sql_optima/internal/config"
)

// PgRepository manages PostgreSQL database connections and provides methods for querying metrics.
// It supports connection pooling, automatic database discovery, and thread-safe operations.
type PgRepository struct {
	conns   map[string]*sql.DB // Connection pool per instance (default DB)
	pgConns map[string]*sql.DB // Cache for per-database connections: "instanceName/dbName" -> *sql.DB
	status  map[string]string  // Instance status: "online", "offline", "error"
	mutex   sync.RWMutex       // Thread-safe access to connections
	cfg     *config.Config     // Application configuration

	// Lightweight in-memory cache for size deltas (growth estimation).
	lastDbSizeBytes map[string]int64
	lastDbSizeAt    map[string]time.Time

	// previousSnapshots stores the last observed metric values for delta computation.
	previousSnapshots map[string]map[int64]PgQueryStat
	// pgssSupported caches whether pg_stat_statements is available on the instance.
	pgssSupported map[string]bool
	// pgVersion caches the server version number.
	pgVersion map[string]int
}

// GetPgVersion returns the cached server version or fetches it if not cached.
func (c *PgRepository) GetPgVersion(instanceName string) int {
	upperName := strings.ToUpper(instanceName)
	c.mutex.RLock()
	version, ok := c.pgVersion[upperName]
	c.mutex.RUnlock()
	if ok {
		return version
	}

	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return 0
	}

	var v int
	err := db.QueryRow("SELECT current_setting('server_version_num')::integer").Scan(&v)
	if err != nil {
		return 0
	}

	if v > 0 {
		c.mutex.Lock()
		c.pgVersion[upperName] = v
		c.mutex.Unlock()
	}
	return v
}

// GetPgssSupported returns true if the instance supports pg_stat_statements.
func (c *PgRepository) GetPgssSupported(instanceName string) bool {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.pgssSupported[strings.ToUpper(instanceName)]
}

// GetPreviousSnapshot returns the last observed stat for a query.
func (c *PgRepository) GetPreviousSnapshot(instanceName string, queryID int64) (PgQueryStat, bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	if m, ok := c.previousSnapshots[strings.ToUpper(instanceName)]; ok {
		s, ok := m[queryID]
		return s, ok
	}
	return PgQueryStat{}, false
}

// UpdatePreviousSnapshot updates the last observed stat for a query.
func (c *PgRepository) UpdatePreviousSnapshot(instanceName string, queryID int64, stat PgQueryStat) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	upperName := strings.ToUpper(instanceName)
	if c.previousSnapshots[upperName] == nil {
		c.previousSnapshots[upperName] = make(map[int64]PgQueryStat)
	}
	c.previousSnapshots[upperName][queryID] = stat
}

// ClearPreviousSnapshots clears the snapshot cache for an instance (e.g. after reset).
func (c *PgRepository) ClearPreviousSnapshots(instanceName string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	delete(c.previousSnapshots, strings.ToUpper(instanceName))
}

// GetConn returns a live PostgreSQL connection for an instance name.
func (c *PgRepository) GetConn(instanceName string) (*sql.DB, bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	db, ok := c.conns[strings.ToUpper(instanceName)]
	return db, ok
}

// GetConnForDB returns a connection to a specific database on an instance.
func (c *PgRepository) GetConnForDB(instanceName, dbName string) (*sql.DB, error) {
	key := strings.ToUpper(instanceName) + "/" + dbName
	c.mutex.RLock()
	db, ok := c.pgConns[key]
	c.mutex.RUnlock()
	if ok && db != nil {
		if err := db.Ping(); err == nil {
			return db, nil
		}
	}

	// Not found or dead, create new
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Double check after lock
	if db, ok = c.pgConns[key]; ok && db != nil {
		if err := db.Ping(); err == nil {
			return db, nil
		}
	}

	var inst *config.Instance
	upperName := strings.ToUpper(instanceName)
	for i := range c.cfg.Instances {
		if strings.ToUpper(c.cfg.Instances[i].Name) == upperName {
			inst = &c.cfg.Instances[i]
			break
		}
	}
	if inst == nil {
		return nil, fmt.Errorf("instance %s not found in config", instanceName)
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

	pgURL := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(user, password),
		Host:   net.JoinHostPort(inst.Host, fmt.Sprintf("%d", port)),
		Path:   dbName,
	}
	q := pgURL.Query()
	q.Set("sslmode", "disable")
	pgURL.RawQuery = q.Encode()

	newDb, err := sql.Open("postgres", pgURL.String())
	if err != nil {
		return nil, err
	}
	newDb.SetMaxOpenConns(2)
	newDb.SetMaxIdleConns(1)
	newDb.SetConnMaxLifetime(time.Minute * 5)

	if err := newDb.Ping(); err != nil {
		newDb.Close()
		return nil, err
	}

	c.pgConns[key] = newDb
	return newDb, nil
}

// HasConnection returns true if the instance has an active connection in the pool.
func (c *PgRepository) HasConnection(instanceName string) bool {
	c.mutex.RLock()
	_, ok := c.conns[strings.ToUpper(instanceName)]
	c.mutex.RUnlock()
	return ok
}

// NewPgRepository creates a new PostgreSQL repository and initializes connections to all configured instances.
func NewPgRepository(cfg *config.Config) *PgRepository {
	c := &PgRepository{
		conns:             make(map[string]*sql.DB),
		pgConns:           make(map[string]*sql.DB),
		status:            make(map[string]string),
		cfg:               cfg,
		lastDbSizeBytes:   make(map[string]int64),
		lastDbSizeAt:      make(map[string]time.Time),
		previousSnapshots: make(map[string]map[int64]PgQueryStat),
		pgssSupported:     make(map[string]bool),
		pgVersion:         make(map[string]int),
	}

	for _, inst := range cfg.Instances {
		if inst.Type == "postgres" {
			upperName := strings.ToUpper(inst.Name)
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

			dbname := strings.TrimSpace(inst.Database)
			if dbname == "" {
				dbname = "postgres"
			}

			pgURL := &url.URL{
				Scheme: "postgres",
				User:   url.UserPassword(user, password),
				Host:   net.JoinHostPort(inst.Host, fmt.Sprintf("%d", port)),
				Path:   dbname,
			}
			q := pgURL.Query()
			q.Set("sslmode", sslmode)
			pgURL.RawQuery = q.Encode()
			connStr := pgURL.String()

			db, err := sql.Open("postgres", connStr)
			if err != nil {
				log.Printf("[POSTGRES] DSN Parse Error %s: %v", inst.Name, err)
				c.status[upperName] = "error"
				continue
			}

			if err := db.Ping(); err != nil {
				log.Printf("[POSTGRES] Connection Failed %s: %v", inst.Name, err)
				c.status[upperName] = "error"
				continue
			}

			log.Printf("[POSTGRES] Connected to %s (%s:%d)", inst.Name, inst.Host, port)

			db.SetMaxOpenConns(5)
			db.SetMaxIdleConns(2)
			db.SetConnMaxLifetime(time.Minute * 10)

			c.conns[upperName] = db
			c.status[upperName] = "online"

			// Fetch version and cache it
			_ = c.GetPgVersion(inst.Name)

			// Check pg_stat_statements support more robustly
			var exists bool
			err = db.QueryRow("SELECT /* SQL_OPTIMA */ EXISTS (SELECT 1 FROM pg_views WHERE viewname = 'pg_stat_statements') OR EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'pg_stat_statements')").Scan(&exists)
			if err == nil && exists {
				c.pgssSupported[upperName] = true
				log.Printf("[POSTGRES] pg_stat_statements supported on %s", inst.Name)
			}
		}
	}
	return c
}

// PingAll tests connectivity to all configured PostgreSQL instances concurrently.
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
			} else {
				c.status[n] = "online"
			}
			c.mutex.Unlock()
		}(name, db)
	}
	wg.Wait()
}

// reconnectInstance attempts to reestablish a connection to a specific PostgreSQL instance.
func (c *PgRepository) reconnectInstance(instanceName string) bool {
	if c.cfg == nil {
		return false
	}

	var inst config.Instance
	found := false
	upperName := strings.ToUpper(instanceName)
	for _, instance := range c.cfg.Instances {
		if strings.ToUpper(instance.Name) == upperName {
			inst = instance
			found = true
			break
		}
	}

	if !found {
		return false
	}

	c.mutex.Lock()
	if oldDb, ok := c.conns[upperName]; ok && oldDb != nil {
		oldDb.Close()
	}
	c.mutex.Unlock()

	port := inst.Port
	if port == 0 {
		port = 5432
	}

	user := inst.User
	password := inst.Password

	if user == "" {
		user = "postgres"
	}

	rcURL := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(user, password),
		Host:   net.JoinHostPort(inst.Host, fmt.Sprintf("%d", port)),
		Path:   "postgres",
	}
	newDb, err := sql.Open("postgres", rcURL.String())
	if err != nil {
		c.mutex.Lock()
		c.status[upperName] = "error"
		c.mutex.Unlock()
		return false
	}

	if err := newDb.Ping(); err != nil {
		newDb.Close()
		c.mutex.Lock()
		c.status[upperName] = "offline"
		c.mutex.Unlock()
		return false
	}

	c.mutex.Lock()
	c.conns[upperName] = newDb
	c.status[upperName] = "online"
	// Clear cached version so it's refetched
	delete(c.pgVersion, upperName)
	c.mutex.Unlock()

	// Refetch version
	_ = c.GetPgVersion(instanceName)

	c.mutex.Lock()
	var exists bool
	err = newDb.QueryRow("SELECT /* SQL_OPTIMA */ EXISTS (SELECT 1 FROM pg_views WHERE viewname = 'pg_stat_statements') OR EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'pg_stat_statements')").Scan(&exists)
	if err == nil && exists {
		c.pgssSupported[upperName] = true
	} else {
		delete(c.pgssSupported, upperName)
	}
	c.mutex.Unlock()

	return true
}

// GetInstanceStatus returns the current connection status of a PostgreSQL instance.
func (c *PgRepository) GetInstanceStatus(instanceName string) string {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	if status, exists := c.status[strings.ToUpper(instanceName)]; exists {
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
	upperName := strings.ToUpper(instanceName)
	c.mutex.RLock()
	db, ok := c.conns[upperName]
	c.mutex.RUnlock()

	if !ok || db == nil {
		c.mutex.Lock()
		c.status[upperName] = "error"
		c.mutex.Unlock()
		return
	}

	err := db.Ping()
	c.mutex.Lock()
	if err != nil {
		c.status[upperName] = "offline"
	} else {
		c.status[upperName] = "online"
	}
	c.mutex.Unlock()
}
