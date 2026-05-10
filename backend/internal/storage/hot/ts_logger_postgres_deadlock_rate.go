// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Compute PostgreSQL deadlock rate using delta tracking.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package hot

// ComputePgDeadlockRate returns deadlocks per minute using delta tracking.
// Returns (rate, true) on second+ call; (0, false) on first.
func (tl *TimescaleLogger) ComputePgDeadlockRate(instanceName string, totalDeadlocks int64, intervalSec float64) (rate float64, ok bool) {
	if intervalSec <= 0 {
		intervalSec = 60
	}
	tl.mu.Lock()
	defer tl.mu.Unlock()

	prev, seen := tl.prevPgDeadlocksTotalAllDBs[instanceName]
	tl.prevPgDeadlocksTotalAllDBs[instanceName] = totalDeadlocks

	if !seen {
		return 0, false
	}

	delta := totalDeadlocks - prev
	if delta < 0 {
		// Counter reset (postgres restart)
		return 0, true
	}

	// rate per minute
	rate = (float64(delta) * 60.0) / intervalSec
	return rate, true
}
