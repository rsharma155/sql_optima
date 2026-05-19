// Package config manages application configuration from YAML and environment variables.
// It supports both SQL Server and PostgreSQL databases with various authentication
// methods including integrated security, certificates, and environment variable-based credentials.
// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Main configuration loader managing database instances (SQL Server and PostgreSQL), credentials, SSL settings, and environment variable overrides.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package config

import (
	"database/sql"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func ConnectToInstance(inst Instance) (*sql.DB, error) {
	if inst.Type == "sqlserver" {
		port := inst.Port
		if port == 0 {
			port = 1433
		}
		catalog := inst.Database
		if catalog == "" {
			catalog = "master"
		}
		encrypt := "true"
		if inst.Encrypt != nil && !*inst.Encrypt {
			encrypt = "false"
		}

		msURL := &url.URL{
			Scheme: "sqlserver",
			User:   url.UserPassword(inst.User, inst.Password),
			Host:   net.JoinHostPort(inst.Host, fmt.Sprintf("%d", port)),
		}
		q := msURL.Query()
		q.Set("database", catalog)
		q.Set("encrypt", encrypt)
		if inst.TrustServerCertificate {
			q.Set("TrustServerCertificate", "true")
		}
		msURL.RawQuery = q.Encode()
		db, err := sql.Open("sqlserver", msURL.String())
		if err == nil {
			db.SetMaxOpenConns(5)
			db.SetMaxIdleConns(2)
			db.SetConnMaxLifetime(10 * time.Minute)
		}
		return db, err
	} else if inst.Type == "postgres" {
		port := inst.Port
		if port == 0 {
			port = 5432
		}
		dbname := inst.Database
		if dbname == "" {
			dbname = "postgres"
		}
		sslmode := inst.SSLMode
		if sslmode == "" {
			sslmode = "disable"
		}

		pgURL := &url.URL{
			Scheme: "postgres",
			User:   url.UserPassword(inst.User, inst.Password),
			Host:   net.JoinHostPort(inst.Host, fmt.Sprintf("%d", port)),
			Path:   dbname,
		}
		q := pgURL.Query()
		q.Set("sslmode", sslmode)
		pgURL.RawQuery = q.Encode()
		db, err := sql.Open("postgres", pgURL.String())
		if err == nil {
			db.SetMaxOpenConns(5)
			db.SetMaxIdleConns(2)
			db.SetConnMaxLifetime(10 * time.Minute)
		}
		return db, err
	}
	return nil, fmt.Errorf("unsupported instance type: %s", inst.Type)
}

type Config struct {
	Instances    []Instance      `yaml:"instances" json:"instances"`
	FeatureFlags map[string]bool `yaml:"feature_flags" json:"feature_flags"`
}

type Instance struct {
	ServerID  uuid.UUID `yaml:"server_id,omitempty" json:"server_id,omitempty"`
	ID        int       `yaml:"id,omitempty" json:"id,omitempty"`
	Name      string    `yaml:"name" json:"name"`
	Type      string    `yaml:"type" json:"type"` // "sqlserver" or "postgres"
	Host      string    `yaml:"host" json:"host"`
	Port      int       `yaml:"port,omitempty" json:"port,omitempty"`
	User      string    `yaml:"user,omitempty" json:"-"`
	Password  string    `yaml:"password,omitempty" json:"-"`
	Databases []string  `yaml:"databases,omitempty" json:"databases,omitempty"`
	Available bool      `yaml:"available,omitempty" json:"available,omitempty"`

	// Security Additions
	TrustServerCertificate bool   `yaml:"trust_server_certificate,omitempty" json:"trust_server_certificate,omitempty"` // SQLSERVER (Azure SQL / MI when CA validation is impractical)
	IntegratedSecurity     bool   `yaml:"integrated_security,omitempty" json:"integrated_security,omitempty"`           // SQLSERVER AD Auth
	Encrypt                *bool  `yaml:"encrypt,omitempty" json:"encrypt,omitempty"`                                   // SQLSERVER: encrypt connections (default: true)
	SSLMode                string `yaml:"sslmode,omitempty" json:"sslmode,omitempty"`                                   // PG
	// Initial catalog: PostgreSQL dbname (default postgres), SQL Server database (default master). Used for RDS / Azure endpoints.
	Database    string `yaml:"database,omitempty" json:"database,omitempty"`
	SSLCert     string `yaml:"sslcert,omitempty" json:"sslcert,omitempty"`             // PG Passwordless Certificate
	SSLKey      string `yaml:"sslkey,omitempty" json:"sslkey,omitempty"`               // PG Passwordless Key
	SSLRootCert string `yaml:"sslrootcert,omitempty" json:"sslrootcertcert,omitempty"` // PG Root CA

	// Postgres HA provider hint (optional): cnpg | patroni | streaming | auto
	HAProvider string `yaml:"ha_provider,omitempty" json:"ha_provider,omitempty"`

	// Optional local disk paths (only works if the monitoring server has access to these paths).
	// Example: pg_disk_paths: { data: "/var/lib/postgresql/data", wal: "/var/lib/postgresql/wal" }
	PGDiskPaths map[string]string `yaml:"pg_disk_paths,omitempty" json:"pg_disk_paths,omitempty"`

	// Optional PgBouncer admin DSN for pooler monitoring (example: "postgres://user:pass@host:6432/pgbouncer?sslmode=disable").
	// Keep this server-side only (not exposed via /api/config).
	PGBouncerAdminDSN string `yaml:"pgbouncer_admin_dsn,omitempty" json:"-"`

	// QueryTimeoutSeconds overrides the default 20s timeout for database queries.
	QueryTimeoutSeconds int `yaml:"query_timeout_seconds,omitempty" json:"query_timeout_seconds,omitempty"`


}

// QueryTimeout returns the configured query timeout or 0 (use default).
func (i Instance) QueryTimeout() time.Duration {
	if i.QueryTimeoutSeconds > 0 {
		return time.Duration(i.QueryTimeoutSeconds) * time.Second
	}
	return 0
}

func LoadConfig(path string) (*Config, error) {
	return LoadConfigWithSecurity(path, LoadSecurity())
}

// LoadConfigWithSecurity loads config and optionally strips YAML passwords so only env credentials apply.
// If the file does not exist, an empty config is returned (all instances will come from the server registry).
func LoadConfigWithSecurity(path string, sec Security) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("[config] %s not found — starting with empty instance list (use Admin UI or API to register targets)\n", path)
			return &Config{Instances: []Instance{}}, nil
		}
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	var validInstances []Instance
	for i := range cfg.Instances {
		inst := &cfg.Instances[i]
		inst.ID = i + 1

		if sec.DisallowYAMLPasswords {
			inst.Password = ""
		}

		// Build env var name: DB_<INSTANCE_NAME>_USER/PASSWORD
		envPrefix := fmt.Sprintf("DB_%s",
			strings.ToUpper(strings.ReplaceAll(inst.Name, "-", "_")))

		if inst.User == "" {
			inst.User = os.Getenv(envPrefix + "_USER")
		}
		if inst.Password == "" {
			inst.Password = os.Getenv(envPrefix + "_PASSWORD")
		}

		// Skip instances without credentials instead of failing entirely
		if inst.User == "" || inst.Password == "" {
			// Do not print environment variable names or other system details to stdout.
			// Keep the signal, but avoid leaking local env naming conventions in logs.
			fmt.Printf("[config] skipping instance %s: missing credentials\n", inst.Name)
			continue
		}

		validInstances = append(validInstances, *inst)
	}

	cfg.Instances = validInstances
	if cfg.Instances == nil {
		cfg.Instances = []Instance{}
	}

	return &cfg, nil
}

// PostgresInstanceByName returns a PostgreSQL instance by its configured name (case-insensitive).
func (c *Config) PostgresInstanceByName(name string) (Instance, bool) {
	if c == nil {
		return Instance{}, false
	}
	want := strings.TrimSpace(name)
	if want == "" {
		return Instance{}, false
	}
	for _, inst := range c.Instances {
		if strings.ToLower(inst.Type) != "postgres" {
			continue
		}
		if strings.EqualFold(inst.Name, want) {
			return inst, true
		}
	}
	return Instance{}, false
}

// DefaultPostgresInstance returns the sole PostgreSQL instance when exactly one is configured.
func (c *Config) DefaultPostgresInstance() (Instance, bool) {
	if c == nil {
		return Instance{}, false
	}
	var found []Instance
	for _, inst := range c.Instances {
		if strings.ToLower(inst.Type) == "postgres" {
			found = append(found, inst)
		}
	}
	if len(found) == 1 {
		return found[0], true
	}
	return Instance{}, false
}
