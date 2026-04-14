// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: OpenID Connect authentication integration for external identity provider verification.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package middleware

import (
	"context"
	"strings"
	"sync"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt/v5"
)

var (
	oidcMu      sync.RWMutex
	oidcVerifier *oidc.IDTokenVerifier
)

// SetOIDCVerifier registers an OIDC ID token verifier (e.g. from InitOIDC). Pass nil to disable.
func SetOIDCVerifier(v *oidc.IDTokenVerifier) {
	oidcMu.Lock()
	defer oidcMu.Unlock()
	oidcVerifier = v
}

func getOIDCVerifier() *oidc.IDTokenVerifier {
	oidcMu.RLock()
	defer oidcMu.RUnlock()
	return oidcVerifier
}

// InitOIDC configures verification for bearer tokens from the given issuer (Keycloak, Auth0, etc.).
func InitOIDC(ctx context.Context, issuerURL, audience string) (*oidc.IDTokenVerifier, error) {
	provider, err := oidc.NewProvider(ctx, issuerURL)
	if err != nil {
		return nil, err
	}
	verifier := provider.Verifier(&oidc.Config{ClientID: audience})
	SetOIDCVerifier(verifier)
	return verifier, nil
}

func validateOIDCToken(ctx context.Context, raw string) (*AuthClaims, error) {
	v := getOIDCVerifier()
	if v == nil {
		return nil, jwt.ErrTokenInvalidClaims
	}
	tok, err := v.Verify(ctx, raw)
	if err != nil {
		return nil, err
	}
	var m map[string]interface{}
	if err := tok.Claims(&m); err != nil {
		return nil, err
	}
	username := firstString(m, "preferred_username", "email", "name", "sub")
	role := mapOIDCRoleClaim(m)
	sub, _ := m["sub"].(string)
	return &AuthClaims{
		UserID:   0,
		Username: username,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: sub,
		},
	}, nil
}

func firstString(m map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		if s, ok := m[k].(string); ok && s != "" {
			return s
		}
	}
	return "unknown"
}

func mapOIDCRoleClaim(m map[string]interface{}) string {
	if s, ok := m["optima_role"].(string); ok {
		return normalizeAppRole(s)
	}
	// Keycloak-style resource_access
	if ra, ok := m["resource_access"].(map[string]interface{}); ok {
		for _, v := range ra {
			rm, ok := v.(map[string]interface{})
			if !ok {
				continue
			}
			roles, ok := rm["roles"].([]interface{})
			if !ok {
				continue
			}
			for _, r := range roles {
				rs, _ := r.(string)
				nr := normalizeAppRole(rs)
				if nr == "admin" {
					return "admin"
				}
				if nr == "dba" {
					return "dba"
				}
			}
		}
	}
	return "viewer"
}

func normalizeAppRole(r string) string {
	switch strings.ToLower(r) {
	case "admin", "administrator":
		return "admin"
	case "dba", "db_admin":
		return "dba"
	case "viewer", "read", "readonly":
		return "viewer"
	default:
		return "viewer"
	}
}
