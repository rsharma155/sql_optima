// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: OIDC group claim → SQL Optima role mapping for enterprise SSO.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package middleware

import (
	"strings"
	"sync"
)

var (
	oidcGroupMu       sync.RWMutex
	oidcGroupClaim    = "groups"
	oidcGroupRoleMap  = map[string]string{} // group name → role
)

// SetOIDCGroupRoleMapping configures which IdP groups map to admin/dba/viewer.
// claim is the JWT claim holding groups (default "groups"; Azure AD often "roles").
// mapping keys are IdP group names; values are optima roles.
func SetOIDCGroupRoleMapping(claim string, mapping map[string]string) {
	oidcGroupMu.Lock()
	defer oidcGroupMu.Unlock()
	if strings.TrimSpace(claim) != "" {
		oidcGroupClaim = strings.TrimSpace(claim)
	}
	clean := make(map[string]string, len(mapping))
	for g, role := range mapping {
		g = strings.TrimSpace(g)
		if g == "" {
			continue
		}
		clean[g] = NormalizeRole(role)
	}
	oidcGroupRoleMap = clean
}

// ParseOIDCGroupRoleMap parses "groupA:admin,groupB:dba,groupC:viewer".
func ParseOIDCGroupRoleMap(raw string) map[string]string {
	out := map[string]string{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		kv := strings.SplitN(part, ":", 2)
		if len(kv) != 2 {
			continue
		}
		g := strings.TrimSpace(kv[0])
		r := strings.TrimSpace(kv[1])
		if g == "" || r == "" {
			continue
		}
		out[g] = NormalizeRole(r)
	}
	return out
}

func roleFromOIDCGroups(m map[string]interface{}) string {
	oidcGroupMu.RLock()
	claim := oidcGroupClaim
	mapping := oidcGroupRoleMap
	oidcGroupMu.RUnlock()
	if len(mapping) == 0 {
		return ""
	}
	groups := extractStringSliceClaim(m, claim)
	best := ""
	for _, g := range groups {
		role, ok := mapping[g]
		if !ok {
			continue
		}
		if role == RoleAdmin {
			return RoleAdmin
		}
		if role == RoleDBA {
			best = RoleDBA
		} else if role == RoleViewer && best == "" {
			best = RoleViewer
		}
	}
	return best
}

func extractStringSliceClaim(m map[string]interface{}, claim string) []string {
	v, ok := m[claim]
	if !ok || v == nil {
		return nil
	}
	switch t := v.(type) {
	case []interface{}:
		out := make([]string, 0, len(t))
		for _, x := range t {
			if s, ok := x.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return t
	case string:
		// Single group as string
		if t != "" {
			return []string{t}
		}
	}
	return nil
}
