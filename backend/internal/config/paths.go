// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Path resolver for config.yaml and frontend directory locations.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package config

import (
	"log"
	"os"
)

// ResolveDataPaths returns config.yaml and frontend/ from the repo root.
// The server must be run from the backend directory.
// If config.yaml is absent (e.g. Docker builds), a warning is logged
// and the path is still returned — LoadConfig will produce an empty config.
func ResolveDataPaths() (configPath, frontendDir string) {
	configPath = "../config.yaml"
	frontendDir = "../frontend"

	if _, err := os.Stat(configPath); err != nil {
		log.Printf("[paths] config.yaml not found at %s — instances will come from server registry only", configPath)
	}

	log.Printf("[paths] using config=%s frontend=%s", configPath, frontendDir)
	return configPath, frontendDir
}
