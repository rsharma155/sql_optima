// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Entry point for PostgreSQL handlers, defining the PostgresHandlers struct and constructor.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/service"
)

type PostgresHandlers struct {
	metricsSvc *service.MetricsService
	cfg        *config.Config
}

func NewPostgresHandlers(metricsSvc *service.MetricsService, cfg *config.Config) *PostgresHandlers {
	return &PostgresHandlers{metricsSvc: metricsSvc, cfg: cfg}
}
