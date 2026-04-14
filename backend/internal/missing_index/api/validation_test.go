// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Test suite for API validation.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package api_test

import (
	"testing"

	"github.com/rsharma155/sql_optima/internal/missing_index/api"
)

func TestRequestValidation(t *testing.T) {
	tests := []struct {
		name    string
		req     api.IndexAdvisorRequest
		wantErr error
	}{
		{
			name: "valid request",
			req: api.IndexAdvisorRequest{
				DatabaseDSN:       "postgres://user:pass@host:5432/db",
				QueryText:         "SELECT * FROM orders WHERE id = 1",
				ExecutionPlanJSON: map[string]any{},
			},
			wantErr: nil,
		},
		{
			name: "empty DSN",
			req: api.IndexAdvisorRequest{
				DatabaseDSN:       "",
				QueryText:         "SELECT * FROM orders",
				ExecutionPlanJSON: map[string]any{},
			},
			wantErr: api.ErrEmptyDSN,
		},
		{
			name: "empty query",
			req: api.IndexAdvisorRequest{
				DatabaseDSN:       "postgres://user:pass@host:5432/db",
				QueryText:         "",
				ExecutionPlanJSON: map[string]any{},
			},
			wantErr: api.ErrEmptyQuery,
		},
		{
			name: "placeholder without params",
			req: api.IndexAdvisorRequest{
				DatabaseDSN:       "postgres://user:pass@host:5432/db",
				QueryText:         "SELECT * FROM orders WHERE id = $1",
				ExecutionPlanJSON: map[string]any{},
			},
			wantErr: api.ErrPlaceholderNoParams,
		},
		{
			name: "placeholder with params",
			req: api.IndexAdvisorRequest{
				DatabaseDSN:       "postgres://user:pass@host:5432/db",
				QueryText:         "SELECT * FROM orders WHERE id = $1",
				ExecutionPlanJSON: map[string]any{},
				QueryParams:       []any{1},
			},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if err != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
