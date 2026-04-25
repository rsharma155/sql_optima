// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: SQL Server repository - core connection management and configuration.
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
	"sync"
	"time"

	_ "github.com/microsoft/go-mssqldb"
	"github.com/rsharma155/sql_optima/internal/collectors"
	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/sqlserver"
)

type SqlServerRepository struct {
	conns          map[string]*sql.DB
	status         map[string]string
	mutex          sync.RWMutex
	prevQueryCache map[string]map[string]QueryState
}

type QueryState struct {
	Executions int64
	CPUMs      float64
}

func NewSqlServerRepository(cfg *config.Config) *SqlServerRepository {
	c := &SqlServerRepository{
		conns:  make(map[string]*sql.DB),
		status: make(map[string]string),
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
				log.Printf("[SQLSERVER] DSN Parse Error %s: %v", inst.Name, err)
				continue
			}

			db.SetMaxOpenConns(5)
			db.SetMaxIdleConns(2)
			db.SetConnMaxLifetime(time.Minute * 10)

			if err := db.Ping(); err != nil {
				c.status[inst.Name] = "offline"
				log.Printf("[SQLSERVER] Connection ping failure %s: %v", inst.Name, err)
			} else {
				c.status[inst.Name] = "online"
			}

			c.conns[inst.Name] = db

			if len(inst.Databases) == 0 {
				query := "SELECT /* SQL_OPTIMA */   name FROM sys.databases WHERE database_id > 4 AND state_desc = 'ONLINE'"
				rows, err := db.Query(query)
				if err == nil {
					var discoverDbs []string
					for rows.Next() {
						var dbName string
						_ = rows.Scan(&dbName)
						discoverDbs = append(discoverDbs, dbName)
					}
					rows.Close()
					cfg.Instances[i].Databases = discoverDbs
				} else {
					log.Printf("[SQLSERVER] Dynamic Database Binding failure %s: %v", inst.Name, err)
				}
			}
		}
	}
	return c
}

func (c *SqlServerRepository) GetConn(instanceName string) (*sql.DB, bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	db, ok := c.conns[instanceName]
	return db, ok
}

func (c *SqlServerRepository) HasConnection(instanceName string) bool {
	c.mutex.RLock()
	_, ok := c.conns[instanceName]
	c.mutex.RUnlock()
	return ok
}

func (c *SqlServerRepository) AsQueryer(instanceName, dbName string) collectors.Queryer {
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
	q := fmt.Sprintf("USE [%s]; %s", strings.ReplaceAll(w.dbName, "]", "]]"), query)
	return w.db.ExecContext(ctx, q, args...)
}

func (w *dbWrapper) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
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
				log.Printf("[SQLSERVER] Handshake warning to %s: %v", n, err)
			} else {
				c.status[n] = "online"
				log.Printf("[SQLSERVER] Success with %s", n)
			}
			c.mutex.Unlock()
		}(name, db)
	}
	wg.Wait()
}
