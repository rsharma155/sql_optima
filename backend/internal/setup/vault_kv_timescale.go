// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Optional HashiCorp Vault KV v2 read for Timescale credentials during first-run setup.
package setup

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	vault "github.com/hashicorp/vault/api"
)

// TimescaleFromVault merges Vault KV data into connection fields when use_vault is enabled.
// logicalPath is the Vault API path (e.g. secret/data/monitoring/timescale) without the /v1/ prefix.
// Expected JSON keys (case-insensitive): host, port, database, username, password, ssl_mode (optional).
func TimescaleFromVault(ctx context.Context, logicalPath string) (host string, port int, database, username, password, sslMode string, err error) {
	addr := strings.TrimSpace(os.Getenv("VAULT_ADDR"))
	tok := strings.TrimSpace(os.Getenv("VAULT_TOKEN"))
	if addr == "" || tok == "" {
		return "", 0, "", "", "", "", fmt.Errorf("VAULT_ADDR and VAULT_TOKEN must be set on the API server to load Timescale credentials from Vault")
	}
	path := strings.TrimSpace(logicalPath)
	path = strings.TrimPrefix(path, "/")
	if path == "" {
		return "", 0, "", "", "", "", fmt.Errorf("vault secret path is required")
	}

	c, err := vault.NewClient(&vault.Config{Address: addr})
	if err != nil {
		return "", 0, "", "", "", "", fmt.Errorf("vault client: %w", err)
	}
	c.SetToken(tok)
	if ns := strings.TrimSpace(os.Getenv("VAULT_NAMESPACE")); ns != "" {
		c.SetNamespace(ns)
	}

	sec, err := c.Logical().ReadWithContext(ctx, path)
	if err != nil {
		return "", 0, "", "", "", "", fmt.Errorf("vault read: %w", err)
	}
	if sec == nil || sec.Data == nil {
		return "", 0, "", "", "", "", fmt.Errorf("vault returned empty secret at %q", path)
	}

	raw, ok := sec.Data["data"].(map[string]interface{})
	if !ok {
		return "", 0, "", "", "", "", fmt.Errorf("vault secret at %q is not KV v2 (missing data.data map)", path)
	}

	m := make(map[string]string)
	for k, v := range raw {
		m[strings.ToLower(strings.TrimSpace(k))] = fmt.Sprint(v)
	}
	get := func(keys ...string) string {
		for _, k := range keys {
			if v := strings.TrimSpace(m[strings.ToLower(k)]); v != "" {
				return v
			}
		}
		return ""
	}

	host = get("host", "hostname", "db_host")
	database = get("database", "dbname", "db_name", "name")
	username = get("username", "user", "db_user")
	password = get("password", "pass", "db_password")
	sslMode = get("ssl_mode", "sslmode")
	if sslMode == "" {
		sslMode = "require"
	}
	portStr := get("port", "db_port")
	if portStr != "" {
		if p, e := strconv.Atoi(portStr); e == nil && p > 0 {
			port = p
		}
	}
	if host == "" || database == "" || username == "" || password == "" || port <= 0 {
		return "", 0, "", "", "", "", fmt.Errorf("vault secret must include host, port, database, username, and password")
	}
	return host, port, database, username, password, sslMode, nil
}
