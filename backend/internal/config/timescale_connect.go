package config

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/rsharma155/sql_optima/internal/storage/hot"
)

// TimescalePersistFileExists reports whether the encrypted Timescale connection file is present.
func TimescalePersistFileExists(configPath string) bool {
	st, err := os.Stat(TimescalePersistPath(configPath))
	if err != nil {
		return false
	}
	return !st.IsDir() && st.Size() > 0
}

// ConnectMetricsTimescale connects to the TimescaleDB metrics repository.
//
// First-run / UI-first behavior: if there is no valid persisted connection from the setup wizard,
// the process does not read TIMESCALEDB_* / DB_* env vars unless TIMESCALE_USE_ENV_ONLY=1 (legacy Docker/dev).
//
// Returns usingEnvTimescale=true when the live connection came from environment variables.
func ConnectMetricsTimescale(configPath string, jwtSecret []byte) (ts *hot.HotStorage, usingEnvTimescale bool, err error) {
	dockerMode := DeploymentIsDocker()
	envOnly := dockerMode || strings.TrimSpace(os.Getenv("TIMESCALE_USE_ENV_ONLY")) == "1"
	if dockerMode {
		log.Println("[deployment] SQL_OPTIMA_DEPLOYMENT=docker: using compose DB_* / TIMESCALEDB_* for metrics TimescaleDB")
	}

	persistedTS, perr := LoadPersistedTimescaleConfig(configPath, jwtSecret)
	if perr != nil {
		log.Printf("[timescale] persisted config read: %v", perr)
	}
	if persistedTS != nil {
		var e error
		ts, e = hot.New(persistedTS)
		if e != nil {
			log.Printf("[WARNING] persisted Timescale connection failed: %v", e)
			ts = nil
		} else {
			log.Println("[Info] Connected to TimescaleDB using UI-persisted credentials")
		}
	}

	if ts == nil && envOnly {
		ts, err = hot.New(nil)
		if err != nil {
			return nil, false, fmt.Errorf("timescale env connection: %w", err)
		}
		if dockerMode {
			log.Println("[Info] Connected to TimescaleDB using Docker/compose environment (DB_* / TIMESCALEDB_*)")
		} else {
			log.Println("[Info] Connected to TimescaleDB using TIMESCALE_USE_ENV_ONLY (DB_* / TIMESCALEDB_* environment)")
		}
		return ts, true, nil
	}

	if ts == nil {
		if !TimescalePersistFileExists(configPath) {
			log.Println("[Info] First run: no encrypted Timescale config at data/; open /setup to add the metrics repository (or set TIMESCALE_USE_ENV_ONLY=1 to use env vars)")
		} else {
			log.Println("[Info] TimescaleDB not connected: persisted file present but invalid or database unreachable (set TIMESCALE_USE_ENV_ONLY=1 for env fallback)")
		}
	}

	return ts, false, nil
}
