// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: SQL Server connection opener with integrated security and certificate support.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package sqlserver

import (
	"database/sql"
	"fmt"

	"github.com/microsoft/go-mssqldb"
)

// OpenMetricsPool returns a *sql.DB for SQL Server monitoring and metrics collection.
// MonitoringSessionInitSQL is applied via go-mssqldb Connector.SessionInitSQL on connect
// and pooled connection reset.
func OpenMetricsPool(connStr string) (*sql.DB, error) {
	connector, err := mssql.NewConnector(connStr)
	if err != nil {
		return nil, fmt.Errorf("mssql DSN: %w", err)
	}
	connector.SessionInitSQL = MonitoringSessionInitSQL
	return sql.OpenDB(connector), nil
}
