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

/*
func init() {
	// Explicitly register the pgx driver as "postgres"
	sql.Register("postgres", stdlib.GetDefaultDriver())
	log.Println("[POSTGRES] Driver explicitly registered")
}
*/

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
}

// GetConn returns a live PostgreSQL connection for an instance name.
func (c *PgRepository) GetConn(instanceName string) (*sql.DB, bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	db, ok := c.conns[instanceName]
	return db, ok
}

// GetConnForDB returns a connection to a specific database on an instance.
func (c *PgRepository) GetConnForDB(instanceName, dbName string) (*sql.DB, error) {
	key := instanceName + "/" + dbName
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
	if db, ok := c.pgConns[key]; ok && db != nil {
		if err := db.Ping(); err == nil {
			return db, nil
		}
	}

	var inst *config.Instance
	for i := range c.cfg.Instances {
		if c.cfg.Instances[i].Name == instanceName {
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
	q.Set("sslmode", "disable") // keep it simple for now, or inherit from inst
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
	_, ok := c.conns[instanceName]
	c.mutex.RUnlock()
	return ok
}

// NewPgRepository creates a new PostgreSQL repository and initializes connections to all configured instances.
// It supports environment variable overrides for credentials and automatic database discovery.
func NewPgRepository(cfg *config.Config) *PgRepository {
	c := &PgRepository{
		conns:           make(map[string]*sql.DB),
		pgConns:         make(map[string]*sql.DB),
		status:          make(map[string]string),
		cfg:             cfg,
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

			dbname := strings.TrimSpace(inst.Database)
			if dbname == "" {
				dbname = "postgres"
			}

			// Build connection URL so special characters in username/password are
			// safely percent-encoded (prevents DSN injection via ; = @ / etc.).
			pgURL := &url.URL{
				Scheme: "postgres",
				User:   url.UserPassword(user, password),
				Host:   net.JoinHostPort(inst.Host, fmt.Sprintf("%d", port)),
				Path:   dbname,
			}
			q := pgURL.Query()
			q.Set("sslmode", sslmode)
			if inst.SSLCert != "" && inst.SSLKey != "" {
				q.Set("sslcert", inst.SSLCert)
				q.Set("sslkey", inst.SSLKey)
			}
			if inst.SSLRootCert != "" {
				q.Set("sslrootcert", inst.SSLRootCert)
			}
			pgURL.RawQuery = q.Encode()
			connStr := pgURL.String()

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
			c.status[inst.Name] = "online"
			log.Printf("[POSTGRES] DEBUG: Added connection to pool for %s, total: %d", inst.Name, len(c.conns))

			// Auto-discover databases if not configured
			if len(inst.Databases) == 0 {
				go func(instName string, db *sql.DB, idx int) {
					defer func() {
						if r := recover(); r != nil {
							log.Printf("[POSTGRES] Panic during database discovery for %s: %v", instName, r)
						}
					}()

					query := "SELECT /* SQL_OPTIMA */   datname FROM pg_database WHERE datistemplate = false AND datname NOT IN ('postgres')"
					rows, err := db.Query(query)
					if err == nil {
						var discoverDbs []string
						for rows.Next() {
							var dbName string
							if err := rows.Scan(&dbName); err != nil {
								log.Printf("[POSTGRES] Failed to scan discovered database row for %s: %v", instName, err)
								continue
							}
							discoverDbs = append(discoverDbs, dbName)
						}
						if err := rows.Err(); err != nil {
							log.Printf("[POSTGRES] Error during database discovery iteration for %s: %v", instName, err)
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

	dbname := strings.TrimSpace(inst.Database)
	if dbname == "" {
		dbname = "postgres"
	}

	rcURL := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(user, password),
		Host:   net.JoinHostPort(inst.Host, fmt.Sprintf("%d", port)),
		Path:   dbname,
	}
	rcQ := rcURL.Query()
	rcQ.Set("sslmode", sslmode)
	if inst.SSLCert != "" && inst.SSLKey != "" {
		rcQ.Set("sslcert", inst.SSLCert)
		rcQ.Set("sslkey", inst.SSLKey)
	}
	if inst.SSLRootCert != "" {
		rcQ.Set("sslrootcert", inst.SSLRootCert)
	}
	rcURL.RawQuery = rcQ.Encode()

	newDb, err := sql.Open("postgres", rcURL.String())
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
