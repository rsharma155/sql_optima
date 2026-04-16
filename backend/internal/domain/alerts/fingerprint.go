// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Deterministic SHA-256 fingerprint generation for alert deduplication.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package alerts

import (
	"crypto/sha256"
	"fmt"
	"strings"
)

// Fingerprint generates a deterministic de-duplication key from the
// combination of instance, engine, category, and a caller-supplied rule name.
// Identical fingerprints within the same open window are treated as repeat
// occurrences rather than new alerts.
func Fingerprint(instanceName string, engine Engine, category, ruleName string) string {
	parts := []string{
		strings.ToLower(strings.TrimSpace(instanceName)),
		string(engine),
		strings.ToLower(strings.TrimSpace(category)),
		strings.ToLower(strings.TrimSpace(ruleName)),
	}
	h := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return fmt.Sprintf("%x", h[:16]) // 32 hex chars
}
