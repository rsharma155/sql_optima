package api

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/rsharma155/sql_optima/internal/api/handlers"
	"github.com/rsharma155/sql_optima/internal/middleware"
)

// RegisterSetupRoutes registers public first-run endpoints under /api/setup/*.
func RegisterSetupRoutes(r *mux.Router, setupLim *middleware.SetupRateLimiter, deps *handlers.SetupDeps) {
	if deps == nil {
		return
	}
	sh := handlers.NewSetupHandlers(deps)
	sub := r.PathPrefix("/api/setup").Subrouter()
	sub.HandleFunc("/status", sh.Status).Methods("GET")

	postWrap := func(h http.HandlerFunc) http.Handler {
		return middleware.SetupRateLimitMiddleware(setupLim, h)
	}
	sub.Handle("/timescale/test", postWrap(http.HandlerFunc(sh.PostTestTimescale))).Methods("POST")
	sub.Handle("/timescale/migrate-step", postWrap(http.HandlerFunc(sh.PostTimescaleMigrateStep))).Methods("POST")
	sub.Handle("/timescale", postWrap(http.HandlerFunc(sh.PostTimescale))).Methods("POST")
	sub.Handle("/bootstrap-admin", postWrap(http.HandlerFunc(sh.PostBootstrapAdmin))).Methods("POST")
}
