// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Table growth tracking collector for capacity planning.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package collectors

import (
	"context"
	"time"

	"github.com/rsharma155/sql_optima/internal/storage/hot"
)

// This file exists to match the collector module structure described in
// `index_storage_details.md`. The SQL Server implementation reuses the existing
// table size snapshot query and persists the growth snapshot into
// monitor.table_size_history (TimescaleDB).

// CollectSQLServerTableGrowthSnapshot is an alias of CollectSQLServerTableSizeSnapshot.
// It returns (db, schema, table, row_count, table_size_mb, index_size_mb).
func CollectSQLServerTableGrowthSnapshot(ctx context.Context, dbq Queryer) ([]SqlServerTableUsageRow, error) {
	return CollectSQLServerTableSizeSnapshot(ctx, dbq)
}

// PersistSQLServerTableGrowthHistory is an alias of PersistSQLServerTableSizeHistory.
// It writes one snapshot row per table into monitor.table_size_history.
func PersistSQLServerTableGrowthHistory(ctx context.Context, tl *hot.TimescaleLogger, serverID string, rows []SqlServerTableUsageRow, capture time.Time) (int, error) {
	return PersistSQLServerTableSizeHistory(ctx, tl, serverID, rows, capture)
}
