// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Route registration for alert engine read and mutation endpoints.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package api

import (
	"github.com/gorilla/mux"
	"github.com/rsharma155/sql_optima/internal/api/handlers"
)

// registerAlertReadRoutes attaches read-only alert engine endpoints.
func registerAlertReadRoutes(sr *mux.Router, ah *handlers.AlertHandlers) {
	sr.HandleFunc("/alerts", ah.ListAlerts).Methods("GET")
	sr.HandleFunc("/alerts/count", ah.CountOpen).Methods("GET")
	sr.HandleFunc("/alerts/{id}", ah.GetAlert).Methods("GET")
	sr.HandleFunc("/alerts/maintenance", ah.ListMaintenanceWindows).Methods("GET")
}

// registerAlertMutationRoutes attaches alert mutation endpoints (acknowledge, resolve, maintenance CRUD).
func registerAlertMutationRoutes(sr *mux.Router, ah *handlers.AlertHandlers) {
	sr.HandleFunc("/alerts/{id}/acknowledge", ah.AcknowledgeAlert).Methods("POST")
	sr.HandleFunc("/alerts/{id}/resolve", ah.ResolveAlert).Methods("POST")
	sr.HandleFunc("/alerts/maintenance", ah.CreateMaintenanceWindow).Methods("POST")
	sr.HandleFunc("/alerts/maintenance/{id}", ah.DeleteMaintenanceWindow).Methods("DELETE")
}
