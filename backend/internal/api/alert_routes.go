// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Route registration for alert engine read and mutation endpoints.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package api

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/rsharma155/sql_optima/internal/api/handlers"
)

// registerAlertReadRoutes attaches read-only alert engine endpoints.
func registerAlertReadRoutes(sr *mux.Router, ah *handlers.AlertHandlers) {
	sr.HandleFunc("/alerts", func(w http.ResponseWriter, r *http.Request) {
		if ah == nil {
			http.Error(w, `{"success":false,"error":"alert engine not configured (TimescaleDB disconnected)"}`, http.StatusServiceUnavailable)
			return
		}
		ah.ListAlerts(w, r)
	}).Methods("GET")

	sr.HandleFunc("/alerts/count", func(w http.ResponseWriter, r *http.Request) {
		if ah == nil {
			http.Error(w, `{"success":false,"error":"alert engine not configured"}`, http.StatusServiceUnavailable)
			return
		}
		ah.CountOpen(w, r)
	}).Methods("GET")

	sr.HandleFunc("/alerts/{id}", func(w http.ResponseWriter, r *http.Request) {
		if ah == nil {
			http.Error(w, `{"success":false,"error":"alert engine not configured"}`, http.StatusServiceUnavailable)
			return
		}
		ah.GetAlert(w, r)
	}).Methods("GET")

	sr.HandleFunc("/alerts/maintenance", func(w http.ResponseWriter, r *http.Request) {
		if ah == nil {
			http.Error(w, `{"success":false,"error":"alert engine not configured"}`, http.StatusServiceUnavailable)
			return
		}
		ah.ListMaintenanceWindows(w, r)
	}).Methods("GET")
}

// registerAlertMutationRoutes attaches alert mutation endpoints (acknowledge, resolve, maintenance CRUD).
func registerAlertMutationRoutes(sr *mux.Router, ah *handlers.AlertHandlers) {
	sr.HandleFunc("/alerts/{id}/acknowledge", func(w http.ResponseWriter, r *http.Request) {
		if ah == nil {
			http.Error(w, `{"success":false,"error":"alert engine not configured"}`, http.StatusServiceUnavailable)
			return
		}
		ah.AcknowledgeAlert(w, r)
	}).Methods("POST")

	sr.HandleFunc("/alerts/{id}/resolve", func(w http.ResponseWriter, r *http.Request) {
		if ah == nil {
			http.Error(w, `{"success":false,"error":"alert engine not configured"}`, http.StatusServiceUnavailable)
			return
		}
		ah.ResolveAlert(w, r)
	}).Methods("POST")

	sr.HandleFunc("/alerts/maintenance", func(w http.ResponseWriter, r *http.Request) {
		if ah == nil {
			http.Error(w, `{"success":false,"error":"alert engine not configured"}`, http.StatusServiceUnavailable)
			return
		}
		ah.CreateMaintenanceWindow(w, r)
	}).Methods("POST")

	sr.HandleFunc("/alerts/maintenance/{id}", func(w http.ResponseWriter, r *http.Request) {
		if ah == nil {
			http.Error(w, `{"success":false,"error":"alert engine not configured"}`, http.StatusServiceUnavailable)
			return
		}
		ah.DeleteMaintenanceWindow(w, r)
	}).Methods("DELETE")
}
