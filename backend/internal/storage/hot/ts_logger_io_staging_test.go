// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Unit tests for staging IO row storage.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package hot

import (
	"reflect"
	"testing"
)

// TestIOStagingRowHasAllRequiredFields verifies that IOStagingRow has the
// 13 data columns that map to staging.sqlserver_io_raw (capture_timestamp and
// server_id are added by the logger, not stored in the row).
func TestIOStagingRowHasAllRequiredFields(t *testing.T) {
	expectedFields := []string{
		"DatabaseID",
		"DatabaseName",
		"FileID",
		"TypeDesc",
		"PhysicalName",
		"NumOfReads",
		"NumOfWrites",
		"NumOfBytesRead",
		"NumOfBytesWritten",
		"IoStallReadMs",
		"IoStallWriteMs",
		"IoStall",
		"SizeOnDiskBytes",
	}

	rt := reflect.TypeOf(IOStagingRow{})
	fieldMap := make(map[string]bool, rt.NumField())
	for i := 0; i < rt.NumField(); i++ {
		fieldMap[rt.Field(i).Name] = true
	}

	for _, want := range expectedFields {
		if !fieldMap[want] {
			t.Errorf("IOStagingRow is missing required field %q", want)
		}
	}
}

// TestIOStagingRowZeroValue ensures a zero-value IOStagingRow is safe to use.
func TestIOStagingRowZeroValue(t *testing.T) {
	var row IOStagingRow
	if row.DatabaseID != 0 {
		t.Errorf("expected zero DatabaseID, got %d", row.DatabaseID)
	}
	if row.NumOfReads != 0 {
		t.Errorf("expected zero NumOfReads, got %d", row.NumOfReads)
	}
	if row.DatabaseName != "" {
		t.Errorf("expected empty DatabaseName, got %q", row.DatabaseName)
	}
}

// TestLogStagingIORowsCompileCheck is a compile-time assertion that
// TimescaleLogger implements LogStagingIORows.
func TestLogStagingIORowsCompileCheck(t *testing.T) {
	// If LogStagingIORows changes signature this file will fail to compile.
	_ = (*TimescaleLogger).LogStagingIORows
}
