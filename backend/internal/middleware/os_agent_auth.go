// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Scoped machine JWT for OS collector ingest (write-only; no dashboard access).
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package middleware

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const (
	// RoleOSAgent is a non-interactive role used only for host metric push.
	RoleOSAgent = "os_agent"
	// ScopeOSMetricsWrite authorizes POST /api/os/metrics only.
	ScopeOSMetricsWrite = "os_metrics:write"
	// DefaultOSAgentTokenTTL bounds blast radius if a host is compromised.
	DefaultOSAgentTokenTTL = 90 * 24 * time.Hour
	maxOSAgentTokenTTL     = 365 * 24 * time.Hour
)

// GenerateOSAgentToken issues a scoped HS256 JWT bound to a monitored server UUID.
// The token cannot access dashboard or admin APIs (see RequireAuth rejection of RoleOSAgent).
// jti is returned so operators can revoke without re-parsing the token.
func GenerateOSAgentToken(serverID uuid.UUID, ttl time.Duration) (token string, jti string, err error) {
	if len(JWTSecret) < 32 {
		return "", "", errors.New("jwt secret is not configured")
	}
	if serverID == uuid.Nil {
		return "", "", errors.New("server_id is required")
	}
	if ttl <= 0 {
		ttl = DefaultOSAgentTokenTTL
	}
	if ttl > maxOSAgentTokenTTL {
		ttl = maxOSAgentTokenTTL
	}

	jti = uuid.NewString()
	claims := AuthClaims{
		UserID:   0,
		Username: "os-agent:" + serverID.String(),
		Role:     RoleOSAgent,
		Scope:    ScopeOSMetricsWrite,
		ServerID: serverID.String(),
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().UTC().Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(time.Now().UTC()),
			Issuer:    "sql-optima",
			Subject:   serverID.String(),
			ID:        jti,
		},
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	token, err = t.SignedString(JWTSecret)
	if err != nil {
		return "", "", err
	}
	return token, jti, nil
}

// HasOSMetricsWriteScope reports whether claims may push host metrics.
func (c *AuthClaims) HasOSMetricsWriteScope() bool {
	if c == nil {
		return false
	}
	if c.Role == RoleOSAgent && c.Scope == ScopeOSMetricsWrite {
		return true
	}
	return IsPrivilegedRole(c.Role)
}

// extractBearerOrCookie returns the raw JWT from Authorization or auth cookie.
func extractBearerOrCookie(r *http.Request) string {
	if authHeader := r.Header.Get("Authorization"); authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && parts[0] == "Bearer" {
			return parts[1]
		}
	}
	if c, err := r.Cookie(AuthCookieName); err == nil {
		return c.Value
	}
	return ""
}

func writeAuthJSON(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// RequireOSMetricsAuth allows dba/admin JWTs or a scoped os_agent machine token.
// Use only on POST /api/os/metrics.
func RequireOSMetricsAuth() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenString := extractBearerOrCookie(r)
			if tokenString == "" {
				writeAuthJSON(w, http.StatusUnauthorized, "missing authorization header or cookie")
				return
			}
			claims, err := ValidateTokenWithContext(r.Context(), tokenString)
			if err != nil {
				writeAuthJSON(w, http.StatusUnauthorized, "invalid or expired token")
				return
			}
			if !claims.HasOSMetricsWriteScope() {
				writeAuthJSON(w, http.StatusForbidden, "insufficient permissions")
				return
			}
			if claims.Role == RoleOSAgent {
				jti := claims.ID
				if checker := getOSAgentRevokeChecker(); checker != nil && jti != "" {
					revoked, rerr := checker.IsRevoked(r.Context(), jti)
					if rerr != nil {
						writeAuthJSON(w, http.StatusServiceUnavailable, "auth check unavailable")
						return
					}
					if revoked {
						writeAuthJSON(w, http.StatusUnauthorized, "token revoked")
						return
					}
				}
			}
			ctx := WithAuthClaims(r.Context(), claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
