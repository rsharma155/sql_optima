// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Tests for canonical RBAC role helpers.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package middleware

import "testing"

func TestNormalizeRole(t *testing.T) {
	cases := map[string]string{
		"admin":          RoleAdmin,
		"Administrator":  RoleAdmin,
		"dba":            RoleDBA,
		"db_admin":       RoleDBA,
		"viewer":         RoleViewer,
		"readonly":       RoleViewer,
		"unknown-role":   RoleViewer,
		RoleOSAgent:      RoleOSAgent,
	}
	for in, want := range cases {
		if got := NormalizeRole(in); got != want {
			t.Fatalf("NormalizeRole(%q)=%q want %q", in, got, want)
		}
	}
}

func TestIsPrivilegedRole(t *testing.T) {
	if !IsPrivilegedRole(RoleAdmin) || !IsPrivilegedRole(RoleDBA) {
		t.Fatal("admin/dba should be privileged")
	}
	if IsPrivilegedRole(RoleViewer) || IsPrivilegedRole(RoleOSAgent) {
		t.Fatal("viewer/os_agent must not be privileged")
	}
}
