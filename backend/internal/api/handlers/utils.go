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

	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/service"
	"github.com/rsharma155/sql_optima/internal/validation"
)

func validateInstanceName(name string) error {
	return validation.ValidateInstanceName(name)
}

func instanceInConfig(cfg *config.Config, name string) bool {
	for _, inst := range cfg.Instances {
		if inst.Name == name {
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
	for _, inst := range cfg.Instances {
		if inst.Name == name {
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
