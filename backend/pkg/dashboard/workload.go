// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Workload capacity calculation for dashboard metrics.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package dashboard

// CompilationRatio computes compilations/batch requests as a ratio (0..1).
// Returns 0 if inputs are missing or invalid.
func CompilationRatio(batchReqPerSec, compilationsPerSec float64) float64 {
	if batchReqPerSec <= 0 || compilationsPerSec < 0 {
		return 0
	}
	return compilationsPerSec / batchReqPerSec
}

func CompilationSeverity(ratio float64) Severity {
	// Rule from Upgrade_main_dashboard.md:
	// If compilations > 10% of batch requests → Yellow warning.
	if ratio > 0.10 {
		return SeverityWarning
	}
	return SeverityOK
}

