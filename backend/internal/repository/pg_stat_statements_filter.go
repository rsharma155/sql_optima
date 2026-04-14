// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: pg_stat_statements filtering and normalization utilities.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"os"
	"sort"
	"strings"
)

// pgStatStatementsExcludedRoleNames returns role names whose statements are omitted from Query Performance.
// Default: dbmonitor_user. Extend with env SQL_OPTIMA_PG_STATEMENTS_EXCLUDE_USERS=comma,separated,names
func pgStatStatementsExcludedRoleNames() []string {
	seen := make(map[string]struct{})
	add := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		seen[name] = struct{}{}
	}
	add("dbmonitor_user")
	if extra := os.Getenv("SQL_OPTIMA_PG_STATEMENTS_EXCLUDE_USERS"); extra != "" {
		for _, p := range strings.Split(extra, ",") {
			add(p)
		}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// buildPgStatStatementsFilters returns the shared WHERE body for alias s with pg_roles r joined on s.userid.
// Excludes monitoring users, autovacuum/maintenance noise, catalog introspection, and replication / logical decoding protocol SQL.
func buildPgStatStatementsFilters() string {
	var b strings.Builder
	b.WriteString(`s.query NOT LIKE '%pg_stat_statements%'`)
	b.WriteString(`
		  AND s.query NOT ILIKE 'autovacuum:%'
		  AND s.query NOT ILIKE 'analyze %'
		  AND s.query NOT ILIKE 'vacuum %'
		  AND s.query NOT ILIKE 'reindex %'
		  AND s.query NOT ILIKE 'checkpoint%'
		  AND s.query NOT ILIKE 'show %'
		  AND s.query NOT ILIKE 'set %'
		  AND s.query NOT ILIKE 'begin%'
		  AND s.query NOT ILIKE 'commit%'
		  AND s.query NOT ILIKE 'rollback%'
		  AND s.query NOT ILIKE '%pg_catalog.%'
		  AND s.query NOT ILIKE '%information_schema.%'
		  AND s.query NOT ILIKE '%pg_toast.%'
		  AND s.query NOT ILIKE '%pg_stat_%'`)
	// Replication / physical & logical decoding (walsender, pg_recvlogical, etc.)
	b.WriteString(`
		  AND s.query NOT ILIKE '%IDENTIFY_SYSTEM%'
		  AND s.query NOT ILIKE '%START_REPLICATION%'
		  AND s.query NOT ILIKE '%CREATE_REPLICATION_SLOT%'
		  AND s.query NOT ILIKE '%ALTER_REPLICATION_SLOT%'
		  AND s.query NOT ILIKE '%DROP_REPLICATION_SLOT%'
		  AND s.query NOT ILIKE '%READ_REPLICATION_SLOT%'
		  AND s.query NOT ILIKE '%pg_logical_slot_get_changes%'
		  AND s.query NOT ILIKE '%pg_logical_slot_peek_changes%'
		  AND s.query NOT ILIKE '%pg_create_logical_replication_slot%'
		  AND s.query NOT ILIKE '%pg_drop_replication_slot%'
		  AND s.query NOT ILIKE '%pg_replication_slot_advance%'
		  AND s.query NOT ILIKE '%pg_sync_replication_slots%'
		  AND s.query NOT ILIKE '%BASE_BACKUP%'
		  AND s.query NOT ILIKE '%TIMELINE_HISTORY%'`)
	roles := pgStatStatementsExcludedRoleNames()
	if len(roles) > 0 {
		b.WriteString(`
		  AND COALESCE(r.rolname, '') NOT IN (`)
		for i, rname := range roles {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString("'")
			b.WriteString(strings.ReplaceAll(rname, "'", "''"))
			b.WriteString("'")
		}
		b.WriteString(")")
	}
	return b.String()
}
