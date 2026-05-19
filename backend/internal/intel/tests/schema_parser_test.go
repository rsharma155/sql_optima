// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Health Intelligence Engine
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package tests




import (
	"testing"

	"github.com/rsharma155/sql_optima/internal/intel/schema_parser"
)

const sampleCreateTable = `
CREATE TABLE cpu_metrics (
    server_id INT NOT NULL,
    collection_time TIMESTAMP NOT NULL,
    cpu_pct FLOAT NOT NULL,
    runnable_tasks_count INT,
    signal_wait_time_ms FLOAT,
    PRIMARY KEY (server_id, collection_time)
);
`

func TestParseSingleTable(t *testing.T) {
	result := schema_parser.ParseCreateTable(sampleCreateTable)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Name != "cpu_metrics" {
		t.Error("expected cpu_metrics, got ", result.Name)
	}
	if len(result.Columns) != 5 {
		t.Error("expected 5 columns, got 0", len(result.Columns))
	}
}

func TestParseTimestampColumns(t *testing.T) {
	result := schema_parser.ParseCreateTable(sampleCreateTable)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	found := false
	for _, c := range result.TimestampColumns {
		if c == "collection_time" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected collection_time in timestamp columns")
	}
}

func TestParseDimensionColumns(t *testing.T) {
	result := schema_parser.ParseCreateTable(sampleCreateTable)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	found := false
	for _, c := range result.DimensionColumns {
		if c == "server_id" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected server_id in dimension columns")
	}
}

func TestParseMultipleTables(t *testing.T) {
	multiSQL := `
	CREATE TABLE cpu_metrics (id INT, collection_time TIMESTAMP, cpu_pct FLOAT);
	CREATE TABLE memory_metrics (id INT, collection_time TIMESTAMP, memory_pct FLOAT);
	`
	results := schema_parser.ParseMultipleTables(multiSQL)
	if len(results) != 2 {
		t.Error("expected 2 tables, got 0", len(results))
	}
}

func TestEmptyInput(t *testing.T) {
	result := schema_parser.ParseCreateTable("")
	if result != nil {
		t.Error("expected nil for empty input")
	}
}

func TestInvalidInput(t *testing.T) {
	result := schema_parser.ParseCreateTable("SELECT * FROM foo")
	if result != nil {
		t.Error("expected nil for invalid input")
	}
}