// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Shared utility functions for handler validation and helper methods.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"context"
	"strings"
	"time"

	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/service"
	"github.com/rsharma155/sql_optima/internal/validation"
)

// ParseTimeRange extracts and normalizes from/to timestamps from query parameters.
func ParseTimeRange(fromStr, toStr string) (time.Time, time.Time) {
	now := time.Now().UTC()
	toT := now
	fromT := now.Add(-1 * time.Hour)

	parse := func(s string, fallback time.Time) time.Time {
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

	if fromStr != "" {
		fromT = parse(fromStr, fromT)
	}
	if toStr != "" {
		toT = parse(toStr, toT)
	}

	// Defensive check: if range is inverted or too small, default to last hour
	if fromT.After(toT) || toT.Sub(fromT) < 10*time.Second {
		toT = now
		fromT = toT.Add(-1 * time.Hour)
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

func firstErrString(errs ...error) string {
	for _, e := range errs {
		if e != nil {
			return e.Error()
		}
	}
	return ""
}

// splitCSV takes a comma-separated string and returns a slice of strings.
// It ignores empty strings and trims surrounding whitespace from each item.
func splitCSV(s string) []string {
	if s == "" {
		return nil // Return empty slice if the query parameter is missing or empty
	}

	parts := strings.Split(s, ",")
	var result []string

	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" { // Skip empty entries like "db1,,db2"
			result = append(result, trimmed)
		}
	}

	return result
}
