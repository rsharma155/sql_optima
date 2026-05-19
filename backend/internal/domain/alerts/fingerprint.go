// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Generate a deterministic fingerprint for an alert to allow
//
//	de-duplication and grouping.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package alerts

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"github.com/google/uuid"
)

// Fingerprint creates a stable identifier for an alert based on its core identity.
// Changes in severity or description don't change the fingerprint, allowing us
// to track the same "incident" over time.
func Fingerprint(serverID uuid.UUID, engine Engine, category, ruleName string) string {
	c := strings.ToLower(strings.TrimSpace(category))
	r := strings.ToLower(strings.TrimSpace(ruleName))
	raw := fmt.Sprintf("%s|%s|%s|%s", serverID.String(), engine, c, r)
	hash := sha256.Sum256([]byte(raw))
	return fmt.Sprintf("%x", hash)
}
