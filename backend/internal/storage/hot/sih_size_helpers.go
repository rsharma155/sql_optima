// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Storage size aggregation helpers for Storage/Index Health KPIs.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package hot

// sihTotalRelationSizeMB returns total on-disk footprint in MB from latest per-table snapshots.
//
// table_size_mb is the authoritative total per relation:
//   - PostgreSQL: pg_total_relation_size (heap + indexes + TOAST)
//   - SQL Server: SUM(allocation_units.total_pages) for the table
//
// index_size_mb is an informational breakdown (subset of table_size_mb) and must not be added.
func sihTotalRelationSizeMB(tableMB float64) float64 {
	if tableMB < 0 {
		return 0
	}
	return tableMB
}
