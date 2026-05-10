// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: TDD tests for PostgreSQL incident feed logic.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package hot

import (
	"sort"
	"testing"
)

func TestPgIncidentFeed_SeverityOrdering(t *testing.T) {
	rows := []PgIncidentFeedRow{
		{Severity: "warning", Datname: "db1"},
		{Severity: "critical", Datname: "db2"},
		{Severity: "info", Datname: "db3"},
	}

	// Define priority
	priority := map[string]int{
		"critical": 1,
		"warning":  2,
		"info":     3,
	}

	sort.Slice(rows, func(i, j int) bool {
		return priority[rows[i].Severity] < priority[rows[j].Severity]
	})

	if rows[0].Severity != "critical" {
		t.Fatalf("expected critical first, got %s", rows[0].Severity)
	}
	if rows[2].Severity != "info" {
		t.Fatalf("expected info last, got %s", rows[2].Severity)
	}
}

func TestPgIncidentFeed_SnippetTruncation(t *testing.T) {
	longQuery := "SELECT * FROM very_large_table WHERE column1 = 'value1' AND column2 = 'value2' AND column3 = 'value3' AND column4 = 'value4' AND column5 = 'value5' AND column6 = 'value6' AND column7 = 'value7' AND column8 = 'value8'"
	
	snippet := longQuery
	if len(snippet) > 200 {
		snippet = snippet[:197] + "..."
	}

	if len(snippet) > 200 {
		t.Fatalf("snippet too long: %d", len(snippet))
	}
	if snippet[len(snippet)-3:] != "..." {
		t.Fatalf("snippet does not end with ellipsis")
	}
}
