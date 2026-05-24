// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Unified collector for sys.dm_os_performance_counters — one query per cycle
//          for all 15 monitored counters, eliminating 14 redundant DMV round-trips.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package sqlserver

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// perfCounterNames is the canonical list of all counters collected in one query.
// Tests reference this slice to guard against accidental removal.
var perfCounterNames = []string{
	"Page life expectancy",
	"Buffer Pool Size (KB)",
	"Total Server Memory (KB)",
	"Target Server Memory (KB)",
	"Memory Grants Pending",
	"Batch Requests/sec",
	"Page Reads/sec",
	"SQL Compilations/sec",
	"Logins/sec",
	"User Connections",
	"Transactions/sec",
	"Buffer cache hit ratio",
	"Buffer cache hit ratio base",
	"Sort Warnings/sec",
	"Hash Warnings/sec",
}

// PerfCounterRow holds one row from sys.dm_os_performance_counters.
type PerfCounterRow struct {
	CounterName  string
	InstanceName string
	CntrValue    int64
	// CntrType: 272696576 = cumulative (needs delta), 65792 = snapshot (use as-is),
	//           1073939712 = ratio numerator, 1073939713 = ratio denominator.
	CntrType   int
	ObjectName string
}

// PerfCountersCollector fetches all target perf counters in a single DMV query.
// It is stateless — previous-snapshot state is held by the calling service.
type PerfCountersCollector struct{}

// Fetch executes one query against sys.dm_os_performance_counters and returns
// all rows for the 15 monitored counter names.
func (c *PerfCountersCollector) Fetch(ctx context.Context, db *sql.DB) ([]PerfCounterRow, error) {
	placeholders := make([]string, len(perfCounterNames))
	args := make([]interface{}, len(perfCounterNames))
	for i, name := range perfCounterNames {
		placeholders[i] = fmt.Sprintf("@p%d", i+1)
		args[i] = name
	}

	q := fmt.Sprintf(`
		/* SQL_OPTIMA */
		SELECT
		    RTRIM(counter_name)  AS counter_name,
		    RTRIM(instance_name) AS instance_name,
		    cntr_value,
		    cntr_type,
		    RTRIM(object_name)   AS object_name
		FROM sys.dm_os_performance_counters WITH (NOLOCK)
		WHERE RTRIM(counter_name) IN (%s)
		  AND object_name NOT LIKE N'%%$%%'
	`, strings.Join(placeholders, ","))

	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("PerfCountersCollector.Fetch: %w", err)
	}
	defer rows.Close()

	var out []PerfCounterRow
	for rows.Next() {
		var r PerfCounterRow
		if err := rows.Scan(&r.CounterName, &r.InstanceName, &r.CntrValue, &r.CntrType, &r.ObjectName); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ComputeRatePerSec returns (curr-prev)/intervalSecs for cumulative counters.
// Returns 0 if the counter reset (curr < prev) or interval is zero.
// Exported so the service layer can call it directly.
func ComputeRatePerSec(curr, prev int64, intervalSecs float64) float64 {
	if intervalSecs <= 0 || curr <= prev {
		return 0
	}
	return float64(curr-prev) / intervalSecs
}

// computeRatePerSec is the unexported alias used by tests in this package.
func computeRatePerSec(curr, prev int64, intervalSecs float64) float64 {
	return ComputeRatePerSec(curr, prev, intervalSecs)
}
