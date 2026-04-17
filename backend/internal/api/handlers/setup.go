package handlers

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/middleware"
	"github.com/rsharma155/sql_optima/internal/service"
	setupsql "github.com/rsharma155/sql_optima/internal/setup"
	"github.com/rsharma155/sql_optima/internal/storage/hot"
)

// SetupDeps wires first-run bootstrap (Timescale repository + first admin).
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

type SetupHandlers struct {
	deps *SetupDeps
}

func NewSetupHandlers(deps *SetupDeps) *SetupHandlers {
	return &SetupHandlers{deps: deps}
}

func (h *SetupHandlers) Status(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if h == nil || h.deps == nil || h.deps.Metrics == nil {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"deployment":               config.DeploymentMode(),
			"timescale_connected":      false,
			"needs_timescale":          true,
			"needs_bootstrap_admin":    false,
			"needs_onboarding_servers": false,
			"user_count":               0,
			"monitoring_server_count":  0,
			"monitoring_active_count":  0,
			"kms_ready":                false,
			"vault_configured":         false,
			"local_kms":                false,
			"public_setup_disabled":    false,
		})
		return
	}

	ms := h.deps.Metrics
	deployment := config.DeploymentMode()
	docker := deployment == "docker"
	tsOK := ms.IsTimescaleConnected()
	userCount := 0
	if ms.UserRepo != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		n, err := ms.UserRepo.CountUsers(ctx)
		cancel()
		if err == nil {
			userCount = n
		}
	}
	srvCount := 0
	activeCount := 0
	if ms.ServerRepo != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		rows, err := ms.ServerRepo.List(ctx, false)
		if err == nil {
			srvCount = len(rows)
		}
		act, errA := ms.ServerRepo.List(ctx, true)
		if errA == nil {
			activeCount = len(act)
		}
	}

	kmsReady := ms.ServerKMS != nil
	needsTimescaleUI := !docker && !tsOK
	needsOnboarding := tsOK && userCount > 0 && activeCount == 0 && kmsReady
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"deployment":                deployment,
		"docker_mode":               docker,
		"dedicated_mode":            !docker,
		"timescale_connected":       tsOK,
		"needs_timescale":           !tsOK,
		"needs_dedicated_timescale": needsTimescaleUI,
		"needs_bootstrap_admin":     tsOK && userCount == 0,
		"needs_onboarding_servers":  needsOnboarding,
		"user_count":                userCount,
		"monitoring_server_count":   srvCount,
		"monitoring_active_count":   activeCount,
		"kms_ready":                 kmsReady,
		"vault_configured":          h.deps.VaultAddrSet,
		"local_kms":                 h.deps.UsingLocalKMS,
		"public_setup_disabled":     h.deps.DisablePublicSetup,
	})
}

func (h *SetupHandlers) PostTimescale(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if h == nil || h.deps == nil || h.deps.Metrics == nil || h.deps.Cfg == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "setup not available"})
		return
	}
	if h.deps.DisablePublicSetup {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "public setup API is disabled"})
		return
	}
	if h.deps.Metrics.IsTimescaleConnected() && !h.deps.AllowTimescaleReconf && config.TimescalePersistFileExists(h.deps.ConfigPath) {
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "timescale already connected; set ALLOW_TIMESCALE_RECONFIG=1 to replace persisted settings"})
		return
	}

	var req struct {
		Host            string `json:"host"`
		Port            any    `json:"port"`
		Database        string `json:"database"`
		Username        string `json:"username"`
		Password        string `json:"password"`
		SSLMode         string `json:"ssl_mode"`
		UseVault        bool   `json:"use_vault"`
		VaultSecretPath string `json:"vault_secret_path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
		return
	}

	var host, db, user, pass, ssl string
	var port int
	if req.UseVault {
		vctx, vcancel := context.WithTimeout(r.Context(), 30*time.Second)
		vh, vp, vdb, vu, vpass, vssl, verr := setupsql.TimescaleFromVault(vctx, strings.TrimSpace(req.VaultSecretPath))
		vcancel()
		if verr != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": verr.Error()})
			return
		}
		host, port, db, user, pass, ssl = vh, vp, vdb, vu, vpass, vssl
	} else {
		port = 5432
		switch v := req.Port.(type) {
		case float64:
			port = int(v)
		case string:
			if p, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && p > 0 {
				port = p
			}
		case int:
			port = v
		}
		host = strings.TrimSpace(req.Host)
		db = strings.TrimSpace(req.Database)
		user = strings.TrimSpace(req.Username)
		pass = req.Password
		ssl = strings.TrimSpace(req.SSLMode)
		if ssl == "" {
			ssl = "require"
		}
		if host == "" || db == "" || user == "" || pass == "" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "host, database, username, and password are required"})
			return
		}
		if port <= 0 || port > 65535 {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid port"})
			return
		}
	}

	cfg := &hot.Config{
		Host:     host,
		Port:     strconv.Itoa(port),
		User:     user,
		Password: pass,
		Database: db,
		SSLMode:  ssl,
		MaxConns: 50,
	}

	ts, err := hot.New(cfg)
	if err != nil {
		log.Printf("[setup/timescale] connect failed: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "could not connect to TimescaleDB; check host, credentials, ssl_mode, and that the schema has been applied"})
		return
	}

	if err := config.SavePersistedTimescaleConfig(h.deps.ConfigPath, h.deps.JWTSecret, cfg); err != nil {
		ts.Close()
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "connected but failed to save encrypted configuration"})
		return
	}

	h.deps.Metrics.RebindTimescale(ts)
	h.deps.Metrics.EnsureServerKMS(h.deps.JWTSecret)
	if h.deps.ReloadFromRegistry != nil {
		h.deps.ReloadFromRegistry()
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "TimescaleDB connection saved. Continue with creating the administrator account.",
	})
}

func (h *SetupHandlers) PostBootstrapAdmin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if h == nil || h.deps == nil || h.deps.Metrics == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "setup not available"})
		return
	}
	if h.deps.DisablePublicSetup {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "public setup API is disabled"})
		return
	}
	if !h.deps.Metrics.IsTimescaleConnected() || h.deps.Metrics.UserRepo == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "timescale / user store not available"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	n, err := h.deps.Metrics.UserRepo.CountUsers(ctx)
	cancel()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "failed to read user directory"})
		return
	}
	if n > 0 {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "administrator already exists"})
		return
	}

	var req struct {
		Username       string `json:"username"`
		Password       string `json:"password"`
		PasswordVerify string `json:"password_verify"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
		return
	}
	u := strings.TrimSpace(req.Username)
	if u == "" || len(req.Password) < 8 {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "username required and password must be at least 8 characters"})
		return
	}
	if len(req.Password) != len(req.PasswordVerify) || subtle.ConstantTimeCompare([]byte(req.Password), []byte(req.PasswordVerify)) != 1 {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "password and confirmation do not match"})
		return
	}

	ctx2, cancel2 := context.WithTimeout(r.Context(), 15*time.Second)
	created, err := h.deps.Metrics.UserRepo.CreateUser(ctx2, u, req.Password, "admin")
	cancel2()
	if err != nil {
		log.Printf("[setup/bootstrap-admin] create failed: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "could not create user (username taken or database error)"})
		return
	}

	if h.deps.Metrics.AuditRepo != nil {
		actx, acancel := context.WithTimeout(r.Context(), 3*time.Second)
		_ = h.deps.Metrics.AuditRepo.Log(actx, "SETUP_ADMIN_CREATED", "", created.Username, clientIP(r), map[string]interface{}{"user_id": created.UserID})
		acancel()
	}

	tok, terr := middleware.GenerateToken(created.UserID, created.Username, created.Role)
	if terr != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "user created but token generation failed; log in manually"})
		return
	}

	// Set HttpOnly auth cookie so the browser is immediately authenticated
	// (same logic as the Login handler).
	http.SetCookie(w, &http.Cookie{
		Name:     middleware.AuthCookieName,
		Value:    tok,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400,
	})

	// Set CSRF token cookie (JS-readable).
	csrfToken, csrfErr := middleware.GenerateCSRFToken()
	if csrfErr == nil {
		middleware.SetCSRFCookie(w, r, csrfToken)
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"token":    tok,
		"user_id":  created.UserID,
		"username": created.Username,
		"role":     created.Role,
		"message":  "Administrator created. You can add monitored SQL Server / PostgreSQL instances from the Admin panel.",
	})
}

// PostTimescaleMigrateStep runs one of 00_timescale_schema.sql, 01_seed_data.sql, 02_rule_engine.sql
// against the provided database (simple-query / multi-statement). Used by the setup wizard.
func (h *SetupHandlers) PostTimescaleMigrateStep(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if h == nil || h.deps == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "setup not available"})
		return
	}
	if h.deps.DisablePublicSetup {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "public setup API is disabled"})
		return
	}

	var req struct {
		Step            int    `json:"step"`
		Host            string `json:"host"`
		Port            any    `json:"port"`
		Database        string `json:"database"`
		Username        string `json:"username"`
		Password        string `json:"password"`
		SSLMode         string `json:"ssl_mode"`
		UseVault        bool   `json:"use_vault"`
		VaultSecretPath string `json:"vault_secret_path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
		return
	}
	if req.Step < 0 || req.Step > 2 {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "step must be 0, 1, or 2"})
		return
	}

	ctx := r.Context()
	var host, db, user, pass, ssl string
	var port int
	if req.UseVault {
		vctx, vcancel := context.WithTimeout(ctx, 30*time.Second)
		vh, vp, vdb, vu, vpass, vssl, verr := setupsql.TimescaleFromVault(vctx, strings.TrimSpace(req.VaultSecretPath))
		vcancel()
		if verr != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": verr.Error(), "step": req.Step})
			return
		}
		host, port, db, user, pass, ssl = vh, vp, vdb, vu, vpass, vssl
	} else {
		port = 5432
		switch v := req.Port.(type) {
		case float64:
			port = int(v)
		case string:
			if p, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && p > 0 {
				port = p
			}
		case int:
			port = v
		}
		host = strings.TrimSpace(req.Host)
		db = strings.TrimSpace(req.Database)
		user = strings.TrimSpace(req.Username)
		pass = req.Password
		ssl = strings.TrimSpace(req.SSLMode)
		if ssl == "" {
			ssl = "require"
		}
		if host == "" || db == "" || user == "" || pass == "" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "host, database, username, and password are required (or enable Vault with a valid secret path)"})
			return
		}
		if port <= 0 || port > 65535 {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid port"})
			return
		}
	}

	stepTimeout := 50 * time.Minute
	if req.Step > 0 {
		stepTimeout = 20 * time.Minute
	}
	mctx, mcancel := context.WithTimeout(ctx, stepTimeout)
	defer mcancel()

	dir, err := setupsql.ResolveMigrationsDir()
	if err != nil {
		log.Printf("[setup/migrate-step] resolve sql dir: %v", err)
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error(), "step": req.Step})
		return
	}

	p := setupsql.TSConnParams{
		Host:     host,
		Port:     port,
		Database: db,
		Username: user,
		Password: pass,
		SSLMode:  ssl,
	}
	file, summary, err := setupsql.RunMigrationStep(mctx, dir, req.Step, p)
	if err != nil {
		log.Printf("[setup/migrate-step] step %d failed: %v", req.Step, err)
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
			"file":    file,
			"step":    req.Step,
		})
		return
	}
	next := req.Step + 1
	if next > 2 {
		next = -1
	}
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":   true,
		"file":      file,
		"summary":   setupsql.TruncateOutput(summary, 14000),
		"step":      req.Step,
		"next_step": next,
	})
}

// PostTestTimescale tests a Timescale connection without persisting (step 1 validation).
func (h *SetupHandlers) PostTestTimescale(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if h == nil || h.deps == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "setup not available"})
		return
	}
	if h.deps.DisablePublicSetup {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "public setup API is disabled"})
		return
	}

	var req struct {
		Host            string `json:"host"`
		Port            any    `json:"port"`
		Database        string `json:"database"`
		Username        string `json:"username"`
		Password        string `json:"password"`
		SSLMode         string `json:"ssl_mode"`
		UseVault        bool   `json:"use_vault"`
		VaultSecretPath string `json:"vault_secret_path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
		return
	}
	var host, db, user, pass, ssl string
	var port int
	if req.UseVault {
		vctx, vcancel := context.WithTimeout(r.Context(), 30*time.Second)
		vh, vp, vdb, vu, vpass, vssl, verr := setupsql.TimescaleFromVault(vctx, strings.TrimSpace(req.VaultSecretPath))
		vcancel()
		if verr != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": verr.Error()})
			return
		}
		host, port, db, user, pass, ssl = vh, vp, vdb, vu, vpass, vssl
	} else {
		port = 5432
		switch v := req.Port.(type) {
		case float64:
			port = int(v)
		case string:
			if p, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && p > 0 {
				port = p
			}
		case int:
			port = v
		}
		host = strings.TrimSpace(req.Host)
		db = strings.TrimSpace(req.Database)
		user = strings.TrimSpace(req.Username)
		pass = req.Password
		ssl = strings.TrimSpace(req.SSLMode)
		if ssl == "" {
			ssl = "require"
		}
		if host == "" || db == "" || user == "" || pass == "" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "host, database, username, and password are required"})
			return
		}
		if port <= 0 || port > 65535 {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "invalid port"})
			return
		}
	}
	cfg := &hot.Config{
		Host:     host,
		Port:     strconv.Itoa(port),
		User:     user,
		Password: pass,
		Database: db,
		SSLMode:  ssl,
		MaxConns: 5,
	}
	ts, err := hot.New(cfg)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "connection failed"})
		return
	}
	ts.Close()
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}
