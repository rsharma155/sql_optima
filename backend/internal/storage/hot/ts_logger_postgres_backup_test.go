// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Provides hot functionality for the backend.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package hot

import "testing"

func TestNormalizeBackupStatus(t *testing.T) {
	if normalizeBackupStatus("SUCCESS") != "success" {
		t.Fatalf("expected success")
	}
	if normalizeBackupStatus(" failed ") != "failed" {
		t.Fatalf("expected failed")
	}
	if normalizeBackupStatus("warn") != "warning" {
		t.Fatalf("expected warning")
	}
	if normalizeBackupStatus("") != "unknown" {
		t.Fatalf("expected unknown")
	}
}

