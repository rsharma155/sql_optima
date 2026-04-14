// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Provides hot functionality for the backend.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package hot

import "testing"

func TestNormalizePgLogSeverity(t *testing.T) {
	if normalizePgLogSeverity("FATAL") != "fatal" {
		t.Fatalf("expected fatal")
	}
	if normalizePgLogSeverity(" panic ") != "panic" {
		t.Fatalf("expected panic")
	}
	if normalizePgLogSeverity("ERR") != "error" {
		t.Fatalf("expected error")
	}
	if normalizePgLogSeverity("") != "unknown" {
		t.Fatalf("expected unknown")
	}
}

