// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Test suite for SQL parser.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package sqlparse

import (
	"context"
	"testing"

	"github.com/rsharma155/sql_optima/internal/missing_index/types"
)

func TestParseSimpleSelect(t *testing.T) {
	query := "SELECT id, name FROM users WHERE status = 'active'"

	analysis, err := Parse(context.TODO(), query)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if analysis == nil {
		t.Fatal("Analysis should not be nil")
	}

	if analysis.StatementType != "SELECT" {
		t.Errorf("Expected SELECT statement, got %s", analysis.StatementType)
	}
}

func TestExtractTables(t *testing.T) {
	tests := []struct {
		query     string
		expTables int
	}{
		{"SELECT * FROM users", 1},
		{"SELECT * FROM public.users", 1},
		{"SELECT * FROM orders WHERE id = 1", 1},
		{"SELECT * FROM users u JOIN orders o ON u.id = o.user_id", 2},
		{"SELECT * FROM a JOIN b ON a.id = b.id JOIN c ON b.id = c.id", 3},
	}

	for _, tt := range tests {
		tables, _ := extractTables(tt.query)
		if len(tables) != tt.expTables {
			t.Errorf("Query: %s, expected %d tables, got %d", tt.query, tt.expTables, len(tables))
		}
	}
}

func TestExtractPredicates(t *testing.T) {
	query := "SELECT * FROM users WHERE status = 'active' AND age > 18"

	predicates := extractPredicates(query, map[string]types.TableRef{})

	if len(predicates) == 0 {
		t.Error("Expected predicates to be extracted")
	}
}

func TestExtractOrderBy(t *testing.T) {
	query := "SELECT * FROM users ORDER BY created_at DESC, name"

	orderBy := extractOrderBy(query)

	if len(orderBy) == 0 {
		t.Error("Expected ORDER BY columns to be extracted")
	}

	if orderBy[0].Column != "created_at" {
		t.Errorf("Expected first column 'created_at', got %s", orderBy[0].Column)
	}
}

func TestExtractProjectedColumns(t *testing.T) {
	query := "SELECT id, name, email FROM users"

	cols := extractProjectedColumns(query)

	if len(cols) != 3 {
		t.Errorf("Expected 3 columns, got %d", len(cols))
	}
}

func TestEmptyQuery(t *testing.T) {
	analysis, err := Parse(context.TODO(), "")
	if err != nil {
		t.Fatalf("Parse failed for empty query: %v", err)
	}

	if analysis != nil {
		t.Error("Expected nil analysis for empty query")
	}
}

func TestExtractLimit(t *testing.T) {
	query := "SELECT * FROM users LIMIT 10"

	limit := extractLimit(query)

	if limit == nil {
		t.Fatal("Expected limit to be extracted")
	}

	if *limit != 10 {
		t.Errorf("Expected limit 10, got %d", *limit)
	}
}
