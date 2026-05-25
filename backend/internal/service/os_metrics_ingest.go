// SPDX-License-Identifier: MIT
package service

import (
	"context"
	"errors"
	"os"
	"strings"

	"github.com/rsharma155/sql_optima/internal/repository"
)

// ErrOSMetricsIngestEnvLocked is returned when OS_METRICS_INGEST_ENABLED env overrides the DB toggle.
var ErrOSMetricsIngestEnvLocked = errors.New("os metrics ingest is controlled by OS_METRICS_INGEST_ENABLED environment variable")

// OSMetricsIngestInfo describes how host metrics ingest is enabled.
type OSMetricsIngestInfo struct {
	Enabled      bool   `json:"enabled"`
	Source       string `json:"source"` // env, database, default
	Configurable bool   `json:"configurable"`
	EnvValue     string `json:"env_value,omitempty"`
}

// ReloadOSMetricsIngest loads the database toggle into memory (no-op when env forces a value).
func (s *MetricsService) ReloadOSMetricsIngest(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if env := osMetricsIngestEnv(); env == "0" || env == "1" {
		s.setOSIngestCache(env == "1")
		return nil
	}
	enabled := false
	if s.platformSettings != nil {
		var err error
		enabled, err = s.platformSettings.GetBool(ctx, repository.PlatformSettingOSMetricsIngest)
		if err != nil {
			return err
		}
	}
	s.setOSIngestCache(enabled)
	return nil
}

func (s *MetricsService) setOSIngestCache(enabled bool) {
	if s == nil {
		return
	}
	s.osIngestMu.Lock()
	s.osIngestEnabled = enabled
	s.osIngestLoaded = true
	s.osIngestMu.Unlock()
}

func (s *MetricsService) cachedOSIngestEnabled() bool {
	if s == nil {
		return false
	}
	s.osIngestMu.RLock()
	defer s.osIngestMu.RUnlock()
	return s.osIngestEnabled
}

// OSMetricsIngestInfo returns current ingest policy for APIs and UI.
func (s *MetricsService) OSMetricsIngestInfo(ctx context.Context) OSMetricsIngestInfo {
	env := osMetricsIngestEnv()
	switch env {
	case "1":
		return OSMetricsIngestInfo{Enabled: true, Source: "env", Configurable: false, EnvValue: "1"}
	case "0":
		return OSMetricsIngestInfo{Enabled: false, Source: "env", Configurable: false, EnvValue: "0"}
	}
	if s != nil && !s.osIngestLoaded {
		_ = s.ReloadOSMetricsIngest(ctx)
	}
	return OSMetricsIngestInfo{
		Enabled:      s != nil && s.cachedOSIngestEnabled(),
		Source:       "database",
		Configurable: true,
	}
}

// IsOSMetricsIngestEnabled reports whether POST /api/os/metrics should accept payloads.
func (s *MetricsService) IsOSMetricsIngestEnabled(ctx context.Context) bool {
	return s.OSMetricsIngestInfo(ctx).Enabled
}

// SetOSMetricsIngestEnabled persists the toggle (database) and updates the in-memory cache.
func (s *MetricsService) SetOSMetricsIngestEnabled(ctx context.Context, enabled bool, updatedBy string) error {
	if s == nil {
		return nil
	}
	env := osMetricsIngestEnv()
	if env == "0" || env == "1" {
		return ErrOSMetricsIngestEnvLocked
	}
	if s.platformSettings == nil {
		return errors.New("platform settings store not available")
	}
	if err := s.platformSettings.SetBool(ctx, repository.PlatformSettingOSMetricsIngest, enabled, updatedBy); err != nil {
		return err
	}
	s.setOSIngestCache(enabled)
	return nil
}

func osMetricsIngestEnv() string {
	return strings.TrimSpace(os.Getenv("OS_METRICS_INGEST_ENABLED"))
}
