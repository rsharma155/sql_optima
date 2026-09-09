// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Canonical RBAC role string constants used across middleware and handlers.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package middleware

import "strings"

// Interactive user roles (JWT claim "role").
const (
	RoleAdmin  = "admin"
	RoleDBA    = "dba"
	RoleViewer = "viewer"
)

// IsPrivilegedRole reports whether role may perform DBA/admin mutations.
func IsPrivilegedRole(role string) bool {
	return role == RoleAdmin || role == RoleDBA
}

// NormalizeRole maps aliases to canonical role constants (OIDC / local).
func NormalizeRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case RoleAdmin, "administrator":
		return RoleAdmin
	case RoleDBA, "db_admin":
		return RoleDBA
	case RoleViewer, "read", "readonly":
		return RoleViewer
	default:
		if role == RoleOSAgent {
			return RoleOSAgent
		}
		return RoleViewer
	}
}
