// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Path resolver for config.yaml and frontend directory locations.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package config

import (
	"log/slog"
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
		slog.Info("[paths] config.yaml not found at %s — instances will come from server registry only", "val", configPath)
	}

	slog.Info("[paths] using config=", "arg1", configPath, "arg2", frontendDir)
	return configPath, frontendDir
}
