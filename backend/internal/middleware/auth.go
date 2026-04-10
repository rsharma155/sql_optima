// Package middleware provides HTTP middleware for authentication and security.
// It includes JWT authentication with role-based access control.
package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// JWTSecret is the signing key for JWT tokens. Set via environment variable.
var JWTSecret = []byte("sql-optima-jwt-secret-change-in-production")

// SetJWTSecret sets the JWT secret for authentication.
func SetJWTSecret(secret []byte) {
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

// ValidateToken parses and validates a JWT, returning the claims.
func ValidateToken(tokenString string) (*AuthClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &AuthClaims{}, func(t *jwt.Token) (interface{}, error) {
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

// RequireAuth is middleware that validates JWT and optionally checks role.
// Usage: RequireAuth("") for any authenticated user, RequireAuth("admin") for admin only.
func RequireAuth(requiredRole string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{"error": "missing authorization header"})
				return
			}

			// Extract "Bearer <token>"
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || parts[0] != "Bearer" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{"error": "invalid authorization format"})
				return
			}

			claims, err := ValidateToken(parts[1])
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

// AuthRequired is the legacy middleware (any authenticated user, no role check).
func AuthRequired(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		RequireAuth("")(next).ServeHTTP(w, r)
	}
}
