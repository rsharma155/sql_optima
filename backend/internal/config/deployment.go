package config

import (
	"os"
	"strings"
)

// DeploymentMode returns how the app expects Timescale to be provisioned:
//   - "docker" — compose provides DB_* / TIMESCALEDB_*; skip UI Timescale step; YAML instances kept when registry is empty.
//   - "dedicated" — UI setup wizard persists Timescale; env DB is not used unless TIMESCALE_USE_ENV_ONLY=1.
func DeploymentMode() string {
	v := strings.TrimSpace(os.Getenv("SQL_OPTIMA_DEPLOYMENT"))
	if strings.EqualFold(v, "docker") {
		return "docker"
	}
	return "dedicated"
}

// DeploymentIsDocker reports SQL_OPTIMA_DEPLOYMENT=docker.
func DeploymentIsDocker() bool {
	return DeploymentMode() == "docker"
}
