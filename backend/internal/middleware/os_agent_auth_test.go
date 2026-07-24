// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Tests for scoped OS-agent JWT (write-only os_metrics ingest).
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestGenerateOSAgentToken_HasScopedClaims(t *testing.T) {
	secret := make([]byte, 32)
	for i := range secret {
		secret[i] = byte(i + 1)
	}
	SetJWTSecret(secret)

	sid := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	tok, jti, err := GenerateOSAgentToken(sid, 48*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if jti == "" {
		t.Fatal("expected non-empty jti")
	}
	claims, err := ValidateToken(tok)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Role != RoleOSAgent {
		t.Fatalf("role = %q", claims.Role)
	}
	if claims.Scope != ScopeOSMetricsWrite {
		t.Fatalf("scope = %q", claims.Scope)
	}
	if claims.ServerID != sid.String() {
		t.Fatalf("server_id = %q", claims.ServerID)
	}
	if !claims.HasOSMetricsWriteScope() {
		t.Fatal("expected HasOSMetricsWriteScope")
	}
}

func TestRequireAuth_RejectsOSAgentOnGeneralRoutes(t *testing.T) {
	secret := make([]byte, 32)
	for i := range secret {
		secret[i] = byte(i + 3)
	}
	SetJWTSecret(secret)
	sid := uuid.New()
	tok, _, err := GenerateOSAgentToken(sid, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	h := RequireAuth("")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 (os_agent must not access general routes)", rr.Code)
	}
}

func TestRequireOSMetricsAuth_AllowsOSAgentAndAdmin(t *testing.T) {
	secret := make([]byte, 32)
	for i := range secret {
		secret[i] = byte(i + 7)
	}
	SetJWTSecret(secret)

	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})
	h := RequireOSMetricsAuth()(okHandler)

	// OS agent token
	tok, _, err := GenerateOSAgentToken(uuid.New(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/os/metrics", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("os_agent status = %d, want 202", rr.Code)
	}

	// Admin JWT still allowed
	adminTok, err := GenerateToken(1, "admin", "admin")
	if err != nil {
		t.Fatal(err)
	}
	req2 := httptest.NewRequest(http.MethodPost, "/api/os/metrics", nil)
	req2.Header.Set("Authorization", "Bearer "+adminTok)
	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusAccepted {
		t.Fatalf("admin status = %d, want 202", rr2.Code)
	}

	// Viewer must not ingest
	viewerTok, err := GenerateToken(2, "viewer", "viewer")
	if err != nil {
		t.Fatal(err)
	}
	req3 := httptest.NewRequest(http.MethodPost, "/api/os/metrics", nil)
	req3.Header.Set("Authorization", "Bearer "+viewerTok)
	rr3 := httptest.NewRecorder()
	h.ServeHTTP(rr3, req3)
	if rr3.Code != http.StatusForbidden {
		t.Fatalf("viewer status = %d, want 403", rr3.Code)
	}
}

func TestGenerateOSAgentToken_RejectsNilServer(t *testing.T) {
	secret := make([]byte, 32)
	for i := range secret {
		secret[i] = byte(i + 9)
	}
	SetJWTSecret(secret)
	_, _, err := GenerateOSAgentToken(uuid.Nil, time.Hour)
	if err == nil {
		t.Fatal("expected error for nil server_id")
	}
}

func TestRequireOSMetricsAuth_RejectsRevokedJTI(t *testing.T) {
	secret := make([]byte, 32)
	for i := range secret {
		secret[i] = byte(i + 13)
	}
	SetJWTSecret(secret)
	t.Cleanup(func() { SetOSAgentRevokeChecker(nil) })

	tok, jti, err := GenerateOSAgentToken(uuid.New(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	SetOSAgentRevokeChecker(revokeMap{jti: true})

	h := RequireOSMetricsAuth()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/os/metrics", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 for revoked jti", rr.Code)
	}
}

type revokeMap map[string]bool

func (m revokeMap) IsRevoked(_ context.Context, jti string) (bool, error) {
	return m[jti], nil
}
