// Package handlers implements the API transport layer.
package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/rsharma155/sql_optima/internal/domain/servers"
	"github.com/rsharma155/sql_optima/internal/service"
)

type PermissionCheckHandlers struct {
	metrics *service.MetricsService
}

func NewPermissionCheckHandlers(metrics *service.MetricsService) *PermissionCheckHandlers {
	return &PermissionCheckHandlers{metrics: metrics}
}

func (h *PermissionCheckHandlers) reg() (store servers.ServerStore, kms servers.KeyManagementService, box servers.SecretBox, audit servers.AuditLogger) {
	if h == nil || h.metrics == nil {
		return nil, nil, nil, nil
	}
	m := h.metrics
	return m.ServerRepo, m.ServerKMS, m.ServerSecretBox, m.AuditRepo
}

func (h *PermissionCheckHandlers) CheckPermissions(w http.ResponseWriter, r *http.Request) {
	idStr := mux.Vars(r)["id"]
	id, err := uuid.Parse(idStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	store, _, _, _ := h.reg()
	if store == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	// Logic to check permissions... (Placeholder for fix)
	_ = id
	_ = json.NewEncoder(w).Encode(map[string]bool{"permitted": true})
}

func sanitizeDBError(err error, dbType string) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "tls:") || strings.Contains(msg, "x509") || strings.Contains(msg, "certificate"):
		return errors.New("SSL/TLS error - try enabling 'Trust server certificate' or set SSL mode to 'disable'")
	case strings.Contains(msg, "login failed") || strings.Contains(msg, "authentication") || strings.Contains(msg, "password"):
		return errors.New("authentication failed - check username and password")
	}
	return err
}
