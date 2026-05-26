// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Shared utility functions for handler validation and helper methods.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"log/slog"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/service"
	"github.com/rsharma155/sql_optima/internal/validation"
)

const maxTimeRange = 7 * 24 * time.Hour

// ParseTimeRange extracts and normalizes from/to timestamps from query parameters.
func ParseTimeRange(fromStr, toStr string) (time.Time, time.Time) {
	now := time.Now().UTC()
	toT := now
	fromT := now.Add(-1 * time.Hour)

	parse := func(s string, fallback time.Time) time.Time {
		s = strings.TrimSpace(s)
		if s == "" || s == "undefined" || s == "null" {
			return fallback
		}
		// 1. Try RFC3339 (from JS toISOString)
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			return t.UTC()
		}
		// 2. Try RFC3339Nano
		if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
			return t.UTC()
		}
		// 3. Try picker format (assume local time if no Z/offset)
		if t, err := time.Parse("2006-01-02T15:04", s); err == nil {
			return t.UTC()
		}
		return fallback
	}

	fromProvided := strings.TrimSpace(fromStr) != "" && fromStr != "undefined" && fromStr != "null"
	toProvided := strings.TrimSpace(toStr) != "" && toStr != "undefined" && toStr != "null"

	if fromProvided {
		fromT = parse(fromStr, fromT)
	}
	if toProvided {
		toT = parse(toStr, toT)
	}

	// Defensive check: if range is inverted or too small, default to last hour
	if fromT.After(toT) || toT.Sub(fromT) < 10*time.Second {
		toT = now
		fromT = toT.Add(-1 * time.Hour)
	}

	// Cap to 7 days only when both endpoints were explicitly supplied
	if fromProvided && toProvided && toT.Sub(fromT) > maxTimeRange {
		fromT = toT.Add(-maxTimeRange)
	}

	// If the range is purely in the future (skew), cap to now
	if fromT.After(now) {
		fromT = now.Add(-1 * time.Hour)
		toT = now
	} else if toT.After(now) {
		toT = now
	}

	return fromT, toT
}

func ParseServerID(r *http.Request, cfg *config.Config) (uuid.UUID, bool) {
	// 1. Try server_id or instance or server query param (UUID)
	for _, p := range []string{"server_id", "instance", "server"} {
		val := r.URL.Query().Get(p)
		if val != "" {
			if id, err := uuid.Parse(val); err == nil {
				return id, true
			}
			// If not UUID, try name lookup (case-insensitive)
			if cfg != nil {
				upperVal := strings.ToUpper(val)
				for _, inst := range cfg.Instances {
					if strings.ToUpper(inst.Name) == upperVal {
						return inst.ServerID, true
					}
				}
				slog.Info("[API] ParseServerID: instance name", "arg1", val, "arg2", len(cfg.Instances))
			} else {
				// cfg is nil — can't look up name; proceed with Nil UUID so the
				// service layer can handle the call (e.g. return 500 for nil pool).
				return uuid.Nil, true
			}
		}
	}

	// 2. Try mux vars
	vars := mux.Vars(r)
	for _, p := range []string{"id", "server_id", "instance"} {
		val := vars[p]
		if val != "" {
			if id, err := uuid.Parse(val); err == nil {
				return id, true
			}
			if cfg != nil {
				upperVal := strings.ToUpper(val)
				for _, inst := range cfg.Instances {
					if strings.ToUpper(inst.Name) == upperVal {
						return inst.ServerID, true
					}
				}
			} else {
				return uuid.Nil, true
			}
		}
	}

	return uuid.Nil, false
}

// instanceLookup is the result of resolveInstanceParam.
type instanceLookup int

const (
	lookupMissing  instanceLookup = iota // no param provided
	lookupNotFound instanceLookup = iota // param given but not found in config
	lookupFound    instanceLookup = iota // resolved OK
)

// resolveInstanceParam resolves an instance query param to a UUID and name.
// It distinguishes between a missing param (→ 400) and a provided but unknown
// name (→ 404), which ParseServerID cannot.
func resolveInstanceParam(r *http.Request, cfg *config.Config) (id uuid.UUID, name string, result instanceLookup) {
	return resolveInstanceParamWithRepo(r.Context(), r, cfg, nil)
}

// resolveInstanceParamWithRepo resolves instance/server_id query params to optima_servers.id.
// Config is checked first; when metricsSvc.ServerRepo is set, registry rows are used as fallback.
func resolveInstanceParamWithRepo(ctx context.Context, r *http.Request, cfg *config.Config, metricsSvc *service.MetricsService) (id uuid.UUID, name string, result instanceLookup) {
	for _, p := range []string{"instance", "server_id", "server"} {
		val := strings.TrimSpace(r.URL.Query().Get(p))
		if val == "" {
			continue
		}
		// Direct UUID — always accepted
		if parsed, err := uuid.Parse(val); err == nil {
			return parsed, val, lookupFound
		}
		// Validate instance name before config lookup — rejects XSS and SQL injection patterns
		if err := validateInstanceName(val); err != nil {
			return uuid.Nil, val, lookupMissing // treat invalid names as "missing" → 400
		}
		// Name lookup in loaded config
		if cfg != nil {
			up := strings.ToUpper(val)
			for _, inst := range cfg.Instances {
				if strings.ToUpper(inst.Name) == up {
					return inst.ServerID, inst.Name, lookupFound
				}
			}
		}
		// Registry fallback (matches TimescaleDB server_id on metrics rows)
		if metricsSvc != nil && metricsSvc.ServerRepo != nil {
			if s, err := metricsSvc.ServerRepo.GetByName(ctx, val); err == nil && s.ID != uuid.Nil {
				return s.ID, s.Name, lookupFound
			}
		}
		if cfg != nil {
			return uuid.Nil, val, lookupNotFound
		}
		return uuid.Nil, val, lookupNotFound
	}
	return uuid.Nil, "", lookupMissing
}

func validateInstanceName(name string) error {
	return validation.ValidateInstanceName(name)
}

func instanceInConfig(cfg *config.Config, name string) bool {
	upperName := strings.ToUpper(name)
	for _, inst := range cfg.Instances {
		if strings.ToUpper(inst.Name) == upperName {
			return true
		}
	}
	return false
}

func instanceExists(ctx context.Context, cfg *config.Config, metricsSvc *service.MetricsService, name string) bool {
	if instanceInConfig(cfg, name) {
		return true
	}
	if metricsSvc != nil && metricsSvc.ServerRepo != nil {
		if _, err := metricsSvc.ServerRepo.GetByName(ctx, name); err == nil {
			return true
		}
	}
	return false
}

func instanceType(cfg *config.Config, name string, want string) bool {
	upperName := strings.ToUpper(name)
	for _, inst := range cfg.Instances {
		if strings.ToUpper(inst.Name) == upperName {
			return inst.Type == want
		}
	}
	return false
}

func instanceTypeFromDB(ctx context.Context, cfg *config.Config, metricsSvc *service.MetricsService, name string, want string) bool {
	if instanceType(cfg, name, want) {
		return true
	}
	if metricsSvc != nil && metricsSvc.ServerRepo != nil {
		if s, err := metricsSvc.ServerRepo.GetByName(ctx, name); err == nil {
			return string(s.DBType) == want
		}
	}
	return false
}

// SendJSON sends a successful JSON response with a standardized envelope.
func SendJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    data,
	})
}

// SendError sends a JSON error response with a standardized envelope.
func SendError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": false,
		"error":   message,
	})
}

