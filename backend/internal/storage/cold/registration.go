// SQL Optima — https://github.com/rsharma155/sql_optima
//
// File: backend/internal/storage/cold/registration.go
// Purpose: Entry point for cold storage table registration. Delegates to domain-
//          specific files so each stays under the 300-line project limit.
//
//   registration_sqlserver_core.go    — CPU, wait, metrics, connections, memory, disk, throughput, locks
//   registration_sqlserver_ha.go      — AG health, risk health, AG cluster info
//   registration_sqlserver_queries.go — long-running queries, procedures, Query Store, scheduler, latches
//   registration_postgres.go          — wait events, IO stats, session activity, backup, DDL, PGSS
//   registration_system.go            — job metrics, settings/roles snapshots, collector runs
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package cold

// RegisterCoreTables registers all 36 TimescaleDB hypertables with the cold
// storage exporter. It is called once at startup from appserver.go.
func RegisterCoreTables(e *Exporter) {
	registerSQLServerCore(e)
	registerSQLServerHA(e)
	registerSQLServerQueries(e)
	registerPostgres(e)
	registerSystem(e)
}
