// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: SQL Server repository - core connection management and configuration.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"log/slog"
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/microsoft/go-mssqldb"
	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/sqlserver"
)

type SqlServerRepository struct {
	conns           map[string]*sql.DB
	serverIDToName  map[string]string
	status          map[string]string
	mutex           sync.RWMutex
	serverInfoCache map[string]CachedServerInfo

	perfCounterMu        sync.Mutex
	perfCounterSnapshots map[uuid.UUID]*perfCounterState

	LocalPool *pgxpool.Pool
}

type perfCounterState struct {
	values     map[string]int64
	capturedAt time.Time
}

type CachedServerInfo struct {
	Edition   string
	StartTime time.Time
}

type QueryState struct {
	Executions int64
	CPUMs      float64
}

func NewSqlServerRepository(cfg *config.Config) *SqlServerRepository {
	c := &SqlServerRepository{
		conns:           make(map[string]*sql.DB),
		serverIDToName:  make(map[string]string),
		status:          make(map[string]string),
		serverInfoCache: make(map[string]CachedServerInfo),
		perfCounterSnapshots: make(map[uuid.UUID]*perfCounterState),
	}

	for i, inst := range cfg.Instances {
		if inst.Type == "sqlserver" {
			port := inst.Port
			if port == 0 {
				port = 1433
			}

			user := inst.User
			password := inst.Password

			envPrefix := fmt.Sprintf("DB_%s", strings.ToUpper(strings.ReplaceAll(inst.Name, "-", "_")))
			if user == "" && !inst.IntegratedSecurity {
				user = os.Getenv(envPrefix + "_USER")
			}
			if password == "" && !inst.IntegratedSecurity {
				password = os.Getenv(envPrefix + "_PASSWORD")
			}

			catalog := strings.TrimSpace(inst.Database)
			if catalog == "" {
				catalog = "master"
			}
			encrypt := "true"
			if inst.Encrypt != nil && !*inst.Encrypt {
				encrypt = "false"
			}

			var connStr string
			if inst.IntegratedSecurity {
				msURL := &url.URL{
					Scheme: "sqlserver",
					Host:   net.JoinHostPort(inst.Host, fmt.Sprintf("%d", port)),
				}
				q := msURL.Query()
				q.Set("database", catalog)
				q.Set("integrated security", "true")
				q.Set("encrypt", encrypt)
				q.Set("app name", "sql-optima")
				if inst.TrustServerCertificate {
					q.Set("TrustServerCertificate", "true")
				} else {
					q.Set("TrustServerCertificate", "false")
				}
				msURL.RawQuery = q.Encode()
				connStr = msURL.String()
			} else {
				msURL := &url.URL{
					Scheme: "sqlserver",
					User:   url.UserPassword(user, password),
					Host:   net.JoinHostPort(inst.Host, fmt.Sprintf("%d", port)),
				}
				q := msURL.Query()
				q.Set("database", catalog)
				q.Set("encrypt", encrypt)
				q.Set("app name", "sql-optima")
				if inst.TrustServerCertificate {
					q.Set("TrustServerCertificate", "true")
				} else {
					q.Set("TrustServerCertificate", "false")
				}
				msURL.RawQuery = q.Encode()
				connStr = msURL.String()
			}

			db, err := sqlserver.OpenMetricsPool(connStr)
			if err != nil {
				c.status[inst.Name] = "offline"
				slog.Error("[SQLSERVER] DSN Parse Error", "target", inst.Name, "err", err)
				continue
			}

			db.SetMaxOpenConns(5)
			db.SetMaxIdleConns(2)
			db.SetConnMaxLifetime(time.Minute * 10)

			if err := db.Ping(); err != nil {
				c.status[strings.ToUpper(inst.Name)] = "offline"
				msg := err.Error()
				if strings.Contains(strings.ToLower(msg), "login") || strings.Contains(strings.ToLower(msg), "certificate") || strings.Contains(strings.ToLower(msg), "tls") {
					slog.Error("[SQLSERVER] Cannot connect to", "target", inst.Name, "err", err)
				} else {
					slog.Error("[SQLSERVER] Cannot connect to", "target", inst.Name, "err", err)
				}
				db.Close()
				continue // don't store a connection that can't authenticate
			}
			c.status[strings.ToUpper(inst.Name)] = "online"
			c.conns[strings.ToUpper(inst.Name)] = db
			c.serverIDToName[strings.ToUpper(inst.ServerID.String())] = strings.ToUpper(inst.Name)

			if len(inst.Databases) == 0 {
				query := "/* SQL_OPTIMA */ SELECT name FROM sys.databases WHERE database_id > 4 AND state_desc = 'ONLINE'"
				rows, err := db.Query(query)
				if err == nil {
					var discoverDbs []string
					for rows.Next() {
						var dbName string
						if err := rows.Scan(&dbName); err == nil {
							discoverDbs = append(discoverDbs, dbName)
						}
					}
					rows.Close()
					
					// Fallback: If no user databases found but we have a connection catalog, use it
					if len(discoverDbs) == 0 && catalog != "" && strings.ToLower(catalog) != "master" {
						discoverDbs = append(discoverDbs, catalog)
					}
					
					cfg.Instances[i].Databases = discoverDbs
				} else {
					slog.Error("[SQLSERVER] Dynamic Database Binding failure", "target", inst.Name, "err", err)
					// Fallback on query error
					if catalog != "" && strings.ToLower(catalog) != "master" {
						cfg.Instances[i].Databases = []string{catalog}
					}
				}
			}
		}
	}
	return c
}

func (c *SqlServerRepository) GetConn(instanceName string) (*sql.DB, bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	upperName := strings.ToUpper(instanceName)
	db, ok := c.conns[upperName]
	if !ok {
		// Try mapping from serverID
		if actualName, mapped := c.serverIDToName[upperName]; mapped {
			db, ok = c.conns[actualName]
		}
	}
	return db, ok
}

func (c *SqlServerRepository) GetServerID(instanceName string) (uuid.UUID, bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	upperName := strings.ToUpper(instanceName)
	for idStr, name := range c.serverIDToName {
		if name == upperName {
			id, err := uuid.Parse(idStr)
			if err == nil {
				return id, true
			}
		}
	}
	return uuid.Nil, false
}

func (c *SqlServerRepository) HasConnection(instanceName string) bool {
	c.mutex.RLock()
	_, ok := c.conns[strings.ToUpper(instanceName)]
	c.mutex.RUnlock()
	return ok
}

func (c *SqlServerRepository) AsQueryer(instanceName, dbName string) Queryer {
	db, ok := c.GetConn(instanceName)
	if !ok {
		return nil
	}
	return &dbWrapper{db: db, dbName: dbName}
}

type dbWrapper struct {
	db     *sql.DB
	dbName string
}

func (w *dbWrapper) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	q := fmt.Sprintf("USE [%s]; %s", strings.ReplaceAll(w.dbName, "]", "]]"), query)
	return w.db.ExecContext(ctx, q, args...)
}

func (w *dbWrapper) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	q := fmt.Sprintf("USE [%s]; %s", strings.ReplaceAll(w.dbName, "]", "]]"), query)
	return w.db.QueryContext(ctx, q, args...)
}

func (c *SqlServerRepository) PingAll() {
	var wg sync.WaitGroup
	for name, db := range c.conns {
		wg.Add(1)
		go func(n string, connection *sql.DB) {
			defer wg.Done()
			err := connection.Ping()
			c.mutex.Lock()
			if err != nil {
				c.status[n] = "offline"
				slog.Warn("[SQLSERVER] Handshake warning to", "target", n, "err", err)
			} else {
				c.status[n] = "online"
				slog.Info("[SQLSERVER] Success with", "val", n)
			}
			c.mutex.Unlock()
		}(name, db)
	}
	wg.Wait()
}

// GetConfigFreq retrieves the execution frequency for a specific collector from optima_collector_configs.
func (c *SqlServerRepository) GetConfigFreq(name string) int {
	var freq int
	query := "SELECT frequency_seconds FROM optima_collector_configs WHERE collector_name = $1 AND is_active = TRUE"
	// Use the local TimescaleDB pool for config lookups
	ctx, cancel := WithQueryTimeout(context.Background(), 0)
	defer cancel()
	err := c.LocalPool.QueryRow(ctx, query, name).Scan(&freq)
	if err != nil {
		return 60 // Default fallback
	}
	return freq
}
