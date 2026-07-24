// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Tests for cold/hot federated lookback policy helpers.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package federation_test

import (
	"strings"
	"testing"
	"time"

	"github.com/rsharma155/sql_optima/internal/storage/cold/federation"
)

func TestNeedsColdLookback(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	hotDays := 30

	fromHot := now.Add(-10 * 24 * time.Hour)
	if federation.NeedsColdLookback(fromHot, now, hotDays) {
		t.Fatal("10d lookback should stay hot-only when hotDays=30")
	}

	fromCold := now.Add(-45 * 24 * time.Hour)
	if !federation.NeedsColdLookback(fromCold, now, hotDays) {
		t.Fatal("45d lookback should need cold when hotDays=30")
	}
}

func TestSplitRange_HotThenCold(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	from := now.Add(-60 * 24 * time.Hour)
	hot, cold, ok := federation.SplitRange(from, now, 30)
	if !ok {
		t.Fatal("expected split")
	}
	if cold.To.After(hot.From) && !cold.To.Equal(hot.From) {
		// cold ends where hot begins
	}
	if !cold.To.Equal(hot.From) {
		t.Fatalf("cold.To (%v) should equal hot.From (%v)", cold.To, hot.From)
	}
	if !hot.To.Equal(now) || !cold.From.Equal(from) {
		t.Fatalf("bounds wrong hot=%+v cold=%+v", hot, cold)
	}
}

func TestSanitizeIcebergTableName(t *testing.T) {
	if got := federation.SanitizeIcebergTableName("sqlserver_cpu_history"); got != "sqlserver_cpu_history" {
		t.Fatalf("got %q", got)
	}
	if federation.SanitizeIcebergTableName("evil;drop") != "" {
		t.Fatal("rejected name should be empty")
	}
	if federation.SanitizeIcebergTableName("monitor.pg_db_load_ts") != "pg_db_load_ts" {
		t.Fatal("schema prefix should be stripped to leaf identifier")
	}
}

func TestBuildIcebergHistorySQL_AllowlistAndInjection(t *testing.T) {
	sid := "11111111-1111-1111-1111-111111111111"
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)

	sql, err := federation.BuildIcebergHistorySQL("sqlserver_cpu_history", sid, from, to)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, "iceberg.default.sqlserver_cpu_history") {
		t.Fatalf("unexpected sql: %s", sql)
	}
	if !strings.Contains(sql, sid) {
		t.Fatal("server_id missing")
	}

	if _, err := federation.BuildIcebergHistorySQL("not_a_real_table", sid, from, to); err == nil {
		t.Fatal("expected allowlist rejection")
	}
	if _, err := federation.BuildIcebergHistorySQL("sqlserver_cpu_history'; DROP TABLE x--", sid, from, to); err == nil {
		t.Fatal("expected injection rejection")
	}
	if _, err := federation.BuildIcebergHistorySQL("sqlserver_cpu_history", "not-a-uuid", from, to); err == nil {
		t.Fatal("expected invalid server_id")
	}
}

func TestMergeTimeSeries_HotWinsOverlap(t *testing.T) {
	t1 := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 1, 1, 11, 0, 0, 0, time.UTC)
	cold := []map[string]interface{}{
		{"timestamp": t1, "sql_process": 10.0},
		{"timestamp": t2, "sql_process": 20.0},
	}
	hot := []map[string]interface{}{
		{"timestamp": t2, "sql_process": 99.0},
	}
	merged := federation.MergeTimeSeries(cold, hot)
	if len(merged) != 2 {
		t.Fatalf("len=%d", len(merged))
	}
	if merged[1]["sql_process"].(float64) != 99.0 {
		t.Fatalf("hot should win overlap: %+v", merged[1])
	}
}
