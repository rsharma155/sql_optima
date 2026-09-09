// SPDX-License-Identifier: MIT
package handlers

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/rsharma155/sql_optima/internal/middleware"
	"github.com/rsharma155/sql_optima/internal/oscollectorbundle"
	"github.com/rsharma155/sql_optima/internal/apiresponse"
	"github.com/rsharma155/sql_optima/internal/service"
)

// OSCollectorHandlers serves OS collector setup status and bundle download.
type OSCollectorHandlers struct {
	metricsSvc     *service.MetricsService
	osAgentRevoker OSAgentTokenRevoker
}

func NewOSCollectorHandlers(svc *service.MetricsService) *OSCollectorHandlers {
	return &OSCollectorHandlers{metricsSvc: svc}
}

// Status returns ingest enablement and whether recent host memory samples exist.
func (h *OSCollectorHandlers) Status(w http.ResponseWriter, r *http.Request) {
	instance := strings.TrimSpace(r.URL.Query().Get("instance"))
	if instance == "" {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "instance query parameter is required"})
		return
	}
	if h.metricsSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"error": "metrics service not configured"})
		return
	}

	st, err := h.metricsSvc.GetOSCollectorStatus(r.Context(), instance)
	if err != nil {
		apiresponse.WriteJSONError(w, http.StatusBadRequest, "failed to resolve OS collector status", err, "handler", "os_collector", "instance", instance)
		return
	}
	metricsBase := strings.TrimSpace(r.URL.Query().Get("metrics_base_url"))
	st["metrics_url"] = resolveOSMetricsURL(r, metricsBase)
	if metricsBase != "" {
		st["app_url"] = metricsBase
	} else {
		st["app_url"] = strings.TrimSuffix(st["metrics_url"].(string), "/api/os/metrics")
	}
	writeJSON(w, http.StatusOK, st)
}

// DownloadBundle streams a zip tailored to the monitored instance.
func (h *OSCollectorHandlers) DownloadBundle(w http.ResponseWriter, r *http.Request) {
	instance := strings.TrimSpace(r.URL.Query().Get("instance"))
	if instance == "" {
		http.Error(w, "instance query parameter is required", http.StatusBadRequest)
		return
	}

	metricsBase := strings.TrimSpace(r.URL.Query().Get("metrics_base_url"))
	metricsURL := resolveOSMetricsURL(r, metricsBase)
	appURL := metricsBase
	if appURL == "" {
		appURL = strings.TrimSuffix(metricsURL, "/api/os/metrics")
	}

	serverID := ""
	if h.metricsSvc != nil {
		if id, err := h.metricsSvc.ResolveServerIDByInstanceName(r.Context(), instance); err == nil {
			serverID = id.String()
		}
	}

	if h.metricsSvc != nil {
		actor := "system"
		if claims := middleware.GetAuthClaims(r); claims != nil && claims.Username != "" {
			actor = claims.Username
		}
		if err := h.metricsSvc.SetOSMetricsIngestEnabled(r.Context(), true, actor); err != nil {
			slog.Warn("[OSCollector] auto-enable ingest on bundle download", "err", err)
		}
	}

	data, err := oscollectorbundle.BuildZip(oscollectorbundle.Params{
		InstanceName: instance,
		ServerID:     serverID,
		AppURL:       appURL,
		MetricsURL:   metricsURL,
		GeneratedAt:  time.Now().UTC(),
	})
	if err != nil {
		apiresponse.WritePlainError(w, http.StatusInternalServerError, "failed to build OS collector bundle", err, "handler", "os_collector")
		return
	}

	filename := fmt.Sprintf("sql-optima-os-collector-%s.zip", oscollectorbundle.SanitizeFilename(instance))
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func resolveOSMetricsURL(r *http.Request, baseOverride string) string {
	if pub := strings.TrimSpace(os.Getenv("SQL_OPTIMA_PUBLIC_URL")); pub != "" {
		return strings.TrimRight(pub, "/") + "/api/os/metrics"
	}
	base := strings.TrimSpace(baseOverride)
	if base == "" {
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
			scheme = strings.ToLower(strings.TrimSpace(strings.Split(proto, ",")[0]))
		}
		host := r.Host
		if fwd := r.Header.Get("X-Forwarded-Host"); fwd != "" {
			host = strings.TrimSpace(strings.Split(fwd, ",")[0])
		}
		base = scheme + "://" + host
	}
	return strings.TrimRight(base, "/") + "/api/os/metrics"
}

// GetIngestConfig returns whether OS metrics ingest is enabled (env, database, or default).
func (h *OSCollectorHandlers) GetIngestConfig(w http.ResponseWriter, r *http.Request) {
	if h.metricsSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"error": "metrics service not configured"})
		return
	}
	info := h.metricsSvc.OSMetricsIngestInfo(r.Context())
	writeJSON(w, http.StatusOK, info)
}

// SetIngestConfig enables or disables OS metrics ingest without restarting the API.
func (h *OSCollectorHandlers) SetIngestConfig(w http.ResponseWriter, r *http.Request) {
	if h.metricsSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"error": "metrics service not configured"})
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "invalid JSON body"})
		return
	}
	actor := ""
	if claims := middleware.GetAuthClaims(r); claims != nil {
		actor = claims.Username
	}
	if err := h.metricsSvc.SetOSMetricsIngestEnabled(r.Context(), body.Enabled, actor); err != nil {
		if err == service.ErrOSMetricsIngestEnvLocked {
			writeJSON(w, http.StatusConflict, map[string]interface{}{
				"error":   err.Error(),
				"details": "Unset OS_METRICS_INGEST_ENABLED or use only 0/1 in deployment env to allow UI control.",
			})
			return
		}
		apiresponse.WriteJSONError(w, http.StatusInternalServerError, "failed to update OS ingest setting", err, "handler", "os_collector")
		return
	}
	info := h.metricsSvc.OSMetricsIngestInfo(r.Context())
	writeJSON(w, http.StatusOK, info)
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
