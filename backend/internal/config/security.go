// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Security configuration loader for authentication modes (local/OIDC), JWT settings, and production password handling.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package config

import (
	"os"
	"strings"
)

// Security holds auth and hardening flags (env-first; merged from AppConfig when using Viper).
type Security struct {
	AuthRequired bool
	AuthMode     string // local | oidc
	// OIDC
	OIDCIssuerURL string
	OIDCAudience  string
	// Production: reject YAML passwords when true
	DisallowYAMLPasswords bool
}

func envTruthy(key string) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	return v == "1" || v == "true" || v == "yes"
}

// LoadSecurity reads security-related environment variables.
func LoadSecurity() Security {
	s := Security{
		AuthRequired:          envTruthy("AUTH_REQUIRED"),
		AuthMode:              strings.TrimSpace(os.Getenv("AUTH_MODE")),
		OIDCIssuerURL:         strings.TrimSpace(os.Getenv("OIDC_ISSUER_URL")),
		OIDCAudience:          strings.TrimSpace(os.Getenv("OIDC_AUDIENCE")),
		DisallowYAMLPasswords: envTruthy("DISALLOW_YAML_PASSWORDS") || envTruthy("AUTH_REQUIRED"),
	}
	if s.AuthMode == "" {
		s.AuthMode = "local"
	}
	return s
}
