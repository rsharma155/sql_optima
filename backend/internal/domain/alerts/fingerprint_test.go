// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Unit tests for deterministic fingerprint generation.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package alerts

import (
	"testing"
	"github.com/google/uuid"
)

func TestFingerprint_Deterministic(t *testing.T) {
	id := uuid.New()
	fp1 := Fingerprint(id, EngineSQLServer, "blocking", "sqlserver_blocking")
	fp2 := Fingerprint(id, EngineSQLServer, "blocking", "sqlserver_blocking")
	if fp1 != fp2 {
		t.Errorf("fingerprints should be deterministic: %q != %q", fp1, fp2)
	}
}

func TestFingerprint_DifferentInputs(t *testing.T) {
	id1 := uuid.New()
	id2 := uuid.New()
	fp1 := Fingerprint(id1, EngineSQLServer, "blocking", "sqlserver_blocking")
	fp2 := Fingerprint(id2, EngineSQLServer, "blocking", "sqlserver_blocking")
	if fp1 == fp2 {
		t.Error("different instances should produce different fingerprints")
	}
}

func TestFingerprint_CaseInsensitive(t *testing.T) {
	id := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	fp1 := Fingerprint(id, EngineSQLServer, "Blocking", "SQLSERVER_Blocking")
	fp2 := Fingerprint(id, EngineSQLServer, "blocking", "sqlserver_blocking")
	if fp1 != fp2 {
		t.Errorf("fingerprints should be case-insensitive: %q != %q", fp1, fp2)
	}
}

func TestFingerprint_TrimsSpaces(t *testing.T) {
	id := uuid.New()
	fp1 := Fingerprint(id, EngineSQLServer, " blocking ", " sqlserver_blocking ")
	fp2 := Fingerprint(id, EngineSQLServer, "blocking", "sqlserver_blocking")
	if fp1 != fp2 {
		t.Errorf("fingerprints should trim spaces: %q != %q", fp1, fp2)
	}
}

func TestFingerprint_Length(t *testing.T) {
	id := uuid.New()
	fp := Fingerprint(id, EngineSQLServer, "blocking", "sqlserver_blocking")
	if len(fp) != 64 { // SHA256 hex string is 64 chars, not 32.
		t.Errorf("fingerprint length = %d, want 64", len(fp))
	}
}
