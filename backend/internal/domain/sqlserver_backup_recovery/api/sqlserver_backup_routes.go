// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Route registrations for SQL Server Backup & Recovery APIs.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package api

import "github.com/gorilla/mux"

func RegisterSQLServerBackupRoutes(sr *mux.Router, h *SQLServerBackupHandler) {
	if h == nil {
		return
	}
	sr.HandleFunc("/sqlserver/backup/dashboard", h.GetDashboard).Methods("GET")
	sr.HandleFunc("/sqlserver/backup/policy", h.GetPolicy).Methods("GET")
	sr.HandleFunc("/sqlserver/backup/policy", h.PutPolicy).Methods("PUT")
}
