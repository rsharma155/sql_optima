// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Shared error helpers for Storage/Index Health Timescale reads.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package hot

import (
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
)

func isMissingRelation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "42P01"
	}
	return false
}

func schemaMissingErr(rel string) error {
	return fmt.Errorf("timescale schema missing (%s). Apply `infrastructure/sql_scripts/00_timescale_schema.sql` to your TimescaleDB", rel)
}

