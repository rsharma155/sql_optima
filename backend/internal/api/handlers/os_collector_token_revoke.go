// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Admin endpoint to revoke a previously minted OS-agent JWT by jti or token.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rsharma155/sql_optima/internal/apiresponse"
	"github.com/rsharma155/sql_optima/internal/middleware"
)

// OSAgentTokenRevoker persists revoked OS-agent JWT ids.
type OSAgentTokenRevoker interface {
	Revoke(ctx context.Context, jti string, serverID uuid.UUID, actor, reason string, expiresAt time.Time) error
}

// SetOSAgentRevoker wires the revoke store onto OS collector handlers (optional).
func (h *OSCollectorHandlers) SetOSAgentRevoker(r OSAgentTokenRevoker) {
	if h != nil {
		h.osAgentRevoker = r
	}
}

// RevokeOSAgentToken marks a token jti as revoked until its natural expiry (or +24h).
// Body: { "jti": "...", "reason"?: "..." } or { "token": "<jwt>", "reason"?: "..." }
func (h *OSCollectorHandlers) RevokeOSAgentToken(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.osAgentRevoker == nil {
		apiresponse.WriteJSONError(w, http.StatusServiceUnavailable, "os agent revoke store not configured", nil)
		return
	}

	var body struct {
		JTI    string `json:"jti"`
		Token  string `json:"token"`
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apiresponse.WriteJSONError(w, http.StatusBadRequest, "invalid request body", err)
		return
	}

	jti := strings.TrimSpace(body.JTI)
	serverID := uuid.Nil
	expiresAt := time.Now().UTC().Add(24 * time.Hour)

	if jti == "" && strings.TrimSpace(body.Token) != "" {
		claims, err := middleware.ValidateToken(strings.TrimSpace(body.Token))
		if err != nil {
			// Still allow revoke of expired tokens: parse without exp check via Generate path claims
			claims, err = middleware.ParseTokenClaimsAllowExpired(strings.TrimSpace(body.Token))
			if err != nil {
				apiresponse.WriteJSONError(w, http.StatusBadRequest, "invalid token", err)
				return
			}
		}
		if claims.Role != middleware.RoleOSAgent {
			apiresponse.WriteJSONError(w, http.StatusBadRequest, "token is not an os_agent token", nil)
			return
		}
		jti = claims.ID
		if claims.ExpiresAt != nil {
			expiresAt = claims.ExpiresAt.Time.UTC()
		}
		if claims.ServerID != "" {
			if sid, err := uuid.Parse(claims.ServerID); err == nil {
				serverID = sid
			}
		}
	}
	if jti == "" {
		apiresponse.WriteJSONError(w, http.StatusBadRequest, "jti or token is required", nil)
		return
	}

	actor := ""
	if claims := middleware.GetAuthClaims(r); claims != nil {
		actor = claims.Username
	}

	if err := h.osAgentRevoker.Revoke(r.Context(), jti, serverID, actor, strings.TrimSpace(body.Reason), expiresAt); err != nil {
		apiresponse.WriteJSONError(w, http.StatusInternalServerError, "failed to revoke os agent token", err)
		return
	}

	middleware.AuditAction(slog.Default(), r, "revoke_os_agent_token",
		slog.String("jti", jti),
		slog.String("actor", actor),
		slog.String("server_id", serverID.String()),
	)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"revoked":    true,
		"jti":        jti,
		"expires_at": expiresAt.Format(time.RFC3339),
	})
}
