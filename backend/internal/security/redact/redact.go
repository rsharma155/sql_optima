// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Best-effort redaction for logs and error messages to avoid leaking
// credentials, DSNs, tokens, or similar secrets.
//
// SPDX-License-Identifier: MIT
package redact

import "regexp"

var (
	// Roughly match postgres URLs: postgres://user:pass@host:port/db?...
	rePostgresURL = regexp.MustCompile(`(?i)\bpostgres(?:ql)?://[^\s'"]+`)

	// Match common key=value secret patterns (including SQL Server connstrings).
	reKVSecret = regexp.MustCompile(`(?i)\b(password|pwd|secret|token|apikey|api_key|jwt_secret|access_token|refresh_token)\s*[:=]\s*([^\s;,"']+)`)

	// Match SQL Server style "user id=...;password=...;" segments.
	reMssqlPassword = regexp.MustCompile(`(?i)\b(password)\s*=\s*([^;]+)`)
)

// String redacts secrets from an arbitrary string (best-effort; not perfect).
func String(s string) string {
	if s == "" {
		return s
	}
	s = rePostgresURL.ReplaceAllString(s, "postgres://REDACTED")
	s = reKVSecret.ReplaceAllString(s, "$1=REDACTED")
	s = reMssqlPassword.ReplaceAllString(s, "$1=REDACTED")
	return s
}

