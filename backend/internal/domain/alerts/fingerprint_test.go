// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Unit tests for deterministic fingerprint generation.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package alerts

import "testing"

func TestFingerprint_Deterministic(t *testing.T) {
	fp1 := Fingerprint("prod-db-01", EngineSQLServer, "blocking", "mssql_blocking")
	fp2 := Fingerprint("prod-db-01", EngineSQLServer, "blocking", "mssql_blocking")
	if fp1 != fp2 {
		t.Errorf("fingerprints should be deterministic: %q != %q", fp1, fp2)
	}
}

func TestFingerprint_DifferentInputs(t *testing.T) {
	fp1 := Fingerprint("prod-db-01", EngineSQLServer, "blocking", "mssql_blocking")
	fp2 := Fingerprint("prod-db-02", EngineSQLServer, "blocking", "mssql_blocking")
	if fp1 == fp2 {
		t.Error("different instances should produce different fingerprints")
	}
}

func TestFingerprint_CaseInsensitive(t *testing.T) {
	fp1 := Fingerprint("PROD-DB-01", EngineSQLServer, "Blocking", "MSSQL_Blocking")
	fp2 := Fingerprint("prod-db-01", EngineSQLServer, "blocking", "mssql_blocking")
	if fp1 != fp2 {
		t.Errorf("fingerprints should be case-insensitive: %q != %q", fp1, fp2)
	}
}

func TestFingerprint_TrimsSpaces(t *testing.T) {
	fp1 := Fingerprint(" prod-db-01 ", EngineSQLServer, " blocking ", " mssql_blocking ")
	fp2 := Fingerprint("prod-db-01", EngineSQLServer, "blocking", "mssql_blocking")
	if fp1 != fp2 {
		t.Errorf("fingerprints should trim spaces: %q != %q", fp1, fp2)
	}
}

func TestFingerprint_Length(t *testing.T) {
	fp := Fingerprint("prod-db-01", EngineSQLServer, "blocking", "mssql_blocking")
	if len(fp) != 32 {
		t.Errorf("fingerprint length = %d, want 32", len(fp))
	}
}
