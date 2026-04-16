// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Test suite for router configuration and route registration.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package api

import (
	"testing"

	"github.com/rsharma155/sql_optima/internal/validation"
)

func TestValidateInstanceName(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{"SQL-local", false},
		{"PG-Cluster-01", false},
		{"ValidServer", false},
		{"Valid_Server_123", false},
		{"Production Postgres", false},
		{"EU (reporting)", false},
		{"../../../etc/passwd", true},
		{"<script>alert(1)</script>", true},
		{"bad;drop", true},
		{"", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validation.ValidateInstanceName(tt.name)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateInstanceName(%q) error = %v, wantErr %v", tt.name, err, tt.wantErr)
			}
		})
	}
}
