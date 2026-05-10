// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: TDD tests for ClassifyQueryType — pure function, no external dependencies.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import "testing"

func TestClassifyQueryType_Select(t *testing.T) {
	cases := []string{
		"SELECT * FROM users",
		"select id from orders",
		"SELECT\tid FROM t",
		"  SELECT 1",
	}
	for _, q := range cases {
		if got := ClassifyQueryType(q); got != "S" {
			t.Errorf("ClassifyQueryType(%q) = %q, want S", q, got)
		}
	}
}

func TestClassifyQueryType_With(t *testing.T) {
	cases := []string{
		"WITH cte AS (SELECT 1) SELECT * FROM cte",
		"with recursive r AS (...) SELECT ...",
	}
	for _, q := range cases {
		if got := ClassifyQueryType(q); got != "S" {
			t.Errorf("ClassifyQueryType(%q) = %q, want S", q, got)
		}
	}
}

func TestClassifyQueryType_Insert(t *testing.T) {
	cases := []string{
		"INSERT INTO users(name) VALUES ($1)",
		"insert into orders select ...",
	}
	for _, q := range cases {
		if got := ClassifyQueryType(q); got != "I" {
			t.Errorf("ClassifyQueryType(%q) = %q, want I", q, got)
		}
	}
}

func TestClassifyQueryType_Update(t *testing.T) {
	cases := []string{
		"UPDATE users SET name = $1 WHERE id = $2",
		"update orders set status='done'",
	}
	for _, q := range cases {
		if got := ClassifyQueryType(q); got != "U" {
			t.Errorf("ClassifyQueryType(%q) = %q, want U", q, got)
		}
	}
}

func TestClassifyQueryType_Delete(t *testing.T) {
	cases := []string{
		"DELETE FROM users WHERE id = $1",
		"delete from sessions",
	}
	for _, q := range cases {
		if got := ClassifyQueryType(q); got != "D" {
			t.Errorf("ClassifyQueryType(%q) = %q, want D", q, got)
		}
	}
}

func TestClassifyQueryType_DDL(t *testing.T) {
	cases := []struct {
		q    string
		want string
	}{
		{"CREATE TABLE foo (id INT)", "E"},
		{"ALTER TABLE foo ADD COLUMN bar TEXT", "E"},
		{"DROP TABLE foo", "E"},
		{"TRUNCATE TABLE foo", "E"},
		{"create index ...", "E"},
	}
	for _, tc := range cases {
		if got := ClassifyQueryType(tc.q); got != tc.want {
			t.Errorf("ClassifyQueryType(%q) = %q, want %q", tc.q, got, tc.want)
		}
	}
}

func TestClassifyQueryType_Other(t *testing.T) {
	cases := []string{
		"CALL my_proc()",
		"MERGE INTO ...",
		"COPY users FROM stdin",
		"",
		"  ",
		"/* comment */ VACUUM",
	}
	for _, q := range cases {
		if got := ClassifyQueryType(q); got != "O" {
			t.Errorf("ClassifyQueryType(%q) = %q, want O", q, got)
		}
	}
}

func TestClassifyQueryType_LeadingWhitespace(t *testing.T) {
	if got := ClassifyQueryType("  \t\nSELECT 1"); got != "S" {
		t.Errorf("got %q, want S for leading whitespace", got)
	}
}
