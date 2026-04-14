// Package api provides HTTP API endpoints for the monitoring dashboard.
// It handles routing, authentication, and response formatting for all REST endpoints.
// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Defines API handlers and routes for all monitoring endpoints. Registers health routes, authentication, and monitoring handlers for both MSSQL and PostgreSQL databases.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package api

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/rsharma155/sql_optima/internal/api/handlers"
	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/middleware"
	"github.com/rsharma155/sql_optima/internal/service"
)

func RegisterHealthRoutes(r *mux.Router, cfg *config.Config, metricsSvc *service.MetricsService, queriesLoaded bool, loginLimiter *middleware.LoginRateLimiter, sec config.Security) {
	r.HandleFunc("/api/health", HandleHealthLiveness).Methods("GET")
	r.HandleFunc("/api/health/ready", func(w http.ResponseWriter, req *http.Request) {
		HandleHealthReadiness(w, req, cfg, queriesLoaded, metricsSvc)
	}).Methods("GET")

	r.HandleFunc("/api/auth/status", HandleAuthStatus(sec)).Methods("GET")

	if !sec.AuthRequired {
		r.HandleFunc("/api/config", func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			statuses := map[string]string{}
			if metricsSvc != nil {
				statuses = metricsSvc.GetAllInstanceStatuses()
			}
			instances := make([]config.Instance, 0, len(cfg.Instances))
			for _, inst := range cfg.Instances {
				copyInst := inst
				if status, ok := statuses[inst.Name]; ok {
					copyInst.Available = status == "online"
				} else {
					copyInst.Available = true
				}
				instances = append(instances, copyInst)
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"instances": instances,
			})
		}).Methods("GET")
	}

	authH := handlers.NewAuthHandlers(metricsSvc)
	adminH := handlers.NewAdminHandlers(metricsSvc)
	widgetAdminH := handlers.NewWidgetAdminHandlers(metricsSvc)
	mssqlH := handlers.NewMssqlHandlers(metricsSvc, cfg)
	postgresH := handlers.NewPostgresHandlers(metricsSvc, cfg)
	liveH := handlers.NewLiveHandlers(metricsSvc, cfg)
	timescaleH := handlers.NewTimescaleHandlers(metricsSvc, cfg)
	sihH := handlers.NewStorageIndexHealthTimescaleHandlers(metricsSvc)
	healthH := handlers.NewHealthHandlers(metricsSvc, cfg)
	dashboardH := handlers.NewDashboardHandlers(metricsSvc, cfg)
	queryH := handlers.NewQueryHandlers(metricsSvc, cfg)

	mon := &monitoringHandlers{
		Mssql: mssqlH, Postgres: postgresH, Live: liveH, Timescale: timescaleH,
		Health: healthH, Dashboard: dashboardH, Query: queryH, SIH: sihH,
	}

	var rulesH *handlers.RulesHandler
	var rulesHErr error
	rulesBP := func(w http.ResponseWriter, req *http.Request) {
		if rulesH == nil {
			rulesH, rulesHErr = handlers.NewRulesHandlerFromConfig(cfg)
			if rulesHErr != nil {
				log.Printf("[Router] RulesHandler initialization error: %v", rulesHErr)
			}
		}
		if rulesH != nil {
			rulesH.BestPractices(w, req)
		} else {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "rule engine not available"})
		}
	}

	// --- Public / open /api branch (no JWT)
	openAPI := r.PathPrefix("/api").Subrouter()
	if loginLimiter != nil {
		loginHF := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			authH.Login(w, req)
		})
		limitedLogin := middleware.LoginRateLimitMiddleware(loginLimiter, loginHF)
		openAPI.Handle("/login", limitedLogin).Methods("POST")
		openAPI.Handle("/auth/login", limitedLogin).Methods("POST")
	}

	if sec.AuthRequired {
		// Strict: only auth endpoints stay public; config is moved behind JWT below.
	} else {
		registerMonitoringReadRoutes(openAPI, mon, rulesBP)
		registerMonitoringElevatedRoutes(openAPI, mssqlH, handlers.PgExplainAnalyze, handlers.PgExplainOptimize, handlers.PgExplainIndexAdvisor(cfg))
		// Legacy: explain was public when auth is not required.
	}

	// --- Any authenticated user (JWT or OIDC)
	authed := r.PathPrefix("/api").Subrouter()
	authed.Use(middleware.RequireAuth(""))
	authed.HandleFunc("/auth/me", authH.Me).Methods("GET")

	if sec.AuthRequired {
		configProtected := r.PathPrefix("/api").Subrouter()
		configProtected.Use(middleware.RequireAuth(""))
		configProtected.Use(middleware.RequireAnyRole("viewer", "dba", "admin"))
		configProtected.HandleFunc("/config", func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			statuses := map[string]string{}
			if metricsSvc != nil {
				statuses = metricsSvc.GetAllInstanceStatuses()
			}
			instances := make([]config.Instance, 0, len(cfg.Instances))
			for _, inst := range cfg.Instances {
				copyInst := inst
				if status, ok := statuses[inst.Name]; ok {
					copyInst.Available = status == "online"
				} else {
					copyInst.Available = true
				}
				instances = append(instances, copyInst)
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"instances": instances,
			})
		}).Methods("GET")

		readAPI := r.PathPrefix("/api").Subrouter()
		readAPI.Use(middleware.RequireAuth(""))
		readAPI.Use(middleware.RequireAnyRole("viewer", "dba", "admin"))
		registerMonitoringReadRoutes(readAPI, mon, rulesBP)

		dbaAPI := r.PathPrefix("/api").Subrouter()
		dbaAPI.Use(middleware.RequireAuth(""))
		dbaAPI.Use(middleware.RequireAnyRole("dba", "admin"))
		registerMonitoringElevatedRoutes(dbaAPI, mssqlH, handlers.PgExplainAnalyze, handlers.PgExplainOptimize, handlers.PgExplainIndexAdvisor(cfg))
		registerPostgresDBAMutations(dbaAPI, postgresH)
		registerDashboardWidgetRoutes(dbaAPI, dashboardH)
	}

	if !sec.AuthRequired {
		legacyAuthed := r.PathPrefix("/api").Subrouter()
		legacyAuthed.Use(middleware.RequireAuth(""))
		legacyAuthed.HandleFunc("/mssql/xevents", mssqlH.XEvents).Methods("GET")
		registerPostgresDBAMutations(legacyAuthed, postgresH)
		registerDashboardWidgetRoutes(legacyAuthed, dashboardH)
	}

	// --- Admin-only
	adminAPI := r.PathPrefix("/api/admin").Subrouter()
	adminAPI.Use(middleware.RequireAuth("admin"))
	adminAPI.HandleFunc("/users", adminH.CreateUser).Methods("POST")
	adminAPI.HandleFunc("/users", adminH.ListUsers).Methods("GET")
	adminAPI.HandleFunc("/users/{id}", adminH.DeleteUser).Methods("DELETE")
	adminAPI.HandleFunc("/users/{id}/role", adminH.UpdateUserRole).Methods("PUT")
	adminAPI.HandleFunc("/widgets/{id}", widgetAdminH.UpdateWidget).Methods("PUT")
	adminAPI.HandleFunc("/widgets/{id}/restore", widgetAdminH.RestoreWidget).Methods("POST")
	adminAPI.HandleFunc("/widgets/{id}", widgetAdminH.GetWidget).Methods("GET")
	adminAPI.HandleFunc("/widgets", widgetAdminH.ListWidgets).Methods("GET")
}
