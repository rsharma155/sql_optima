// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Shared helper utilities for the alert service layer.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package service

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/rsharma155/sql_optima/internal/domain/alerts"
)

func parseUUID(s string) (uuid.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, fmt.Errorf("%w: %s", alerts.ErrInvalidAlertID, s)
	}
	return id, nil
}

// isNoDataError returns true for errors that mean "no data available yet"
// rather than a genuine database failure — specifically pgx.ErrNoRows and
// PostgreSQL 42P01 (undefined_table).
func isNoDataError(err error) bool {
	if errors.Is(err, pgx.ErrNoRows) {
		return true
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "42P01" {
		return true
	}
	return false
}
