// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Tests for OIDC group → role mapping.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package middleware

import "testing"

func TestParseOIDCGroupRoleMap(t *testing.T) {
	m := ParseOIDCGroupRoleMap("sql-optima-admins:admin, sql-optima-dbas:dba,viewers:viewer")
	if m["sql-optima-admins"] != RoleAdmin || m["sql-optima-dbas"] != RoleDBA || m["viewers"] != RoleViewer {
		t.Fatalf("unexpected map: %+v", m)
	}
}

func TestRoleFromOIDCGroups_PrefersAdmin(t *testing.T) {
	t.Cleanup(func() { SetOIDCGroupRoleMapping("groups", nil) })
	SetOIDCGroupRoleMapping("groups", map[string]string{
		"admins": RoleAdmin,
		"dbas":   RoleDBA,
		"reads":  RoleViewer,
	})
	role := mapOIDCRoleClaim(map[string]interface{}{
		"groups": []interface{}{"reads", "dbas", "admins"},
	})
	if role != RoleAdmin {
		t.Fatalf("got %q want admin", role)
	}
}

func TestRoleFromOIDCGroups_AzureRolesClaim(t *testing.T) {
	t.Cleanup(func() { SetOIDCGroupRoleMapping("groups", nil) })
	SetOIDCGroupRoleMapping("roles", map[string]string{"App.DBA": RoleDBA})
	role := mapOIDCRoleClaim(map[string]interface{}{
		"roles": []string{"App.DBA"},
	})
	if role != RoleDBA {
		t.Fatalf("got %q want dba", role)
	}
}
