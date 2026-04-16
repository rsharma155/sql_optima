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
	// Check DB
	if metricsSvc != nil && metricsSvc.ServerRepo != nil {
		servers, err := metricsSvc.ServerRepo.List(ctx, true)
		if err == nil {
			for _, s := range servers {
				if s.Name == name {
					return true
				}
			}
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
	// Check DB
	if metricsSvc != nil && metricsSvc.ServerRepo != nil {
		servers, err := metricsSvc.ServerRepo.List(ctx, true)
		if err == nil {
			for _, s := range servers {
				if s.Name == name {
					return string(s.DBType) == want
				}
			}
		}
	}
	return false
}
