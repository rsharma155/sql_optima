// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Domain-level sentinel errors for the alert engine.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package alerts

import "errors"

var (
	ErrAlertNotFound        = errors.New("alert not found")
	ErrAlertAlreadyResolved = errors.New("alert is already resolved")
	ErrInvalidAlertID       = errors.New("invalid alert id")
	ErrInvalidSeverity      = errors.New("invalid severity value")
	ErrInvalidStatus        = errors.New("invalid status value")
	ErrInvalidEngine        = errors.New("invalid engine value")
	ErrMissingFingerprint   = errors.New("fingerprint is required")
	ErrMissingInstanceName  = errors.New("instance name is required")
	ErrMissingCategory      = errors.New("category is required")
	ErrMissingTitle         = errors.New("title is required")
)
