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
// PgSystemDatabases lists PostgreSQL cluster-internal databases to skip during collection.
// Use SQL_OPTIMA_PG_INCLUDE_SYSTEM_DATABASES=true to override.
var PgSystemDatabases = []string{"postgres", "template0", "template1"}

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
	add("dbmonitor")
	add("sql_optima")
	add("sqloptima")
	// Infrastructure / HA / proxy roles
	add("pgbouncer")
	add("repmgr")
	add("barman")
	add("patroni")
	add("pg_monitor")
	add("streaming_replica")
	// add("postgres") // superuser — removed from default exclude to allow monitoring benchmarking/load-tests run as postgres
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
		  AND s.query NOT LIKE '%/* SQL_OPTIMA */%'
		  AND s.query NOT ILIKE '<insufficient privilege>%'
		  AND s.query NOT ILIKE '%sql_optima%'
		  AND s.query NOT ILIKE '%sqloptima%'
		  AND s.query NOT ILIKE '%SQLOptima%'
		  AND s.query NOT ILIKE '%DeltaCollector%'
		  AND s.query NOT ILIKE 'autovacuum:%'
		  AND s.query NOT ILIKE 'analyze %'
		  AND s.query NOT ILIKE 'vacuum %'
		  AND s.query NOT ILIKE 'reindex %'
		  AND s.query NOT ILIKE 'checkpoint%'
		  AND s.query NOT ILIKE 'discard %'
		  AND s.query NOT ILIKE 'deallocate%'
		  AND s.query NOT ILIKE 'prepare %'
		  AND s.query NOT ILIKE 'listen %'
		  AND s.query NOT ILIKE 'notify %'
		  AND s.query NOT ILIKE 'show %'
		  AND s.query NOT ILIKE 'set %'
		  AND s.query NOT ILIKE 'fetch %'
		  AND s.query NOT ILIKE 'declare %'
		  AND s.query NOT ILIKE 'begin%'
		  AND s.query NOT ILIKE 'commit%'
		  AND s.query NOT ILIKE 'rollback%'
		  AND s.query NOT ILIKE 'copy %'
		  AND s.query NOT ILIKE 'cluster %'
		  AND s.query NOT ILIKE 'explain %'
		  AND s.query NOT ILIKE '%pg_backend_pid%'
		  AND s.query NOT ILIKE '%pg_terminate_backend%'
		  AND s.query NOT ILIKE '%pg_cancel_backend%'
		  AND s.query NOT ILIKE '%pg_reload_conf%'
		  AND s.query NOT ILIKE '%pg_advisory_lock%'
		  AND s.query NOT ILIKE '%pg_try_advisory_lock%'
		  AND s.query NOT ILIKE '%pg_advisory_unlock%'
		  AND s.query NOT ILIKE '%pg_total_relation_size%'
		  AND s.query NOT ILIKE '%pg_database_size%'
		  AND s.query NOT ILIKE '%pg_relation_size%'
		  AND s.query NOT ILIKE '%pg_indexes_size%'
		  AND s.query NOT ILIKE '%timescaledb_information.%'
		  AND s.query NOT ILIKE '%_timescaledb_internal.%'
		  AND s.query NOT ILIKE '%pg_stat_monitor%'
		  AND s.query NOT ILIKE '%SELECT /* SQL_OPTIMA */%'
		  AND s.query <> 'SELECT 1'
		  AND s.query <> 'SELECT $1'`)
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

	// ORM / framework internal catalog queries
	b.WriteString(`
		  AND s.query NOT ILIKE '%pg_type%'
		  AND s.query NOT ILIKE '%pg_attribute%'
		  AND s.query NOT ILIKE '%pg_class%'
		  AND s.query NOT ILIKE '%pg_namespace%'
		  AND s.query NOT ILIKE '%pg_constraint%'
		  AND s.query NOT ILIKE '%pg_description%'
		  AND s.query NOT ILIKE '%information_schema.%'`)
	// Connection pool / health-check pings
	b.WriteString(`
		  AND s.query NOT IN ('SELECT 1', 'SELECT $1', 'select 1', '/* ping */', 'select version()')`)

	// Performance thresholds: Ignore very light/rare queries.
	// NOTE: To capture ALL queries (including low frequency and fast execution),
	// these filters (calls > 10 and mean time > 5ms) are removed to help troubleshoot "no data" issues.
	b.WriteString(`
		  AND s.calls > 0`)

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
