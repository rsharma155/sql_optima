package api

import (
	"encoding/json"
	"net/http"

	"github.com/rsharma155/sql_optima/internal/config"
)

// HandleAuthStatus exposes whether the API expects authentication (for SPA boot).
func HandleAuthStatus(sec config.Security) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"auth_required": sec.AuthRequired,
			"auth_mode":     sec.AuthMode,
		})
	}
}
