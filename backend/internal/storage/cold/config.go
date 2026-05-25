// SQL Optima — https://github.com/rsharma155/sql_optima
//
// File: backend/internal/storage/cold/config.go
// Purpose: Environment-based configuration for the cold storage archival pipeline.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package cold

import (
	"os"
	"strconv"
	"time"
)

// Config holds all configuration for the cold storage tier.
type Config struct {
	Enabled            bool
	Endpoint           string
	Bucket             string
	Region             string
	AccessKeyID        string
	SecretAccessKey    string
	ForcePathStyle     bool
	Prefix             string
	CatalogURL         string
	ExportBatchSize    int
	ExportLagDays      int
	PurgeRetentionDays int
	LocalStagingDir    string
}

// ConfigFromEnv loads the cold storage configuration from environment variables.
func ConfigFromEnv() *Config {
	return &Config{
		Enabled:            envBool("COLD_STORAGE_ENABLED", false),
		Endpoint:           envStr("COLD_STORAGE_ENDPOINT", "http://minio:9000"),
		Bucket:             envStr("COLD_STORAGE_BUCKET", "sql-optima-cold"),
		Region:             envStr("COLD_STORAGE_REGION", "us-east-1"),
		AccessKeyID:        envStr("COLD_STORAGE_ACCESS_KEY_ID", ""),
		SecretAccessKey:    envStr("COLD_STORAGE_SECRET_ACCESS_KEY", ""),
		ForcePathStyle:     envBool("COLD_STORAGE_FORCE_PATH_STYLE", true),
		Prefix:             envStr("COLD_STORAGE_PREFIX", "metrics/"),
		CatalogURL:         envStr("COLD_STORAGE_CATALOG_URL", ""),
		ExportBatchSize:    envInt("COLD_STORAGE_BATCH_SIZE", 50000),
		ExportLagDays:      envInt("COLD_STORAGE_LAG_DAYS", 2),
		PurgeRetentionDays: envInt("COLD_STORAGE_PURGE_RETENTION_DAYS", 30),
		LocalStagingDir:    envStr("COLD_STORAGE_STAGING_DIR", "/tmp/sql-optima-cold-staging"),
	}
}

// ExportCutoff returns the upper bound timestamp for the current export run.
// Data newer than this is left in hot storage.
func (c *Config) ExportCutoff() time.Time {
	return time.Now().UTC().AddDate(0, 0, -c.ExportLagDays).Truncate(24 * time.Hour)
}

func envStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envBool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return i
}
