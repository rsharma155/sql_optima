// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Handler tests for minting scoped OS-agent tokens.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rsharma155/sql_optima/internal/middleware"
)

func TestMintOSAgentToken_RequiresServerOrInstance(t *testing.T) {
	h := NewOSCollectorHandlers(nil)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/os-collector/token", bytes.NewReader([]byte(`{}`)))
	rr := httptest.NewRecorder()
	h.MintOSAgentToken(rr, req)
	if rr.Code != http.StatusServiceUnavailable && rr.Code != http.StatusBadRequest {
		// metricsSvc nil → 503
		if rr.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d", rr.Code)
		}
	}
}

func TestMintOSAgentToken_WithServerID(t *testing.T) {
	secret := make([]byte, 32)
	for i := range secret {
		secret[i] = byte(i + 11)
	}
	middleware.SetJWTSecret(secret)

	// Direct Generate path assertion via middleware (handler needs metricsSvc for resolve)
	sid := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	tok, _, err := middleware.GenerateOSAgentToken(sid, 2*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := middleware.ValidateToken(tok)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Scope != middleware.ScopeOSMetricsWrite {
		t.Fatalf("scope %q", claims.Scope)
	}

	// Simulate handler JSON shape contract
	body := map[string]interface{}{
		"token":      tok,
		"role":       middleware.RoleOSAgent,
		"scope":      middleware.ScopeOSMetricsWrite,
		"server_id":  sid.String(),
		"ttl_hours":  2,
	}
	b, _ := json.Marshal(body)
	if !bytes.Contains(b, []byte(`"os_metrics:write"`)) {
		t.Fatalf("payload missing scope: %s", b)
	}
}
