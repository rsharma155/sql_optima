// SPDX-License-Identifier: MIT
package repository

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const PlatformSettingOSMetricsIngest = "os_metrics_ingest_enabled"

// PlatformSettingsRepository stores app-wide toggles in TimescaleDB.
type PlatformSettingsRepository struct {
	pool *pgxpool.Pool
}

func NewPlatformSettingsRepository(pool *pgxpool.Pool) *PlatformSettingsRepository {
	return &PlatformSettingsRepository{pool: pool}
}

func (r *PlatformSettingsRepository) GetBool(ctx context.Context, key string) (bool, error) {
	if r == nil || r.pool == nil {
		return false, nil
	}
	var raw string
	err := r.pool.QueryRow(ctx, `
		SELECT setting_value FROM optima_platform_settings WHERE setting_key = $1`, key).Scan(&raw)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || isMissingPlatformSettings(err) {
			return false, nil
		}
		return false, err
	}
	return parseBoolSetting(raw), nil
}

func (r *PlatformSettingsRepository) SetBool(ctx context.Context, key string, enabled bool, updatedBy string) error {
	if r == nil || r.pool == nil {
		return nil
	}
	val := "false"
	if enabled {
		val = "true"
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO optima_platform_settings (setting_key, setting_value, updated_at, updated_by)
		VALUES ($1, $2, now(), $3)
		ON CONFLICT (setting_key) DO UPDATE SET
			setting_value = EXCLUDED.setting_value,
			updated_at = now(),
			updated_by = EXCLUDED.updated_by`,
		key, val, updatedBy)
	return err
}

func parseBoolSetting(raw string) bool {
	raw = strings.TrimSpace(strings.ToLower(raw))
	switch raw {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func isMissingPlatformSettings(err error) bool {
	if err == nil {
		return false
	}
	if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == "42P01" {
		return true
	}
	return strings.Contains(err.Error(), "does not exist")
}

// ParseBoolSetting exports bool parsing for tests.
func ParseBoolSetting(raw string) bool { return parseBoolSetting(raw) }

// FormatBoolSetting exports canonical storage format.
func FormatBoolSetting(enabled bool) string {
	return strconv.FormatBool(enabled)
}
