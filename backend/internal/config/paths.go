package config

import (
	"log"
	"os"
)

// ResolveDataPaths always uses config.yaml, queries.yml, and frontend/ from the repo root.
// The server must be run from the backend directory.
func ResolveDataPaths() (configPath, queriesPath, frontendDir string) {
	configPath = "../config.yaml"
	queriesPath = "../queries.yml"
	frontendDir = "../frontend"

	if _, err := os.Stat(configPath); err != nil {
		log.Fatalf("[FATAL] config.yaml not found at %s (run from backend directory)", configPath)
	}

	log.Printf("[paths] using config=%s queries=%s frontend=%s", configPath, queriesPath, frontendDir)
	return configPath, queriesPath, frontendDir
}
