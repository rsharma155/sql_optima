// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Checks whether the monitoring database user has all required permissions
//
//	and returns targeted grant/create-user SQL scripts for missing ones.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/mux"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/rsharma155/sql_optima/internal/domain/servers"
	"github.com/rsharma155/sql_optima/internal/sqlserver"
)

// PermissionCheckResult is the outcome of a single permission probe.
type PermissionCheckResult struct {
	Name     string `json:"name"`
	OK       bool   `json:"ok"`
	Detail   string `json:"detail,omitempty"`
	Optional bool   `json:"optional,omitempty"`
}

// permissionsCheckResponse is returned by both check-permissions endpoints.
type permissionsCheckResponse struct {
	PermissionsOK    bool                    `json:"permissions_ok"`
	Checks           []PermissionCheckResult `json:"checks"`
	GrantScript      string                  `json:"grant_script,omitempty"`
	CreateUserScript string                  `json:"create_user_script,omitempty"`
}

// CheckPermissionsDraft accepts inline credentials (same body as test-draft) and probes
// all required monitoring permissions without persisting anything.
// POST /api/admin/servers/check-permissions-draft
func (h *AdminServerHandlers) CheckPermissionsDraft(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var req struct {
		DBType                 string `json:"db_type"`
		Host                   string `json:"host"`
		Port                   any    `json:"port"`
		Username               string `json:"username"`
		Password               string `json:"password"`
		SSLMode                string `json:"ssl_mode"`
		Database               string `json:"database"`
		TrustServerCertificate bool   `json:"trust_server_certificate"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
		return
	}
	port := parsePortAny(req.Port)
	s := servers.Server{
		DBType:   servers.DBType(strings.TrimSpace(req.DBType)),
		Host:     strings.TrimSpace(req.Host),
		Port:     port,
		Username: strings.TrimSpace(req.Username),
		SSLMode:  servers.SSLMode(strings.TrimSpace(req.SSLMode)),
	}
	cred := servers.CredentialPayload{
		Password:               req.Password,
		SSLMode:                strings.TrimSpace(req.SSLMode),
		Database:               strings.TrimSpace(req.Database),
		TrustServerCertificate: req.TrustServerCertificate,
	}
	defer zeroString(&cred.Password)

	result, err := runPermissionChecks(r.Context(), s, cred)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(result)
}

// CheckPermissions probes permissions for a saved server by ID.
// POST /api/admin/servers/{id}/check-permissions
func (h *AdminServerHandlers) CheckPermissions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	store, kms, box, _ := h.reg()
	if h == nil || store == nil || kms == nil || box == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "server registry not configured"})
		return
	}
	id := strings.TrimSpace(mux.Vars(r)["id"])
	if id == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "id is required"})
		return
	}

	s, encSecret, encDEK, err := store.GetEncrypted(r.Context(), id)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "server not found"})
		return
	}
	plaintextDEK, err := kms.DecryptDataKey(r.Context(), encDEK)
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "key management unavailable"})
		return
	}
	defer zeroBytes(plaintextDEK)

	plainJSON, err := box.Decrypt(encSecret, plaintextDEK)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "failed to decrypt credentials"})
		return
	}
	defer zeroBytes(plainJSON)

	var cred servers.CredentialPayload
	if err := json.Unmarshal(plainJSON, &cred); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "credential payload invalid"})
		return
	}
	if strings.TrimSpace(cred.Password) == "" {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "credential payload missing password"})
		return
	}
	defer zeroString(&cred.Password)

	result, err := runPermissionChecks(r.Context(), s, cred)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(result)
}

// runPermissionChecks dispatches to the DB-specific check logic.
func runPermissionChecks(ctx context.Context, s servers.Server, cred servers.CredentialPayload) (*permissionsCheckResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	switch s.DBType {
	case servers.DBPostgres:
		return checkPostgresPermissions(ctx, s, cred)
	case servers.DBSQLServer:
		return checkSQLServerPermissions(ctx, s, cred)
	default:
		return nil, errors.New("unsupported db_type")
	}
}

// ── PostgreSQL ────────────────────────────────────────────────────────────────

type pgProbe struct {
	name     string
	query    string // returns a single boolean row for role checks, or succeeds/fails for view checks
	isRole   bool   // true: query returns bool (pg_has_role); false: test that the query runs without error
	grant    string // SQL fragment to grant this permission
	optional bool   // optional: not included in permissions_ok
}

// pgProbes lists all permissions required for SQL Optima monitoring.
var pgProbes = []pgProbe{
	{
		name:   "Role: pg_monitor",
		query:  `SELECT pg_has_role(current_user, 'pg_monitor', 'member')`,
		isRole: true,
		grant:  `GRANT pg_monitor TO {user};`,
	},
	{
		name:   "Role: pg_read_all_settings",
		query:  `SELECT pg_has_role(current_user, 'pg_read_all_settings', 'member')`,
		isRole: true,
		grant:  `GRANT pg_read_all_settings TO {user};`,
	},
	{
		name:   "Role: pg_read_all_stats",
		query:  `SELECT pg_has_role(current_user, 'pg_read_all_stats', 'member')`,
		isRole: true,
		grant:  `GRANT pg_read_all_stats TO {user};`,
	},
	{
		name:   "Role: pg_stat_scan_tables",
		query:  `SELECT pg_has_role(current_user, 'pg_stat_scan_tables', 'member')`,
		isRole: true,
		grant:  `GRANT pg_stat_scan_tables TO {user};`,
	},
	{
		name:  "SELECT pg_stat_activity",
		query: `SELECT 1 FROM pg_stat_activity WHERE false`,
		grant: `GRANT SELECT ON pg_stat_activity TO {user};`,
	},
	{
		name:  "SELECT pg_stat_bgwriter",
		query: `SELECT 1 FROM pg_stat_bgwriter WHERE false`,
		grant: `GRANT SELECT ON pg_stat_bgwriter TO {user};`,
	},
	{
		name:  "SELECT pg_stat_database",
		query: `SELECT 1 FROM pg_stat_database WHERE false`,
		grant: `GRANT SELECT ON pg_stat_database TO {user};`,
	},
	{
		name:  "SELECT pg_locks",
		query: `SELECT 1 FROM pg_locks WHERE false`,
		grant: `GRANT SELECT ON pg_locks TO {user};`,
	},
	{
		name:  "SELECT pg_stat_replication",
		query: `SELECT 1 FROM pg_stat_replication WHERE false`,
		grant: `GRANT SELECT ON pg_stat_replication TO {user};`,
	},
	{
		name:  "SELECT pg_replication_slots",
		query: `SELECT 1 FROM pg_replication_slots WHERE false`,
		grant: `GRANT SELECT ON pg_replication_slots TO {user};`,
	},
	{
		name:  "SELECT pg_settings",
		query: `SELECT 1 FROM pg_settings WHERE false`,
		grant: `GRANT SELECT ON pg_settings TO {user};`,
	},
	{
		name:  "SELECT pg_stat_user_tables",
		query: `SELECT 1 FROM pg_stat_user_tables WHERE false`,
		grant: `GRANT SELECT ON pg_catalog.pg_stat_user_tables TO {user};`,
	},
	{
		name:  "SELECT pg_stat_user_indexes",
		query: `SELECT 1 FROM pg_stat_user_indexes WHERE false`,
		grant: `GRANT SELECT ON pg_catalog.pg_stat_user_indexes TO {user};`,
	},
	{
		name:     "SELECT pg_stat_statements (extension)",
		query:    `SELECT 1 FROM pg_stat_statements WHERE false`,
		grant:    `-- Requires pg_stat_statements extension to be installed first:\n-- CREATE EXTENSION IF NOT EXISTS pg_stat_statements;\nGRANT SELECT ON pg_stat_statements TO {user};`,
		optional: true,
	},
}

func checkPostgresPermissions(ctx context.Context, s servers.Server, cred servers.CredentialPayload) (*permissionsCheckResponse, error) {
	sslmode := postgresSSLMode(cred, s)
	dbname := postgresDBName(cred, s)

	pgURL := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(s.Username, cred.Password),
		Host:   net.JoinHostPort(s.Host, itoa(s.Port)),
		Path:   dbname,
	}
	q := pgURL.Query()
	q.Set("sslmode", sslmode)
	pgURL.RawQuery = q.Encode()

	db, err := sql.Open("postgres", pgURL.String())
	if err != nil {
		return nil, sanitizeDBError(err, "postgres")
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return nil, sanitizeDBError(err, "postgres")
	}

	var checks []PermissionCheckResult
	var missingGrants []string
	allOK := true

	for _, p := range pgProbes {
		result := PermissionCheckResult{Name: p.name, Optional: p.optional}

		if p.isRole {
			var hasRole bool
			scanErr := db.QueryRowContext(ctx, p.query).Scan(&hasRole)
			if scanErr != nil {
				// Role likely doesn't exist on this PostgreSQL version, treat as missing.
				result.OK = false
				result.Detail = "role not available on this server (may be unsupported PostgreSQL version)"
			} else if !hasRole {
				result.OK = false
				result.Detail = "role not granted to this user"
			} else {
				result.OK = true
			}
		} else {
			rows, queryErr := db.QueryContext(ctx, p.query)
			if queryErr != nil {
				result.OK = false
				result.Detail = "permission denied"
			} else {
				rows.Close()
				result.OK = true
			}
		}

		if !result.OK {
			if !p.optional {
				allOK = false
			}
			grant := strings.ReplaceAll(p.grant, "{user}", pgQuoteIdent(s.Username))
			missingGrants = append(missingGrants, grant)
		}
		checks = append(checks, result)
	}

	resp := &permissionsCheckResponse{
		PermissionsOK:    allOK,
		Checks:           checks,
		CreateUserScript: buildPGCreateUserScript(s.Username),
	}
	if len(missingGrants) > 0 {
		resp.GrantScript = buildPGGrantScript(s.Username, missingGrants)
	}
	return resp, nil
}

// pgQuoteIdent double-quotes a PostgreSQL identifier and escapes internal quotes.
func pgQuoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func buildPGGrantScript(username string, missingGrants []string) string {
	quoted := pgQuoteIdent(username)
	var b strings.Builder
	b.WriteString("-- ============================================================\n")
	b.WriteString("-- SQL Optima: Grant missing monitoring permissions\n")
	b.WriteString("-- Run as a PostgreSQL superuser on the target database\n")
	b.WriteString("-- ============================================================\n\n")
	for _, g := range missingGrants {
		line := strings.ReplaceAll(g, pgQuoteIdent(""), pgQuoteIdent("")) // noop, grants already substituted
		_ = line
		b.WriteString(strings.ReplaceAll(g, "{user}", quoted))
		b.WriteString("\n")
	}
	return b.String()
}

func buildPGCreateUserScript(username string) string {
	quoted := pgQuoteIdent(username)
	raw := username

	var b strings.Builder
	b.WriteString("-- ============================================================\n")
	b.WriteString("-- SQL Optima: Create monitoring user for PostgreSQL\n")
	b.WriteString("-- Run as a PostgreSQL superuser\n")
	b.WriteString("-- After creation, set a strong password:\n")
	b.WriteString(fmt.Sprintf("--   ALTER ROLE %s PASSWORD 'your-secret-from-vault';\n", quoted))
	b.WriteString("-- ============================================================\n\n")

	b.WriteString("DO $$\n")
	b.WriteString("BEGIN\n")
	b.WriteString(fmt.Sprintf("    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = '%s') THEN\n", escapeSQLStr(raw)))
	b.WriteString(fmt.Sprintf("        CREATE ROLE %s WITH\n", quoted))
	b.WriteString("            LOGIN\n")
	b.WriteString("            NOSUPERUSER\n")
	b.WriteString("            NOCREATEDB\n")
	b.WriteString("            NOCREATEROLE\n")
	b.WriteString("            NOREPLICATION\n")
	b.WriteString("            CONNECTION LIMIT 100;\n")
	b.WriteString(fmt.Sprintf("        RAISE NOTICE 'Role %s created — set password with ALTER ROLE %s PASSWORD ''...''';\n", quoted, quoted))
	b.WriteString("    ELSE\n")
	b.WriteString(fmt.Sprintf("        RAISE NOTICE 'Role %s already exists.';\n", quoted))
	b.WriteString("    END IF;\n")
	b.WriteString("END\n")
	b.WriteString("$$;\n\n")

	b.WriteString("-- Grant required system roles\n")
	b.WriteString(fmt.Sprintf("GRANT pg_read_all_settings TO %s;\n", quoted))
	b.WriteString(fmt.Sprintf("GRANT pg_read_all_stats TO %s;\n", quoted))
	b.WriteString(fmt.Sprintf("GRANT pg_stat_scan_tables TO %s;\n", quoted))
	b.WriteString(fmt.Sprintf("GRANT pg_monitor TO %s;\n\n", quoted))

	b.WriteString("-- Grant SELECT on key monitoring views\n")
	b.WriteString(fmt.Sprintf("GRANT SELECT ON pg_stat_activity TO %s;\n", quoted))
	b.WriteString(fmt.Sprintf("GRANT SELECT ON pg_stat_bgwriter TO %s;\n", quoted))
	b.WriteString(fmt.Sprintf("GRANT SELECT ON pg_stat_database TO %s;\n", quoted))
	b.WriteString(fmt.Sprintf("GRANT SELECT ON pg_stat_user_indexes TO %s;\n", quoted))
	b.WriteString(fmt.Sprintf("GRANT SELECT ON pg_stat_replication TO %s;\n", quoted))
	b.WriteString(fmt.Sprintf("GRANT SELECT ON pg_replication_slots TO %s;\n", quoted))
	b.WriteString(fmt.Sprintf("GRANT SELECT ON pg_locks TO %s;\n\n", quoted))

	b.WriteString("-- Grant SELECT on pg_catalog system tables\n")
	b.WriteString(fmt.Sprintf("GRANT SELECT ON pg_catalog.pg_stat_activity TO %s;\n", quoted))
	b.WriteString(fmt.Sprintf("GRANT SELECT ON pg_catalog.pg_stat_bgwriter TO %s;\n", quoted))
	b.WriteString(fmt.Sprintf("GRANT SELECT ON pg_catalog.pg_stat_database TO %s;\n", quoted))
	b.WriteString(fmt.Sprintf("GRANT SELECT ON pg_catalog.pg_stat_user_tables TO %s;\n", quoted))
	b.WriteString(fmt.Sprintf("GRANT SELECT ON pg_catalog.pg_stat_user_indexes TO %s;\n", quoted))
	b.WriteString(fmt.Sprintf("GRANT SELECT ON pg_catalog.pg_stat_sys_tables TO %s;\n", quoted))
	b.WriteString(fmt.Sprintf("GRANT SELECT ON pg_catalog.pg_statio_user_tables TO %s;\n", quoted))
	b.WriteString(fmt.Sprintf("GRANT SELECT ON pg_catalog.pg_statio_sys_tables TO %s;\n", quoted))
	b.WriteString(fmt.Sprintf("GRANT SELECT ON pg_catalog.pg_locks TO %s;\n", quoted))
	b.WriteString(fmt.Sprintf("GRANT SELECT ON pg_catalog.pg_stat_replication TO %s;\n", quoted))
	b.WriteString(fmt.Sprintf("GRANT SELECT ON pg_catalog.pg_replication_origin_status TO %s;\n", quoted))
	b.WriteString(fmt.Sprintf("GRANT SELECT ON pg_catalog.pg_replication_slots TO %s;\n", quoted))
	b.WriteString(fmt.Sprintf("GRANT SELECT ON pg_catalog.pg_settings TO %s;\n", quoted))
	b.WriteString(fmt.Sprintf("GRANT SELECT ON pg_catalog.pg_roles TO %s;\n", quoted))
	b.WriteString(fmt.Sprintf("GRANT SELECT ON pg_catalog.pg_database TO %s;\n", quoted))
	b.WriteString(fmt.Sprintf("GRANT SELECT ON pg_catalog.pg_namespace TO %s;\n", quoted))
	b.WriteString(fmt.Sprintf("GRANT SELECT ON pg_catalog.pg_class TO %s;\n", quoted))
	b.WriteString(fmt.Sprintf("GRANT SELECT ON pg_catalog.pg_attribute TO %s;\n", quoted))
	b.WriteString(fmt.Sprintf("GRANT SELECT ON pg_catalog.pg_proc TO %s;\n", quoted))
	b.WriteString(fmt.Sprintf("GRANT SELECT ON pg_catalog.pg_type TO %s;\n", quoted))
	b.WriteString(fmt.Sprintf("GRANT SELECT ON pg_catalog.pg_index TO %s;\n", quoted))
	b.WriteString(fmt.Sprintf("GRANT SELECT ON pg_catalog.pg_inherits TO %s;\n", quoted))
	b.WriteString(fmt.Sprintf("GRANT SELECT ON pg_catalog.pg_tablespace TO %s;\n\n", quoted))

	b.WriteString("-- Grant access to information_schema\n")
	b.WriteString(fmt.Sprintf("GRANT SELECT ON information_schema.tables TO %s;\n", quoted))
	b.WriteString(fmt.Sprintf("GRANT SELECT ON information_schema.columns TO %s;\n", quoted))
	b.WriteString(fmt.Sprintf("GRANT SELECT ON information_schema.views TO %s;\n", quoted))
	b.WriteString(fmt.Sprintf("GRANT SELECT ON information_schema.routines TO %s;\n", quoted))
	b.WriteString(fmt.Sprintf("GRANT SELECT ON information_schema.table_privileges TO %s;\n", quoted))
	b.WriteString(fmt.Sprintf("GRANT SELECT ON information_schema.column_privileges TO %s;\n\n", quoted))

	b.WriteString("-- Optional: pg_stat_statements extension (if installed)\n")
	b.WriteString("DO $$\n")
	b.WriteString("BEGIN\n")
	b.WriteString("    IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'pg_stat_statements') THEN\n")
	b.WriteString(fmt.Sprintf("        EXECUTE 'GRANT SELECT ON pg_stat_statements TO %s';\n", quoted))
	b.WriteString(fmt.Sprintf("        RAISE NOTICE 'Granted pg_stat_statements access to %s';\n", quoted))
	b.WriteString("    ELSE\n")
	b.WriteString("        RAISE NOTICE 'pg_stat_statements not installed (optional).';\n")
	b.WriteString("    END IF;\n")
	b.WriteString("END\n")
	b.WriteString("$$;\n")

	return b.String()
}

// ── SQL Server ────────────────────────────────────────────────────────────────

type mssqlProbe struct {
	name     string
	query    string // SELECT HAS_PERMS_BY_NAME returns 1/0, or a query that succeeds/fails
	isPerms  bool   // true: query returns an int (1=granted, 0=denied); false: test execution
	grant    string // SQL fragment to grant this permission
	optional bool
}

var mssqlProbes = []mssqlProbe{
	{
		name:    "VIEW SERVER STATE",
		query:   `SELECT HAS_PERMS_BY_NAME(NULL, NULL, 'VIEW SERVER STATE')`,
		isPerms: true,
		grant:   "GRANT VIEW SERVER STATE TO [{user}];",
	},
	{
		name:    "VIEW ANY DEFINITION",
		query:   `SELECT HAS_PERMS_BY_NAME(NULL, NULL, 'VIEW ANY DEFINITION')`,
		isPerms: true,
		grant:   "GRANT VIEW ANY DEFINITION TO [{user}];",
	},
	{
		name:  "sys.dm_exec_sessions (DMV access)",
		query: `SELECT TOP 0 session_id FROM sys.dm_exec_sessions`,
		grant: "-- Requires VIEW SERVER STATE:\nGRANT VIEW SERVER STATE TO [{user}];",
	},
	{
		name:  "sys.dm_exec_query_stats (DMV access)",
		query: `SELECT TOP 0 sql_handle FROM sys.dm_exec_query_stats`,
		grant: "-- Requires VIEW SERVER STATE:\nGRANT VIEW SERVER STATE TO [{user}];",
	},
	{
		name:  "sys.dm_os_performance_counters (DMV access)",
		query: `SELECT TOP 0 counter_name FROM sys.dm_os_performance_counters`,
		grant: "-- Requires VIEW SERVER STATE:\nGRANT VIEW SERVER STATE TO [{user}];",
	},
	{
		name:  "msdb: sysjobs",
		query: `SELECT TOP 0 job_id FROM msdb.dbo.sysjobs`,
		grant: "-- In msdb database:\nUSE msdb;\nGO\nIF NOT EXISTS (SELECT name FROM sys.database_principals WHERE name = '{user}')\nBEGIN\n    CREATE USER [{user}] FOR LOGIN [{user}];\nEND\nGO\nGRANT SELECT ON dbo.sysjobs TO [{user}];\nEXEC sp_addrolemember 'SQLAgentReaderRole', '{user}';\nGO",
	},
	{
		name:     "msdb: sysjobhistory",
		query:    `SELECT TOP 0 job_id FROM msdb.dbo.sysjobhistory`,
		grant:    "-- In msdb database:\nGRANT SELECT ON msdb.dbo.sysjobhistory TO [{user}];\nGO",
		optional: true,
	},
	{
		name:     "msdb: sysjobactivity",
		query:    `SELECT TOP 0 job_id FROM msdb.dbo.sysjobactivity`,
		grant:    "-- In msdb database:\nGRANT SELECT ON msdb.dbo.sysjobactivity TO [{user}];\nGO",
		optional: true,
	},
}

func checkSQLServerPermissions(ctx context.Context, s servers.Server, cred servers.CredentialPayload) (*permissionsCheckResponse, error) {
	cat := sqlServerInitialCatalog(cred)
	trust := "false"
	if cred.TrustServerCertificate {
		trust = "true"
	}
	encrypt := "true"
	switch strings.ToLower(strings.TrimSpace(cred.SSLMode)) {
	case "disable", "disabled", "false", "no", "off":
		encrypt = "false"
	}
	msURL := &url.URL{
		Scheme: "sqlserver",
		User:   url.UserPassword(s.Username, cred.Password),
		Host:   net.JoinHostPort(s.Host, itoa(s.Port)),
	}
	q := msURL.Query()
	q.Set("database", cat)
	q.Set("encrypt", encrypt)
	q.Set("TrustServerCertificate", trust)
	msURL.RawQuery = q.Encode()

	db, err := sqlserver.OpenMetricsPool(msURL.String())
	if err != nil {
		return nil, sanitizeDBError(err, "sqlserver")
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return nil, sanitizeDBError(err, "sqlserver")
	}

	var checks []PermissionCheckResult
	var missingGrants []string
	allOK := true

	for _, p := range mssqlProbes {
		result := PermissionCheckResult{Name: p.name, Optional: p.optional}

		if p.isPerms {
			var granted sql.NullInt64
			scanErr := db.QueryRowContext(ctx, p.query).Scan(&granted)
			if scanErr != nil || !granted.Valid || granted.Int64 != 1 {
				result.OK = false
				result.Detail = "permission not granted"
			} else {
				result.OK = true
			}
		} else {
			rows, queryErr := db.QueryContext(ctx, p.query)
			if queryErr != nil {
				result.OK = false
				result.Detail = "permission denied"
			} else {
				rows.Close()
				result.OK = true
			}
		}

		if !result.OK {
			if !p.optional {
				allOK = false
			}
			grant := strings.ReplaceAll(p.grant, "{user}", s.Username)
			grant = strings.ReplaceAll(grant, "[{user}]", fmt.Sprintf("[%s]", escapeSQLBracket(s.Username)))
			missingGrants = append(missingGrants, grant)
		}
		checks = append(checks, result)
	}

	resp := &permissionsCheckResponse{
		PermissionsOK:    allOK,
		Checks:           checks,
		CreateUserScript: buildMSSQLCreateUserScript(s.Username),
	}
	if len(missingGrants) > 0 {
		resp.GrantScript = buildMSSQLGrantScript(s.Username, missingGrants)
	}
	return resp, nil
}

func buildMSSQLGrantScript(username string, missingGrants []string) string {
	bracketed := fmt.Sprintf("[%s]", escapeSQLBracket(username))
	var b strings.Builder
	b.WriteString("-- ============================================================\n")
	b.WriteString("-- SQL Optima: Grant missing monitoring permissions\n")
	b.WriteString("-- Run as sysadmin on your SQL Server instance\n")
	b.WriteString("-- ============================================================\n\n")
	b.WriteString("USE master;\nGO\n\n")

	masterGrants := []string{}
	msdbGrants := []string{}

	for _, g := range missingGrants {
		if strings.Contains(g, "msdb") {
			msdbGrants = append(msdbGrants, g)
		} else {
			masterGrants = append(masterGrants, g)
		}
	}

	for _, g := range masterGrants {
		line := strings.ReplaceAll(g, "{user}", username)
		line = strings.ReplaceAll(line, "[{user}]", bracketed)
		b.WriteString(line)
		b.WriteString("\n")
	}
	if len(masterGrants) > 0 {
		b.WriteString("GO\n")
	}
	if len(msdbGrants) > 0 {
		b.WriteString("\nUSE msdb;\nGO\n")
		seen := map[string]bool{}
		for _, g := range msdbGrants {
			line := strings.ReplaceAll(g, "{user}", username)
			line = strings.ReplaceAll(line, "[{user}]", bracketed)
			if !seen[line] {
				b.WriteString(line)
				b.WriteString("\n")
				seen[line] = true
			}
		}
		b.WriteString("GO\n")
	}
	return b.String()
}

func buildMSSQLCreateUserScript(username string) string {
	bracketed := fmt.Sprintf("[%s]", escapeSQLBracket(username))

	var b strings.Builder
	b.WriteString("-- ============================================================\n")
	b.WriteString("-- SQL Optima: Create monitoring login/user for SQL Server\n")
	b.WriteString("-- Run as sysadmin on your SQL Server instance\n")
	b.WriteString("-- Replace __STRONG_PASSWORD__ with a password from your vault\n")
	b.WriteString("-- ============================================================\n\n")

	b.WriteString("USE master;\nGO\n\n")

	b.WriteString(fmt.Sprintf("-- Create login (if it doesn't exist)\nIF NOT EXISTS (SELECT name FROM sys.server_principals WHERE name = '%s')\nBEGIN\n", escapeSQLStr(username)))
	b.WriteString(fmt.Sprintf("    CREATE LOGIN %s WITH\n", bracketed))
	b.WriteString("        PASSWORD = N'__STRONG_PASSWORD__',\n")
	b.WriteString("        DEFAULT_DATABASE = [master],\n")
	b.WriteString("        CHECK_POLICY = ON,\n")
	b.WriteString("        CHECK_EXPIRATION = OFF;\n")
	b.WriteString(fmt.Sprintf("    PRINT 'Login %s created.';\n", bracketed))
	b.WriteString("END\nELSE\nBEGIN\n")
	b.WriteString(fmt.Sprintf("    PRINT 'Login %s already exists.';\n", bracketed))
	b.WriteString("END\nGO\n\n")

	b.WriteString("-- Grant server-level permissions\n")
	b.WriteString(fmt.Sprintf("GRANT VIEW SERVER STATE TO %s;\n", bracketed))
	b.WriteString(fmt.Sprintf("GRANT VIEW ANY DEFINITION TO %s;\n", bracketed))
	b.WriteString("GO\n\n")

	b.WriteString("-- Create user in msdb for SQL Agent job monitoring\n")
	b.WriteString("USE msdb;\nGO\n\n")
	b.WriteString(fmt.Sprintf("IF NOT EXISTS (SELECT name FROM sys.database_principals WHERE name = '%s')\nBEGIN\n", escapeSQLStr(username)))
	b.WriteString(fmt.Sprintf("    CREATE USER %s FOR LOGIN %s;\n", bracketed, bracketed))
	b.WriteString(fmt.Sprintf("    PRINT 'User %s created in msdb.';\n", bracketed))
	b.WriteString("END\nGO\n\n")

	b.WriteString("-- Grant SELECT on SQL Agent tables\n")
	b.WriteString(fmt.Sprintf("GRANT SELECT ON dbo.sysjobs TO %s;\n", bracketed))
	b.WriteString(fmt.Sprintf("GRANT SELECT ON dbo.sysjobschedules TO %s;\n", bracketed))
	b.WriteString(fmt.Sprintf("GRANT SELECT ON dbo.sysjobactivity TO %s;\n", bracketed))
	b.WriteString(fmt.Sprintf("GRANT SELECT ON dbo.sysjobhistory TO %s;\n", bracketed))
	b.WriteString(fmt.Sprintf("GRANT SELECT ON dbo.sysschedules TO %s;\n", bracketed))
	b.WriteString(fmt.Sprintf("GRANT SELECT ON dbo.syscategories TO %s;\n", bracketed))
	b.WriteString(fmt.Sprintf("GRANT SELECT ON dbo.sysjobsteps TO %s;\n", bracketed))
	b.WriteString(fmt.Sprintf("GRANT SELECT ON dbo.sysoperators TO %s;\n", bracketed))
	b.WriteString("GO\n\n")

	b.WriteString("-- Add to SQLAgentReaderRole for enhanced job visibility\n")
	b.WriteString(fmt.Sprintf("EXEC sp_addrolemember 'SQLAgentReaderRole', '%s';\n", escapeSQLStr(username)))
	b.WriteString("GO\n\n")

	b.WriteString("-- Optional: create user in each target database for query monitoring\n")
	b.WriteString("-- Run the block below for each database you want to monitor:\n")
	b.WriteString("/*\n")
	b.WriteString("USE [YourDatabaseName];\nGO\n")
	b.WriteString(fmt.Sprintf("IF NOT EXISTS (SELECT name FROM sys.database_principals WHERE name = '%s')\nBEGIN\n", escapeSQLStr(username)))
	b.WriteString(fmt.Sprintf("    CREATE USER %s FOR LOGIN %s;\n", bracketed, bracketed))
	b.WriteString("END\n")
	b.WriteString(fmt.Sprintf("GRANT SELECT ON SCHEMA::dbo TO %s;\n", bracketed))
	b.WriteString("GO\n*/\n")

	return b.String()
}

// ── helpers ───────────────────────────────────────────────────────────────────

// escapeSQLStr escapes single quotes in a string literal for use inside SQL single-quoted strings.
func escapeSQLStr(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// escapeSQLBracket escapes closing brackets inside a SQL Server bracketed identifier.
func escapeSQLBracket(s string) string {
	return strings.ReplaceAll(s, "]", "]]")
}

// parsePortAny converts the flexible port type (float64/string/int) from JSON into an int.
func parsePortAny(v any) int {
	switch p := v.(type) {
	case float64:
		return int(p)
	case string:
		// Handled by strconv.Atoi equivalent inline to avoid import cycle.
		n := 0
		for _, c := range p {
			if c < '0' || c > '9' {
				return 0
			}
			n = n*10 + int(c-'0')
		}
		return n
	case int:
		return p
	}
	return 0
}
