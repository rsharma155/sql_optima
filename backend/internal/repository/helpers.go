// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Internal repository helper functions.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"hash/crc64"
	"time"

	"github.com/google/uuid"
)

var crcTable = crc64.MakeTable(crc64.ECMA)

// ComputeCRC64 generates a fast hash for state-change tracking.
func ComputeCRC64(s string) uint64 {
	return crc64.Checksum([]byte(s), crcTable)
}

// CollectorRunEntry tracks meta-monitoring data for a pulse run.
type CollectorRunEntry struct {
	Name         string
	ServerID     uuid.UUID
	StartTime    time.Time
	EndTime      time.Time
	Status       string // 'success', 'failed'
	RowsInserted int
	Error        string
	DurationMs   int
}
