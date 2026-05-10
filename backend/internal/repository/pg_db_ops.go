// Package repository provides data access layer for database operations.
// It handles connections and queries for both PostgreSQL and SQL Server databases.
// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL database-specific operations and discovery.
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
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/rsharma155/sql_optima/internal/config"
)

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

	ctdbURL := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(user, password),
		Host:   net.JoinHostPort(inst.Host, fmt.Sprintf("%d", port)),
		Path:   dbName,
	}
	ctdbQ := ctdbURL.Query()
	ctdbQ.Set("sslmode", sslmode)
	if inst.SSLCert != "" && inst.SSLKey != "" {
		ctdbQ.Set("sslcert", inst.SSLCert)
		ctdbQ.Set("sslkey", inst.SSLKey)
	}
	if inst.SSLRootCert != "" {
		ctdbQ.Set("sslrootcert", inst.SSLRootCert)
	}
	ctdbURL.RawQuery = ctdbQ.Encode()

	db, err := sql.Open("postgres", ctdbURL.String())
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

// GetDatabases returns the list of user databases (excludes template databases and 'postgres').
func (c *PgRepository) GetDatabases(instanceName string) ([]string, error) {
	c.mutex.RLock()
	db, ok := c.conns[strings.ToUpper(instanceName)]
	c.mutex.RUnlock()

	if !ok || db == nil {
		log.Printf("[POSTGRES] GetDatabases: connection not found for %s, attempting reconnect", instanceName)
		if c.reconnectInstance(instanceName) {
			c.mutex.RLock()
			db, ok = c.conns[strings.ToUpper(instanceName)]
			c.mutex.RUnlock()
			if !ok || db == nil {
				return nil, fmt.Errorf("connection not found after reconnect")
			}
		} else {
			return nil, fmt.Errorf("connection not found")
		}
	}

	// List all non-template databases including postgres (needed for CNPG)
	query := "SELECT /* SQL_OPTIMA */   datname FROM pg_database WHERE datistemplate = false AND datallowconn = true ORDER BY datname"
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
		if name := strings.TrimSpace(dbName); name != "" {
			databases = append(databases, name)
		}
	}

	log.Printf("[POSTGRES] GetDatabases: found %d databases for %s: %v", len(databases), instanceName, databases)
	return databases, nil
}
