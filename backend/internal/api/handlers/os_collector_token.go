// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Admin/DBA endpoint to mint scoped OS-agent JWTs for host metric ingest.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rsharma155/sql_optima/internal/apiresponse"
	"github.com/rsharma155/sql_optima/internal/middleware"
)

// MintOSAgentToken issues a write-only JWT for POST /api/os/metrics.
// Body: { "server_id": "<uuid>", "ttl_hours"?: number }  (ttl default 2160h = 90d, max 8760h)
// Query alternate: ?instance=<name> resolves server_id.
func (h *OSCollectorHandlers) MintOSAgentToken(w http.ResponseWriter, r *http.Request) {
	if h.metricsSvc == nil {
		apiresponse.WriteJSONError(w, http.StatusServiceUnavailable, "metrics service not configured", nil)
		return
	}

	var body struct {
		ServerID string `json:"server_id"`
		Instance string `json:"instance"`
		TTLHours int    `json:"ttl_hours"`
	}
	if r.Body != nil && r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apiresponse.WriteJSONError(w, http.StatusBadRequest, "invalid request body", err)
			return
		}
	}
	if body.Instance == "" {
		body.Instance = strings.TrimSpace(r.URL.Query().Get("instance"))
	}
	if body.ServerID == "" {
		body.ServerID = strings.TrimSpace(r.URL.Query().Get("server_id"))
	}
	if q := r.URL.Query().Get("ttl_hours"); q != "" && body.TTLHours == 0 {
		if n, err := strconv.Atoi(q); err == nil {
			body.TTLHours = n
		}
	}

	var serverID uuid.UUID
	var err error
	switch {
	case body.ServerID != "":
		serverID, err = uuid.Parse(body.ServerID)
		if err != nil {
			apiresponse.WriteJSONError(w, http.StatusBadRequest, "invalid server_id", err)
			return
		}
	case body.Instance != "":
		serverID, err = h.metricsSvc.ResolveServerIDByInstanceName(r.Context(), body.Instance)
		if err != nil {
			apiresponse.WriteJSONError(w, http.StatusBadRequest, "instance not found", err)
			return
		}
	default:
		apiresponse.WriteJSONError(w, http.StatusBadRequest, "server_id or instance is required", nil)
		return
	}

	ttl := middleware.DefaultOSAgentTokenTTL
	if body.TTLHours > 0 {
		ttl = time.Duration(body.TTLHours) * time.Hour
	}

	tok, jti, err := middleware.GenerateOSAgentToken(serverID, ttl)
	if err != nil {
		apiresponse.WriteJSONError(w, http.StatusInternalServerError, "failed to mint os agent token", err)
		return
	}

	actor := ""
	if claims := middleware.GetAuthClaims(r); claims != nil {
		actor = claims.Username
	}
	middleware.AuditAction(slog.Default(), r, "mint_os_agent_token",
		slog.String("server_id", serverID.String()),
		slog.String("actor", actor),
		slog.String("jti", jti),
		slog.Int("ttl_hours", int(ttl.Hours())),
	)

	expires := time.Now().UTC().Add(ttl)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"token":        tok,
		"token_type":   "Bearer",
		"jti":          jti,
		"role":         middleware.RoleOSAgent,
		"scope":        middleware.ScopeOSMetricsWrite,
		"server_id":    serverID.String(),
		"expires_at":   expires.Format(time.RFC3339),
		"ttl_hours":    int(ttl.Hours()),
		"usage":        "Set SQL_OPTIMA_API_KEY to this token on the DB host. Prefer over admin JWT. Revoke via POST /api/admin/os-collector/token/revoke with jti.",
	})
}
