package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/rsharma155/sql_optima/internal/domain/servers"
	"github.com/rsharma155/sql_optima/internal/middleware"
	"github.com/rsharma155/sql_optima/internal/service"
	"github.com/rsharma155/sql_optima/internal/sqlserver"
)

type AdminServerHandlers struct {
	metrics *service.MetricsService
	tester  ServerConnectionTester
}

func NewAdminServerHandlers(metrics *service.MetricsService) *AdminServerHandlers {
	return &AdminServerHandlers{metrics: metrics}
}

func (h *AdminServerHandlers) reg() (store servers.ServerStore, kms servers.KeyManagementService, box servers.SecretBox, audit servers.AuditLogger) {
	if h == nil || h.metrics == nil {
		return nil, nil, nil, nil
	}
	m := h.metrics
	var aud servers.AuditLogger
	if m.AuditRepo != nil {
		aud = m.AuditRepo
	}
	return m.ServerRepo, m.ServerKMS, m.ServerSecretBox, aud
}

type ServerConnectionTester interface {
	Test(ctx context.Context, s servers.Server, cred servers.CredentialPayload) error
}

type defaultServerConnectionTester struct{}

func sanitizeDBError(err error, dbType string) error {
	if err == nil {
		return nil
	}
	errStr := strings.ToLower(err.Error())
	switch {
	case strings.Contains(errStr, "no such host") || strings.Contains(errStr, "lookup"):
		return errors.New("host not found or unreachable")
	case strings.Contains(errStr, "connection refused") || strings.Contains(errStr, "connection reset"):
		return errors.New("connection refused - check host and port")
	case strings.Contains(errStr, "authentication failed") || strings.Contains(errStr, "password") || strings.Contains(errStr, "login failed"):
		return errors.New("authentication failed - check username and password")
	case strings.Contains(errStr, "ssl") || strings.Contains(errStr, "certificate"):
		return errors.New("SSL/TLS error - check SSL mode or certificates")
	case strings.Contains(errStr, "timeout"):
		return errors.New("connection timeout - server may be slow or unreachable")
	default:
		return errors.New("connection failed - check server details")
	}
}

func sqlServerInitialCatalog(cred servers.CredentialPayload) string {
	d := strings.TrimSpace(cred.Database)
	if d != "" {
		return d
	}
	return "master"
}

func postgresDBName(cred servers.CredentialPayload, s servers.Server) string {
	if d := strings.TrimSpace(cred.Database); d != "" {
		return d
	}
	return "postgres"
}

func postgresSSLMode(cred servers.CredentialPayload, s servers.Server) string {
	if m := strings.TrimSpace(cred.SSLMode); m != "" {
		return m
	}
	m := strings.TrimSpace(string(s.SSLMode))
	if m == "" {
		return "require"
	}
	return m
}

func (t defaultServerConnectionTester) Test(ctx context.Context, s servers.Server, cred servers.CredentialPayload) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	password := cred.Password

	switch s.DBType {
	case servers.DBPostgres:
		sslmode := postgresSSLMode(cred, s)
		dbname := postgresDBName(cred, s)
		dsn := "host=" + s.Host +
			" port=" + itoa(s.Port) +
			" user=" + s.Username +
			" password=" + password +
			" dbname=" + dbname +
			" sslmode=" + sslmode
		db, err := sql.Open("postgres", dsn)
		if err != nil {
			return sanitizeDBError(err, "postgres")
		}
		defer db.Close()
		err = db.PingContext(ctx)
		if err != nil {
			return sanitizeDBError(err, "postgres")
		}
		return nil

	case servers.DBSQLServer:
		// Azure SQL / Managed Instance / RDS SQL: encrypt mandatory; trust optional when validating server cert is impractical.
		cat := sqlServerInitialCatalog(cred)
		trust := "false"
		if cred.TrustServerCertificate {
			trust = "true"
		}
		connStr := "server=" + s.Host +
			";port=" + itoa(s.Port) +
			";database=" + cat +
			";user id=" + s.Username +
			";password=" + password +
			";encrypt=true;trustservercertificate=" + trust + ";"
		db, err := sqlserver.OpenMetricsPool(connStr)
		if err != nil {
			return sanitizeDBError(err, "sqlserver")
		}
		defer db.Close()
		err = db.PingContext(ctx)
		if err != nil {
			return sanitizeDBError(err, "sqlserver")
		}
		return nil
	default:
		return errors.New("unsupported db_type")
	}
}

func (h *AdminServerHandlers) notifyRegistryChanged() {
	if h == nil || h.metrics == nil || h.metrics.RegistryReload == nil {
		return
	}
	h.metrics.RegistryReload()
}

// TestServerDraft checks connectivity using the same rules as "Test" on a saved server, without persisting.
func (h *AdminServerHandlers) TestServerDraft(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var req struct {
		Name                   string `json:"name"`
		DBType                 string `json:"db_type"`
		Host                   string `json:"host"`
		Port                   any    `json:"port"`
		Username               string `json:"username"`
		Password               string `json:"password"`
		SSLMode                string `json:"ssl_mode"`
		Database               string `json:"database"`
		TrustServerCertificate bool   `json:"trust_server_certificate"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
		return
	}
	port := 0
	switch v := req.Port.(type) {
	case float64:
		port = int(v)
	case string:
		if p, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			port = p
		}
	case int:
		port = v
	}
	in := servers.CreateServerInput{
		Name:                   req.Name,
		DBType:                 servers.DBType(strings.TrimSpace(req.DBType)),
		Host:                   req.Host,
		Port:                   port,
		Username:               req.Username,
		Password:               req.Password,
		SSLMode:                req.SSLMode,
		Database:               strings.TrimSpace(req.Database),
		TrustServerCertificate: req.TrustServerCertificate,
		Actor:                  "",
	}
	if err := in.Validate(); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	now := time.Now().UTC()
	s := servers.Server{
		Name:      strings.TrimSpace(in.Name),
		DBType:    in.DBType,
		Host:      strings.TrimSpace(in.Host),
		Port:      in.Port,
		Username:  strings.TrimSpace(in.Username),
		AuthType:  servers.AuthStatic,
		SSLMode:   servers.SSLMode(strings.TrimSpace(in.SSLMode)),
		IsActive:  true,
		CreatedAt: now,
		UpdatedAt: now,
	}
	cred := servers.CredentialPayload{
		Password:               in.Password,
		SSLMode:                strings.TrimSpace(in.SSLMode),
		Database:               strings.TrimSpace(in.Database),
		TrustServerCertificate: in.TrustServerCertificate,
	}
	defer zeroString(&cred.Password)
	tester := h.tester
	if tester == nil {
		tester = defaultServerConnectionTester{}
	}
	err := tester.Test(r.Context(), s, cred)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}

func (h *AdminServerHandlers) AddServer(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	store, kms, box, audit := h.reg()
	if h == nil || store == nil || kms == nil || box == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "server registry not configured"})
		return
	}

	var req struct {
		Name                   string `json:"name"`
		DBType                 string `json:"db_type"`
		Host                   string `json:"host"`
		Port                   int    `json:"port"`
		Username               string `json:"username"`
		Password               string `json:"password"`
		SSLMode                string `json:"ssl_mode"`
		Database               string `json:"database"`                 // PG: dbname (default postgres). SQL Server: catalog (default master).
		TrustServerCertificate bool   `json:"trust_server_certificate"` // SQL Server (Azure / MI): allow TLS without full CA validation.
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
		return
	}

	claims := middleware.GetAuthClaims(r)
	actor := ""
	if claims != nil {
		actor = claims.Username
	}

	in := servers.CreateServerInput{
		Name:                   req.Name,
		DBType:                 servers.DBType(strings.TrimSpace(req.DBType)),
		Host:                   req.Host,
		Port:                   req.Port,
		Username:               req.Username,
		Password:               req.Password,
		SSLMode:                req.SSLMode,
		Database:               strings.TrimSpace(req.Database),
		TrustServerCertificate: req.TrustServerCertificate,
		Actor:                  actor,
	}
	if err := in.Validate(); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	// Create credential payload JSON (server-side only).
	credJSON, err := json.Marshal(servers.CredentialPayload{
		Password:               in.Password,
		SSLMode:                strings.TrimSpace(in.SSLMode),
		Database:               strings.TrimSpace(in.Database),
		TrustServerCertificate: in.TrustServerCertificate,
		Extra:                  map[string]interface{}{},
	})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "failed to encode credential payload"})
		return
	}

	// Envelope encryption: DEK from KMS, encrypt credentials, store encrypted_secret + encrypted_dek.
	ctx := r.Context()
	plaintextDEK, encryptedDEK, err := kms.GenerateDataKey(ctx)
	if err != nil {
		log.Printf("[admin/servers] GenerateDataKey failed: %v", err)
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "key management unavailable"})
		return
	}
	defer zeroBytes(plaintextDEK)

	encryptedSecret, err := box.Encrypt(credJSON, plaintextDEK)
	zeroBytes(credJSON)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "failed to encrypt credentials"})
		return
	}

	now := time.Now().UTC()
	s := servers.Server{
		Name:      strings.TrimSpace(in.Name),
		DBType:    in.DBType,
		Host:      strings.TrimSpace(in.Host),
		Port:      in.Port,
		Username:  strings.TrimSpace(in.Username),
		AuthType:  servers.AuthStatic,
		SSLMode:   servers.SSLMode(strings.TrimSpace(in.SSLMode)),
		IsActive:  true,
		CreatedAt: now,
		UpdatedAt: now,
		CreatedBy: actor,
	}

	created, err := store.Create(ctx, s, encryptedSecret, encryptedDEK)
	if err != nil {
		log.Printf("[admin/servers] store.Create failed: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "failed to persist server"})
		return
	}

	// Best-effort audit log.
	if audit != nil {
		_ = audit.Log(ctx, "SERVER_ADDED", created.ID, actor, clientIP(r), map[string]interface{}{
			"name":    created.Name,
			"db_type": string(created.DBType),
			"host":    created.Host,
			"port":    created.Port,
		})
	}

	h.notifyRegistryChanged()

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"server": map[string]interface{}{
			"id":        created.ID,
			"name":      created.Name,
			"db_type":   created.DBType,
			"host":      created.Host,
			"port":      created.Port,
			"username":  created.Username,
			"ssl_mode":  created.SSLMode,
			"is_active": created.IsActive,
		},
	})
}

func (h *AdminServerHandlers) ListServers(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	store, _, _, _ := h.reg()
	if h == nil || store == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "server registry not configured"})
		return
	}
	rows, err := store.List(r.Context(), false)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "failed to list servers"})
		return
	}

	out := make([]map[string]interface{}, 0, len(rows))
	for _, s := range rows {
		out = append(out, map[string]interface{}{
			"id":        s.ID,
			"name":      s.Name,
			"db_type":   s.DBType,
			"host":      s.Host,
			"port":      s.Port,
			"username":  s.Username,
			"ssl_mode":  s.SSLMode,
			"is_active": s.IsActive,
			"last_tested": func() any {
				if s.LastTestAt == nil || s.LastTestAt.IsZero() {
					return nil
				}
				return s.LastTestAt.UTC().Format(time.RFC3339)
			}(),
			"created_at": func() any {
				if s.CreatedAt.IsZero() {
					return nil
				}
				return s.CreatedAt.UTC().Format(time.RFC3339)
			}(),
			"updated_at": func() any {
				if s.UpdatedAt.IsZero() {
					return nil
				}
				return s.UpdatedAt.UTC().Format(time.RFC3339)
			}(),
		})
	}
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"servers": out})
}

func (h *AdminServerHandlers) TestServer(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	store, kms, box, audit := h.reg()
	if h == nil || store == nil || kms == nil || box == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "server registry not configured"})
		return
	}
	vars := mux.Vars(r)
	id := strings.TrimSpace(vars["id"])
	if id == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "id is required"})
		return
	}

	claims := middleware.GetAuthClaims(r)
	actor := ""
	if claims != nil {
		actor = claims.Username
	}

	s, encSecret, encDEK, err := store.GetEncrypted(r.Context(), id)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "server not found"})
		return
	}
	plaintextDEK, err := kms.DecryptDataKey(r.Context(), encDEK)
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "key management unavailable"})
		return
	}
	defer zeroBytes(plaintextDEK)

	plainJSON, err := box.Decrypt(encSecret, plaintextDEK)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "failed to decrypt credentials"})
		return
	}
	defer zeroBytes(plainJSON)

	var cred servers.CredentialPayload
	if err := json.Unmarshal(plainJSON, &cred); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "credential payload invalid"})
		return
	}
	if strings.TrimSpace(cred.Password) == "" {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "credential payload missing password"})
		return
	}
	defer zeroString(&cred.Password)

	tester := h.tester
	if tester == nil {
		tester = defaultServerConnectionTester{}
	}
	err = tester.Test(r.Context(), s, cred)
	ok := err == nil

	if !ok {
		log.Printf("[AdminServer] Test connection failed for server %s (%s:%d): %v", s.Name, s.Host, s.Port, err)
	}

	if audit != nil {
		_ = audit.Log(r.Context(), "CREDENTIAL_ACCESSED", s.ID, actor, clientIP(r), map[string]interface{}{
			"action":  "test_connection",
			"success": ok,
			"db_type": string(s.DBType),
		})
	}

	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}
	if store != nil {
		_ = store.TouchLastTest(r.Context(), id, time.Now().UTC())
	}
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}

func (h *AdminServerHandlers) DeleteServer(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	store, _, _, audit := h.reg()
	if h == nil || store == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "server registry not configured"})
		return
	}
	id := strings.TrimSpace(mux.Vars(r)["id"])
	if id == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "id is required"})
		return
	}
	claims := middleware.GetAuthClaims(r)
	actor := ""
	if claims != nil {
		actor = claims.Username
	}
	if err := store.Delete(r.Context(), id); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if audit != nil {
		_ = audit.Log(r.Context(), "SERVER_DELETED", id, actor, clientIP(r), map[string]interface{}{"permanent": true})
	}
	h.notifyRegistryChanged()
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}

func (h *AdminServerHandlers) PatchServer(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	store, _, _, audit := h.reg()
	if h == nil || store == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "server registry not configured"})
		return
	}
	id := strings.TrimSpace(mux.Vars(r)["id"])
	if id == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "id is required"})
		return
	}
	var req struct {
		IsActive *bool `json:"is_active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
		return
	}
	if req.IsActive == nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "is_active is required"})
		return
	}
	claims := middleware.GetAuthClaims(r)
	actor := ""
	if claims != nil {
		actor = claims.Username
	}
	if err := store.SetActive(r.Context(), id, *req.IsActive); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "failed to update server"})
		return
	}
	if audit != nil {
		_ = audit.Log(r.Context(), "SERVER_UPDATED", id, actor, clientIP(r), map[string]interface{}{
			"is_active": *req.IsActive,
		})
	}
	h.notifyRegistryChanged()
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}

func (h *AdminServerHandlers) RotateServer(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	store, kms, box, audit := h.reg()
	if h == nil || store == nil || kms == nil || box == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "server registry not configured"})
		return
	}
	id := strings.TrimSpace(mux.Vars(r)["id"])
	if id == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "id is required"})
		return
	}
	var req struct {
		Password                 string `json:"password"`
		SSLMode                  string `json:"ssl_mode"`
		Database                 string `json:"database"`
		TrustServerCertificate   *bool  `json:"trust_server_certificate"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
		return
	}
	if strings.TrimSpace(req.Password) == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "password is required"})
		return
	}

	s, encSecret, encDEK, err := store.GetEncrypted(r.Context(), id)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "server not found"})
		return
	}

	ssl := strings.TrimSpace(req.SSLMode)
	if ssl == "" {
		ssl = strings.TrimSpace(string(s.SSLMode))
	}
	if ssl == "" {
		ssl = "require"
	}

	ctx := r.Context()
	oldDEK, err := kms.DecryptDataKey(ctx, encDEK)
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "key management unavailable"})
		return
	}
	defer zeroBytes(oldDEK)
	prevJSON, err := box.Decrypt(encSecret, oldDEK)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "failed to decrypt credentials"})
		return
	}
	defer zeroBytes(prevJSON)
	var prev servers.CredentialPayload
	_ = json.Unmarshal(prevJSON, &prev)

	cred := servers.CredentialPayload{
		Password:               req.Password,
		SSLMode:                ssl,
		Database:               strings.TrimSpace(prev.Database),
		TrustServerCertificate: prev.TrustServerCertificate,
		Extra:                  prev.Extra,
	}
	if cred.Extra == nil {
		cred.Extra = map[string]interface{}{}
	}
	if strings.TrimSpace(req.Database) != "" {
		cred.Database = strings.TrimSpace(req.Database)
	}
	if req.TrustServerCertificate != nil {
		cred.TrustServerCertificate = *req.TrustServerCertificate
	}

	credJSON, err := json.Marshal(cred)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "failed to encode credential payload"})
		return
	}

	newPlainDEK, encryptedDEK, err := kms.GenerateDataKey(ctx)
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "key management unavailable"})
		return
	}
	defer zeroBytes(newPlainDEK)

	encryptedSecret, err := box.Encrypt(credJSON, newPlainDEK)
	zeroBytes(credJSON)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "failed to encrypt credentials"})
		return
	}

	if err := store.UpdateCredentials(ctx, id, encryptedSecret, encryptedDEK); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "failed to persist credentials"})
		return
	}

	claims := middleware.GetAuthClaims(r)
	actor := ""
	if claims != nil {
		actor = claims.Username
	}
	if audit != nil {
		_ = audit.Log(ctx, "CREDENTIAL_ROTATED", id, actor, clientIP(r), map[string]interface{}{
			"db_type": string(s.DBType),
		})
	}
	h.notifyRegistryChanged()
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}

func (h *AdminServerHandlers) UpdateServer(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	store, kms, box, audit := h.reg()
	if h == nil || store == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "server registry not configured"})
		return
	}
	id := strings.TrimSpace(mux.Vars(r)["id"])
	if id == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "id is required"})
		return
	}

	var req struct {
		Name                   string `json:"name"`
		Host                   string `json:"host"`
		Port                   any    `json:"port"`
		Username               string `json:"username"`
		Password               string `json:"password"`
		SSLMode                string `json:"ssl_mode"`
		Database               string `json:"database"`
		TrustServerCertificate *bool  `json:"trust_server_certificate,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
		return
	}

	port := 0
	switch v := req.Port.(type) {
	case float64:
		port = int(v)
	case string:
		if p, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			port = p
		}
	case int:
		port = v
	}

	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Host) == "" || strings.TrimSpace(req.Username) == "" || port <= 0 || port > 65535 {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "name, host, username, and valid port are required"})
		return
	}

	claims := middleware.GetAuthClaims(r)
	actor := ""
	if claims != nil {
		actor = claims.Username
	}

	sslMode := strings.TrimSpace(req.SSLMode)
	if sslMode == "" {
		sslMode = "require"
	}

	if strings.TrimSpace(req.Password) != "" {
		if kms == nil || box == nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "key management unavailable"})
			return
		}
		s, _, _, err := store.GetEncrypted(r.Context(), id)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "server not found"})
			return
		}
		credPay := servers.CredentialPayload{
			Password:               req.Password,
			SSLMode:                sslMode,
			Database:               strings.TrimSpace(req.Database),
			TrustServerCertificate: false,
		}
		if req.TrustServerCertificate != nil {
			credPay.TrustServerCertificate = *req.TrustServerCertificate
		}
		credJSON, err := json.Marshal(credPay)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "failed to encode credential payload"})
			return
		}
		ctx := r.Context()
		plaintextDEK, encryptedDEK, err := kms.GenerateDataKey(ctx)
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "key management unavailable"})
			return
		}
		defer zeroBytes(plaintextDEK)
		encryptedSecret, err := box.Encrypt(credJSON, plaintextDEK)
		zeroBytes(credJSON)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "failed to encrypt credentials"})
			return
		}
		if err := store.UpdateMetadata(ctx, id, req.Name, req.Host, port, req.Username, sslMode); err != nil {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "server not found"})
			return
		}
		if err := store.UpdateCredentials(ctx, id, encryptedSecret, encryptedDEK); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "failed to persist credentials"})
			return
		}
		if audit != nil {
			_ = audit.Log(ctx, "SERVER_UPDATED", id, actor, clientIP(r), map[string]interface{}{
				"db_type": string(s.DBType),
				"action":  "metadata_and_credentials",
			})
		}
		h.notifyRegistryChanged()
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
		return
	}

	needCredPatch := strings.TrimSpace(req.Database) != "" || req.TrustServerCertificate != nil
	if needCredPatch {
		if kms == nil || box == nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "key management unavailable"})
			return
		}
		ctx := r.Context()
		s, encSecret, encDEK, err := store.GetEncrypted(ctx, id)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "server not found"})
			return
		}
		dekPlain, err := kms.DecryptDataKey(ctx, encDEK)
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "key management unavailable"})
			return
		}
		defer zeroBytes(dekPlain)
		plainJSON, err := box.Decrypt(encSecret, dekPlain)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "failed to decrypt credentials"})
			return
		}
		defer zeroBytes(plainJSON)
		var cred servers.CredentialPayload
		if err := json.Unmarshal(plainJSON, &cred); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "credential payload invalid"})
			return
		}
		if strings.TrimSpace(req.Database) != "" {
			cred.Database = strings.TrimSpace(req.Database)
		}
		if req.TrustServerCertificate != nil {
			cred.TrustServerCertificate = *req.TrustServerCertificate
		}
		newJSON, err := json.Marshal(cred)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "failed to encode credential payload"})
			return
		}
		encryptedSecret, err := box.Encrypt(newJSON, dekPlain)
		zeroBytes(newJSON)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "failed to encrypt credentials"})
			return
		}
		if err := store.UpdateMetadata(ctx, id, req.Name, req.Host, port, req.Username, sslMode); err != nil {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "server not found"})
			return
		}
		if err := store.UpdateCredentials(ctx, id, encryptedSecret, encDEK); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "failed to persist credentials"})
			return
		}
		if audit != nil {
			_ = audit.Log(ctx, "SERVER_UPDATED", id, actor, clientIP(r), map[string]interface{}{
				"db_type": string(s.DBType),
				"action":  "metadata_and_credential_options",
			})
		}
		h.notifyRegistryChanged()
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
		return
	}

	if err := store.UpdateMetadata(r.Context(), id, req.Name, req.Host, port, req.Username, sslMode); err != nil {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "server not found"})
		return
	}
	if audit != nil {
		_ = audit.Log(r.Context(), "SERVER_UPDATED", id, actor, clientIP(r), map[string]interface{}{"action": "metadata"})
	}
	h.notifyRegistryChanged()
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}

func clientIP(r *http.Request) string {
	// Best-effort; prefer reverse-proxy header if present.
	if xf := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xf != "" {
		parts := strings.Split(xf, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}

func zeroBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

func zeroString(s *string) {
	if s == nil || *s == "" {
		return
	}
	// Best-effort: replace with same-length dummy.
	*s = strings.Repeat("0", len(*s))
}

func itoa(n int) string {
	// avoid strconv import in this file's hot path
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	var buf [32]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + (n % 10))
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
