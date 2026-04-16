// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Shared helper utilities for the alert service layer.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package service

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/rsharma155/sql_optima/internal/domain/alerts"
)

func parseUUID(s string) (uuid.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, fmt.Errorf("%w: %s", alerts.ErrInvalidAlertID, s)
	}
	return id, nil
}
