package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/rsharma155/sql_optima/internal/domain/servers"
	"github.com/rsharma155/sql_optima/internal/middleware"
	"github.com/rsharma155/sql_optima/internal/service"
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
	store = m.ServerRepo
	kms = m.ServerKMS
	box = m.ServerSecretBox
	if m.AuditRepo != nil {
		audit = m.AuditRepo
	}
	return
}

type ServerConnectionTester interface {
	Test(ctx context.Context, s servers.Server, cred servers.CredentialPayload) error
}

func (h *AdminServerHandlers) ListServers(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	store, _, _, _ := h.reg()
	if store == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	list, err := store.List(r.Context(), false)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if list == nil {
		list = []servers.Server{}
	}
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"servers": list})
}

func (h *AdminServerHandlers) AddServer(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	store, kms, box, audit := h.reg()
	if store == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "server registry not available"})
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
		Database               string `json:"database"`
		TrustServerCertificate bool   `json:"trust_server_certificate"`
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

	// Encrypt credentials before storage.
	cred := servers.CredentialPayload{
		Password:               req.Password,
		SSLMode:                req.SSLMode,
		Database:               req.Database,
		TrustServerCertificate: req.TrustServerCertificate,
	}
	credJSON, err := json.Marshal(cred)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "failed to marshal credentials"})
		return
	}

	var encSecret, encDEK []byte
	if kms != nil && box != nil {
		plaintextDEK, eDEK, kerr := kms.GenerateDataKey(r.Context())
		if kerr != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "key generation failed: " + kerr.Error()})
			return
		}
		eSecret, berr := box.Encrypt(credJSON, plaintextDEK)
		if berr != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "credential encryption failed"})
			return
		}
		encSecret = eSecret
		encDEK = eDEK
	} else {
		// KMS not configured — store credential JSON directly (dev/local mode only).
		encSecret = credJSON
		encDEK = []byte("no-kms")
	}

	s := servers.Server{
		Name:      req.Name,
		DBType:    servers.DBType(req.DBType),
		Host:      req.Host,
		Port:      req.Port,
		Username:  req.Username,
		SSLMode:   servers.SSLMode(req.SSLMode),
		AuthType:  servers.AuthType("static"),
		IsActive:  true,
		CreatedBy: actor,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	res, err := store.Create(r.Context(), s, encSecret, encDEK)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if audit != nil {
		_ = audit.Log(r.Context(), "add_server", res.ID, actor, r.RemoteAddr, nil)
	}

	if h.metrics != nil && h.metrics.RegistryReload != nil {
		go h.metrics.RegistryReload()
	}

	_ = json.NewEncoder(w).Encode(res)
}

func (h *AdminServerHandlers) UpdateServer(w http.ResponseWriter, r *http.Request) {
	idStr := mux.Vars(r)["id"]
	id, err := uuid.Parse(idStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	store, _, _, audit := h.reg()
	var req struct {
		Name     string `json:"name"`
		Host     string `json:"host"`
		Port     int    `json:"port"`
		Username string `json:"username"`
		SSLMode  string `json:"ssl_mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	claims := middleware.GetAuthClaims(r)
	actor := ""
	if claims != nil {
		actor = claims.Username
	}

	err = store.UpdateMetadata(r.Context(), id, req.Name, req.Host, req.Port, req.Username, req.SSLMode)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if audit != nil {
		_ = audit.Log(r.Context(), "update_server", id, actor, r.RemoteAddr, nil)
	}
	if h.metrics != nil && h.metrics.RegistryReload != nil {
		go h.metrics.RegistryReload()
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AdminServerHandlers) DeleteServer(w http.ResponseWriter, r *http.Request) {
	idStr := mux.Vars(r)["id"]
	id, err := uuid.Parse(idStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	store, _, _, audit := h.reg()
	claims := middleware.GetAuthClaims(r)
	actor := ""
	if claims != nil {
		actor = claims.Username
	}

	err = store.Delete(r.Context(), id)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if audit != nil {
		_ = audit.Log(r.Context(), "delete_server", id, actor, r.RemoteAddr, nil)
	}
	if h.metrics != nil && h.metrics.RegistryReload != nil {
		go h.metrics.RegistryReload()
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AdminServerHandlers) TestServerDraft(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var req struct {
		DBType                 string `json:"db_type"`
		Host                   string `json:"host"`
		Port                   int    `json:"port"`
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

	ctx, cancel := context.WithTimeout(r.Context(), draftConnectTimeout)
	defer cancel()

	var probeErr error
	switch req.DBType {
	case "sqlserver":
		probeErr = probeSQLServer(ctx, req.Host, req.Port, req.Username, req.Password, req.Database, req.TrustServerCertificate)
	case "postgres":
		probeErr = probePostgres(ctx, req.Host, req.Port, req.Username, req.Password, req.Database, req.SSLMode)
	default:
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "unsupported db_type: " + req.DBType})
		return
	}

	if probeErr != nil {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": probeErr.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (h *AdminServerHandlers) ToggleServerActive(w http.ResponseWriter, r *http.Request) {
	idStr := mux.Vars(r)["id"]
	id, err := uuid.Parse(idStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	store, _, _, _ := h.reg()
	_ = store.SetActive(r.Context(), id, true)
	w.WriteHeader(http.StatusNoContent)
}

func (h *AdminServerHandlers) CheckPermissionsDraft(w http.ResponseWriter, r *http.Request) { w.Header().Set("Content-Type", "application/json"); _ = json.NewEncoder(w).Encode(map[string]interface{}{}) }
func (h *AdminServerHandlers) TestServer(w http.ResponseWriter, r *http.Request)            { w.Header().Set("Content-Type", "application/json"); _ = json.NewEncoder(w).Encode(map[string]interface{}{}) }
func (h *AdminServerHandlers) CheckPermissions(w http.ResponseWriter, r *http.Request)      { w.Header().Set("Content-Type", "application/json"); _ = json.NewEncoder(w).Encode(map[string]interface{}{}) }
func (h *AdminServerHandlers) RotateServer(w http.ResponseWriter, r *http.Request)          { w.Header().Set("Content-Type", "application/json"); _ = json.NewEncoder(w).Encode(map[string]interface{}{}) }
func (h *AdminServerHandlers) GetServer(w http.ResponseWriter, r *http.Request)             { w.Header().Set("Content-Type", "application/json"); _ = json.NewEncoder(w).Encode(map[string]interface{}{}) }
func (h *AdminServerHandlers) PatchServer(w http.ResponseWriter, r *http.Request)           { w.Header().Set("Content-Type", "application/json"); _ = json.NewEncoder(w).Encode(map[string]interface{}{}) }
