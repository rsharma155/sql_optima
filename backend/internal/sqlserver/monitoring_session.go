// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: SQL Server monitoring session management for DMV queries.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package sqlserver

// MonitoringSessionInitSQL is executed by go-mssqldb on new connections and on
// pooled connection reset (driver.SessionResetter), before application queries run.
const MonitoringSessionInitSQL = `SET NOCOUNT ON;
SET TRANSACTION ISOLATION LEVEL READ UNCOMMITTED;
SET DEADLOCK_PRIORITY LOW;
SET LOCK_TIMEOUT 5000;`
