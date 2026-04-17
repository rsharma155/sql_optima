// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Instance validation for configuration and connection status checking.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package validation

import "regexp"

// Letters and numbers in any script, plus a small set of punctuation common in display names.
// Rejects path/query metacharacters, quotes, semicolons, and angle brackets.
var instanceNameRe = regexp.MustCompile(`^[\p{L}\p{N} _.\-()@,]+$`)

// ValidateInstanceName validates an instance identifier that is used in URLs and DB lookups.
// Display names may include spaces and punctuation; characters that break URLs or SQL literals are rejected.
func ValidateInstanceName(name string) error {
	if name == "" {
		return &Error{Message: "missing instance name"}
	}
	if !instanceNameRe.MatchString(name) {
		return &Error{Message: "invalid instance name: contains invalid characters"}
	}
	if len(name) > 255 {
		return &Error{Message: "instance name too long"}
	}
	return nil
}

type Error struct {
	Message string
}

func (e *Error) Error() string { return e.Message }

