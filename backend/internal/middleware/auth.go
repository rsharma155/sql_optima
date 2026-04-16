// Package middleware provides HTTP middleware for authentication and security.
// It includes JWT authentication with role-based access control.
// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: JWT authentication middleware with token generation, validation, and role-based access control.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// JWTSecret is the signing key for JWT tokens. It must be set at startup.
var JWTSecret []byte

// SetJWTSecret sets the JWT secret for authentication.
func SetJWTSecret(secret []byte) {
	if len(secret) < 32 {
		panic("JWT secret must be at least 32 bytes")
	}
	JWTSecret = secret
}

// AuthClaims represents the JWT payload.
type AuthClaims struct {
	UserID   int    `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

// GenerateToken creates a signed JWT for the given user.
func GenerateToken(userID int, username, role string) (string, error) {
	if len(JWTSecret) < 32 {
		return "", errors.New("jwt secret is not configured")
	}

	claims := AuthClaims{
		UserID:   userID,
		Username: username,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "sql-optima",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(JWTSecret)
}

// ValidateToken parses and validates a bearer token (OIDC if configured, otherwise local HS256 JWT).
func ValidateToken(tokenString string) (*AuthClaims, error) {
	return ValidateTokenWithContext(context.Background(), tokenString)
}

// ValidateTokenWithContext is like ValidateToken but uses ctx for OIDC verification.
func ValidateTokenWithContext(ctx context.Context, tokenString string) (*AuthClaims, error) {
	if getOIDCVerifier() != nil {
		return validateOIDCToken(ctx, tokenString)
	}
	return validateLocalJWT(tokenString)
}

func validateLocalJWT(tokenString string) (*AuthClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &AuthClaims{}, func(t *jwt.Token) (interface{}, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, jwt.ErrTokenUnverifiable
		}
		return JWTSecret, nil
	})
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*AuthClaims)
	if !ok || !token.Valid {
		return nil, jwt.ErrTokenInvalidClaims
	}

	return claims, nil
}

// contextKey is a custom type for context keys.
type contextKey string

const authContextKey contextKey = "auth_claims"

const (
	// AuthCookieName is the HttpOnly cookie that carries the JWT for browser clients.
	AuthCookieName = "sql_optima_token"
)

// RequireAuth is middleware that validates JWT and optionally checks role.
// Usage: RequireAuth("") for any authenticated user, RequireAuth("admin") for admin only.
//
// Token resolution order:
//  1. Authorization: Bearer <token> header (API clients, curl, SDKs).
//  2. HttpOnly cookie "sql_optima_token" (browser clients).
func RequireAuth(requiredRole string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var tokenString string

			// 1. Prefer Authorization header (API / non-browser callers).
			if authHeader := r.Header.Get("Authorization"); authHeader != "" {
				parts := strings.SplitN(authHeader, " ", 2)
				if len(parts) == 2 && parts[0] == "Bearer" {
					tokenString = parts[1]
				}
			}

			// 2. Fall back to HttpOnly cookie (browser callers).
			if tokenString == "" {
				if c, err := r.Cookie(AuthCookieName); err == nil {
					tokenString = c.Value
				}
			}

			if tokenString == "" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{"error": "missing authorization header or cookie"})
				return
			}

			claims, err := ValidateTokenWithContext(r.Context(), tokenString)
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{"error": "invalid or expired token"})
				return
			}

			// Role check
			if requiredRole != "" && claims.Role != requiredRole {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				json.NewEncoder(w).Encode(map[string]string{"error": "insufficient permissions"})
				return
			}

			// Store claims in request context for downstream handlers
			ctx := context.WithValue(r.Context(), authContextKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetAuthClaims extracts the authenticated user's claims from the request context.
func GetAuthClaims(r *http.Request) *AuthClaims {
	claims, ok := r.Context().Value(authContextKey).(*AuthClaims)
	if !ok {
		return nil
	}
	return claims
}

// WithAuthClaims returns a new context carrying the given claims.
// Intended for handler tests that need to simulate authenticated requests.
func WithAuthClaims(ctx context.Context, claims *AuthClaims) context.Context {
	return context.WithValue(ctx, authContextKey, claims)
}

// AuthRequired is the legacy middleware (any authenticated user, no role check).
func AuthRequired(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		RequireAuth("")(next).ServeHTTP(w, r)
	}
}
