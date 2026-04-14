// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Delta computation for storage index health metrics over time.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package storageindex

// Delta clamps counter resets (<0) by returning ok=false.
// Callers should drop the sample when ok=false (e.g., SQL Server DMV reset after restart).
func Delta(current, previous int64) (delta int64, ok bool) {
	d := current - previous
	if d < 0 {
		return 0, false
	}
	return d, true
}

