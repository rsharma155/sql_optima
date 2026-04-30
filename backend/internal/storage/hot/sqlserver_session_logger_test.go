// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Unit tests for SQL Server Session and Classification storage-layer.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package hot

import (
	"context"
	"testing"
)

func TestTimescaleLogger_SessionAndClassificationMethods(t *testing.T) {
	// This test ensures the TimescaleLogger implements the expected identity and classification methods.
	var _ interface {
		RunSQLServerIdentityUpsertJob(ctx context.Context) error
		RunSQLServerClassificationJob(ctx context.Context) error
	} = (*TimescaleLogger)(nil)
}
