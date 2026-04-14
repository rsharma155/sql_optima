// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Provides hot functionality for the backend.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package hot

import "testing"

func TestVacuumProgressRowStructExists(t *testing.T) {
	// compile-time guard
	var _ PostgresVacuumProgressRow
}

