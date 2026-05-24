// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Initial application setup and configuration API handlers.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/middleware"
	"github.com/rsharma155/sql_optima/internal/repository"
	"github.com/rsharma155/sql_optima/internal/service"
	"github.com/rsharma155/sql_optima/internal/setup"
	"github.com/rsharma155/sql_optima/internal/storage/hot"
)

type SetupHandlers struct {
	metricsSvc         *service.MetricsService
	disablePublicSetup bool
	reloadFromRegistry func()
	configPath         string
	jwtSecret          []byte
}

type SetupDeps struct {
	Metrics              *service.MetricsService
	Cfg                  *config.Config
	ConfigPath           string
	JWTSecret            []byte
	ReloadFromRegistry   func()
	VaultAddrSet         bool
	UsingLocalKMS        bool
	DisablePublicSetup   bool
	AllowTimescaleReconf bool
}

func NewSetupHandlers(deps *SetupDeps) *SetupHandlers {
	if deps == nil {
		return &SetupHandlers{}
	}
	return &SetupHandlers{
		metricsSvc:         deps.Metrics,
		disablePublicSetup: deps.DisablePublicSetup,
		reloadFromRegistry: deps.ReloadFromRegistry,
		configPath:         deps.ConfigPath,
		jwtSecret:          deps.JWTSecret,
	}
}

// Status returns the current setup state so the frontend knows whether to show
// the setup wizard or proceed to the main app.
func (h *SetupHandlers) Status(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	deployment := strings.TrimSpace(os.Getenv("SQL_OPTIMA_DEPLOYMENT"))
	dockerMode := deployment == "docker"

	tsConnected := h.metricsSvc != nil && h.metricsSvc.IsTimescaleConnected()
	needsTimescale := !tsConnected

	needsAdmin := false
	needsOnboarding := false
	if tsConnected {
		pool := h.metricsSvc.GetTimescaleDBPool()
		var adminCount int
		if err := pool.QueryRow(r.Context(),
			`SELECT COUNT(*) FROM optima_users WHERE role = 'admin'`,
		).Scan(&adminCount); err != nil || adminCount == 0 {
			needsAdmin = true
		}

		if !needsAdmin {
			var serverCount int
			if err := pool.QueryRow(r.Context(),
				`SELECT COUNT(*) FROM optima_servers WHERE is_active = true`,
			).Scan(&serverCount); err != nil || serverCount == 0 {
				needsOnboarding = true
			}
		}
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"timescale_connected":       tsConnected,
		"needs_timescale":           needsTimescale,
		"needs_dedicated_timescale": needsTimescale && !dockerMode,
		"needs_bootstrap_admin":     needsAdmin,
		"needs_onboarding_servers":  needsOnboarding,
		"docker_mode":               dockerMode,
		"deployment":                deployment,
		"public_setup_disabled":     h.disablePublicSetup,
	})
}

// PostTestTimescale opens a one-shot connection to the supplied credentials and
// returns {"success":true} or {"success":false,"error":"..."}.
func (h *SetupHandlers) PostTestTimescale(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var body struct {
		Host            string `json:"host"`
		Port            int    `json:"port"`
		Database        string `json:"database"`
		Username        string `json:"username"`
		Password        string `json:"password"`
		SSLMode         string `json:"ssl_mode"`
		UseVault        bool   `json:"use_vault"`
		VaultSecretPath string `json:"vault_secret_path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Invalid request body"})
		return
	}

	if body.Port == 0 {
		body.Port = 5432
	}
	if body.SSLMode == "" {
		body.SSLMode = "disable"
	}

	dsn := fmt.Sprintf("host=%s port=%d dbname=%s user=%s password=%s sslmode=%s connect_timeout=10",
		body.Host, body.Port, body.Database, body.Username, body.Password, body.SSLMode)

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
		return
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}

// PostBootstrapAdmin creates the first admin account and returns a JWT so the
// user is immediately logged in.
func (h *SetupHandlers) PostBootstrapAdmin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var body struct {
		Username       string `json:"username"`
		Password       string `json:"password"`
		PasswordVerify string `json:"password_verify"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "Invalid request body"})
		return
	}

	if body.Username == "" || len(body.Password) < 8 {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "Username required and password must be ≥ 8 characters"})
		return
	}
	if body.Password != body.PasswordVerify {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "Passwords do not match"})
		return
	}

	if h.metricsSvc == nil || !h.metricsSvc.IsTimescaleConnected() {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "Metrics database not connected"})
		return
	}

	pool := h.metricsSvc.GetTimescaleDBPool()
	userRepo := repository.NewUserRepository(pool)

	// Guard: only allow bootstrap when no admin exists yet.
	var adminCount int
	if err := pool.QueryRow(r.Context(), `SELECT COUNT(*) FROM optima_users WHERE role = 'admin'`).Scan(&adminCount); err == nil && adminCount > 0 {
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "An admin account already exists"})
		return
	}

	user, err := userRepo.CreateUser(r.Context(), body.Username, body.Password, "admin")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		return
	}

	token, err := middleware.GenerateToken(user.UserID, user.Username, user.Role)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "Admin created but token generation failed"})
		return
	}

	// Set HttpOnly auth cookie so subsequent API calls are authenticated.
	http.SetCookie(w, &http.Cookie{
		Name:     middleware.AuthCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400,
	})

	// Set JS-readable CSRF cookie so subsequent mutating requests pass CSRF checks.
	if csrfToken, err := middleware.GenerateCSRFToken(); err == nil {
		middleware.SetCSRFCookie(w, r, csrfToken)
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"token":    token,
		"user_id":  user.UserID,
		"username": user.Username,
		"role":     user.Role,
	})
}

// PostTimescale persists the TimescaleDB credentials and rebinds the live connection pool.
// Called by the setup wizard after all migration steps succeed.
func (h *SetupHandlers) PostTimescale(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var body struct {
		Host            string `json:"host"`
		Port            int    `json:"port"`
		Database        string `json:"database"`
		Username        string `json:"username"`
		Password        string `json:"password"`
		SSLMode         string `json:"ssl_mode"`
		UseVault        bool   `json:"use_vault"`
		VaultSecretPath string `json:"vault_secret_path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "Invalid request body"})
		return
	}
	if body.Port == 0 {
		body.Port = 5432
	}
	if body.SSLMode == "" {
		body.SSLMode = "disable"
	}

	cfg := &hot.Config{
		Host:     body.Host,
		Port:     strconv.Itoa(body.Port),
		User:     body.Username,
		Password: body.Password,
		Database: body.Database,
		SSLMode:  body.SSLMode,
		MaxConns: 50,
	}

	if h.configPath != "" && len(h.jwtSecret) > 0 {
		if err := hot.SavePersistedTimescaleConfig(h.configPath, h.jwtSecret, cfg); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "Failed to persist configuration: " + err.Error()})
			return
		}
	}

	newStorage, err := hot.New(cfg)
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "TimescaleDB connection failed: " + err.Error()})
		return
	}

	if h.metricsSvc != nil {
		if err := h.metricsSvc.RebindTimescale(newStorage); err != nil {
			newStorage.Close()
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "Failed to activate connection: " + err.Error()})
			return
		}
		// Ensure credential encryption is ready even when TimescaleDB was not
		// available at startup (first-run wizard flow).
		h.metricsSvc.InitKMSIfNeeded(h.jwtSecret)
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}

// PostTimescaleMigrateStep runs a single numbered SQL migration script against
// the TimescaleDB instance supplied in the request body.
func (h *SetupHandlers) PostTimescaleMigrateStep(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var body struct {
		Host            string `json:"host"`
		Port            int    `json:"port"`
		Database        string `json:"database"`
		Username        string `json:"username"`
		Password        string `json:"password"`
		SSLMode         string `json:"ssl_mode"`
		Step            int    `json:"step"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Invalid request body"})
		return
	}

	if body.Port == 0 {
		body.Port = 5432
	}
	if body.SSLMode == "" {
		body.SSLMode = "disable"
	}

	dir, err := setup.ResolveMigrationsDir()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Cannot locate SQL script directory: " + err.Error() + ". Set SQL_OPTIMA_SQL_SCRIPTS_DIR env var to the folder containing 01_timescale_schema.sql.",
		})
		return
	}

	params := setup.TSConnParams{
		Host:     body.Host,
		Port:     body.Port,
		Database: body.Database,
		Username: body.Username,
		Password: body.Password,
		SSLMode:  body.SSLMode,
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()

	fileName, summary, err := setup.RunMigrationStep(ctx, dir, body.Step, params)
	if err != nil {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("[%s] %s", fileName, err.Error()),
		})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"file":     fileName,
		"summary":  setup.TruncateOutput(summary, 2000),
	})
}

func (h *SetupHandlers) CompleteSetup(w http.ResponseWriter, r *http.Request) {
	if err := h.metricsSvc.RebindTimescale(nil); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if err := h.metricsSvc.EnsureServerKMS(); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
}
