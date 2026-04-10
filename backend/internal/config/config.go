// Package config manages application configuration from YAML and environment variables.
// It supports both SQL Server and PostgreSQL databases with various authentication
// methods including integrated security, certificates, and environment variable-based credentials.
package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Instances []Instance `yaml:"instances" json:"instances"`
}

type Instance struct {
	ID        int      `yaml:"id,omitempty" json:"id,omitempty"`
	Name      string   `yaml:"name" json:"name"`
	Type      string   `yaml:"type" json:"type"` // "sqlserver" or "postgres"
	Host      string   `yaml:"host" json:"host"`
	Port      int      `yaml:"port,omitempty" json:"port,omitempty"`
	User      string   `yaml:"user,omitempty" json:"-"`
	Password  string   `yaml:"password,omitempty" json:"-"`
	Databases []string `yaml:"databases,omitempty" json:"databases,omitempty"`

	// Security Additions
	TrustServerCertificate bool   `yaml:"trust_server_certificate,omitempty" json:"trust_server_certificate,omitempty"` // MSSQL
	IntegratedSecurity     bool   `yaml:"integrated_security,omitempty" json:"integrated_security,omitempty"`           // MSSQL AD Auth
	SSLMode                string `yaml:"sslmode,omitempty" json:"sslmode,omitempty"`                                   // PG
	SSLCert                string `yaml:"sslcert,omitempty" json:"sslcert,omitempty"`                                   // PG Passwordless Certificate
	SSLKey                 string `yaml:"sslkey,omitempty" json:"sslkey,omitempty"`                                     // PG Passwordless Key
	SSLRootCert            string `yaml:"sslrootcert,omitempty" json:"sslrootcertcert,omitempty"`                       // PG Root CA

	// Postgres HA provider hint (optional): cnpg | patroni | streaming | auto
	HAProvider string `yaml:"ha_provider,omitempty" json:"ha_provider,omitempty"`

	// Optional local disk paths (only works if the monitoring server has access to these paths).
	// Example: pg_disk_paths: { data: "/var/lib/postgresql/data", wal: "/var/lib/postgresql/wal" }
	PGDiskPaths map[string]string `yaml:"pg_disk_paths,omitempty" json:"pg_disk_paths,omitempty"`

	// Optional PgBouncer admin DSN for pooler monitoring (example: "postgres://user:pass@host:6432/pgbouncer?sslmode=disable").
	// Keep this server-side only (not exposed via /api/config).
	PGBouncerAdminDSN string `yaml:"pgbouncer_admin_dsn,omitempty" json:"-"`
}

func LoadConfig(path string) (*Config, error) {
	return LoadConfigWithSecurity(path, LoadSecurity())
}

// LoadConfigWithSecurity loads config and optionally strips YAML passwords so only env credentials apply.
func LoadConfigWithSecurity(path string, sec Security) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
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
			fmt.Printf("[config] skipping instance %s: missing %s_USER or %s_PASSWORD env vars\n",
				inst.Name, envPrefix, envPrefix)
			continue
		}

		validInstances = append(validInstances, *inst)
	}

	cfg.Instances = validInstances

	return &cfg, nil
}
