// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Model for collector frequency configuration.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package models

import "time"

type CollectorConfig struct {
	ID               int       `json:"id"`
	CollectorName    string    `json:"collector_name"`
	Module           string    `json:"module"`
	FrequencySeconds int       `json:"frequency_seconds"`
	IsActive         bool      `json:"is_active"`
	UpdatedAt        time.Time `json:"updated_at"`
	UpdatedBy        string    `json:"updated_by"`
}
