// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Memory analyzer type definitions.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package hot

import "time"

// spillDeltaState tracks previous perf counter values to compute per-second spill rate.
type spillDeltaState struct {
	lastTS   time.Time
	lastSort int64
	lastHash int64
}

