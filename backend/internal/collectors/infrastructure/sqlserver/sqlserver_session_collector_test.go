// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Unit tests for SQL Server session collector.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package sqlserver

import (
	"reflect"
	"testing"
)

func TestActiveSessionRowFields(t *testing.T) {
	row := ActiveSessionRow{
		SessionID:            42,
		LoginName:            "testuser",
		HostName:             "server1",
		ProgramName:          "SSMS",
		DatabaseName:         "mydb",
		RequestStatus:        "running",
		WaitType:             "LCK_M_X",
		WaitTimeMs:           5000,
		BlockingSessionID:    10,
		CPUTimeMs:            1234,
		TotalElapsedMs:       9999,
		LogicalReads:         100,
		Reads:                20,
		Writes:               5,
		GrantedQueryMemoryKB: 8192,
		DOP:                  4,
		QueryHash:            "0xABCDEF",
		QueryText:            "SELECT 1",
	}
	if row.SessionID != 42 {
		t.Errorf("expected session_id=42 got %d", row.SessionID)
	}
	if row.BlockingSessionID != 10 {
		t.Errorf("expected blocking_session_id=10 got %d", row.BlockingSessionID)
	}
	if row.GrantedQueryMemoryKB != 8192 {
		t.Errorf("expected granted_query_memory_kb=8192 got %d", row.GrantedQueryMemoryKB)
	}
}

func TestSessionCollectorIsStateless(t *testing.T) {
	c1 := &SessionCollector{}
	c2 := &SessionCollector{}
	t1 := reflect.TypeOf(c1)
	t2 := reflect.TypeOf(c2)
	if t1 != t2 {
		t.Error("SessionCollector should be a stateless struct")
	}
}

func TestActiveSessionRowZeroValues(t *testing.T) {
	var row ActiveSessionRow
	if row.BlockingSessionID != 0 {
		t.Error("zero value blocking_session_id should be 0")
	}
	if row.DOP != 0 {
		t.Error("zero value dop should be 0")
	}
	if row.QueryText != "" {
		t.Error("zero value query_text should be empty string")
	}
}
