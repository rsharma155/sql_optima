// SQL Optima — https://github.com/rsharma155/sql_optima
//
// File: backend/internal/storage/cold/config_test.go
// Purpose: Unit tests for the cold storage configuration module.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package cold

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestConfigFromEnv(t *testing.T) {
	// Set test environment variables
	os.Setenv("COLD_STORAGE_ENABLED", "true")
	os.Setenv("COLD_STORAGE_ENDPOINT", "http://localhost:9000")
	os.Setenv("COLD_STORAGE_BUCKET", "test-bucket")
	os.Setenv("COLD_STORAGE_REGION", "us-west-1")
	os.Setenv("COLD_STORAGE_ACCESS_KEY_ID", "test-key")
	os.Setenv("COLD_STORAGE_SECRET_ACCESS_KEY", "test-secret")
	os.Setenv("COLD_STORAGE_FORCE_PATH_STYLE", "true")
	os.Setenv("COLD_STORAGE_PREFIX", "test-prefix/")
	os.Setenv("COLD_STORAGE_LAG_DAYS", "3")
	os.Setenv("COLD_STORAGE_BATCH_SIZE", "1000")
	os.Setenv("COLD_STORAGE_STAGING_DIR", "/tmp/test-staging")

	defer func() {
		// Clean up
		os.Unsetenv("COLD_STORAGE_ENABLED")
		os.Unsetenv("COLD_STORAGE_ENDPOINT")
		os.Unsetenv("COLD_STORAGE_BUCKET")
		os.Unsetenv("COLD_STORAGE_REGION")
		os.Unsetenv("COLD_STORAGE_ACCESS_KEY_ID")
		os.Unsetenv("COLD_STORAGE_SECRET_ACCESS_KEY")
		os.Unsetenv("COLD_STORAGE_FORCE_PATH_STYLE")
		os.Unsetenv("COLD_STORAGE_PREFIX")
		os.Unsetenv("COLD_STORAGE_LAG_DAYS")
		os.Unsetenv("COLD_STORAGE_BATCH_SIZE")
		os.Unsetenv("COLD_STORAGE_STAGING_DIR")
	}()

	cfg := ConfigFromEnv()

	assert.True(t, cfg.Enabled)
	assert.Equal(t, "http://localhost:9000", cfg.Endpoint)
	assert.Equal(t, "test-bucket", cfg.Bucket)
	assert.Equal(t, "us-west-1", cfg.Region)
	assert.Equal(t, "test-key", cfg.AccessKeyID)
	assert.Equal(t, "test-secret", cfg.SecretAccessKey)
	assert.True(t, cfg.ForcePathStyle)
	assert.Equal(t, "test-prefix/", cfg.Prefix)
	assert.Equal(t, 3, cfg.ExportLagDays)
	assert.Equal(t, 1000, cfg.ExportBatchSize)
	assert.Equal(t, "/tmp/test-staging", cfg.LocalStagingDir)
}

func TestExportCutoff(t *testing.T) {
	cfg := &Config{
		ExportLagDays: 2,
	}

	cutoff := cfg.ExportCutoff()
	expected := time.Now().UTC().AddDate(0, 0, -2).Truncate(24 * time.Hour)

	// Allow for a small time difference during test execution
	assert.WithinDuration(t, expected, cutoff, 1*time.Second)
}
