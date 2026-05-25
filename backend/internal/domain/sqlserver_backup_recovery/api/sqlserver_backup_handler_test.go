// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Unit tests for SQL Server Backup & Recovery handler helpers.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package api

import "testing"

func TestFilterFailedBackupJobs(t *testing.T) {
	failures := []map[string]interface{}{
		{"job_name": "Nightly Backup"},
		{"job_name": "Index Rebuild"},
		{"job_name": "ETL Load"},
	}
	n := filterFailedBackupJobs(failures)
	if n != 1 {
		t.Fatalf("expected 1 backup-related failure, got %d", n)
	}
}

func TestLogShippingSignals_Behind(t *testing.T) {
	rows := []map[string]interface{}{
		{"restore_delay_minutes": 45, "restore_threshold_minutes": 30},
	}
	behind, enabled := logShippingSignals(rows)
	if !enabled || behind != 1 {
		t.Fatalf("behind=%d enabled=%v", behind, enabled)
	}
}

func TestSplitOverall(t *testing.T) {
	o, c := splitOverall("needs_attention:bad")
	if o != "needs_attention" || c != "bad" {
		t.Fatalf("o=%s c=%s", o, c)
	}
}
