// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Build config.Instance slices from optima_servers + envelope secrets (API, worker).
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package repository

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/domain/servers"
)

// LoadInstancesFromServerRegistry returns active monitoring targets when Timescale, KMS,
// and decryptable credentials are available. Empty slice means no usable servers (not an error).
func LoadInstancesFromServerRegistry(ctx context.Context, pool *pgxpool.Pool, kms servers.KeyManagementService, box servers.SecretBox) ([]config.Instance, error) {
	if pool == nil || kms == nil || box == nil {
		return nil, nil
	}
	repo := NewServerRegistryRepository(pool)
	active, err := repo.List(ctx, true)
	if err != nil {
		return nil, err
	}
	if len(active) == 0 {
		return []config.Instance{}, nil
	}

	out := make([]config.Instance, 0, len(active))
	for _, s := range active {
		s2, encSecret, encDEK, err := repo.GetEncrypted(ctx, s.ID)
		if err != nil {
			slog.Warn("[registry] failed to fetch encrypted credentials", "server", s.Name, "err", err)
			continue
		}
		dek, err := kms.DecryptDataKey(ctx, encDEK)
		if err != nil {
			slog.Warn("[registry] DEK decryption failed — KMS key mismatch or Vault unavailable", "server", s.Name, "err", err)
			continue
		}
		plainJSON, err := box.Decrypt(encSecret, dek)
		for i := range dek {
			dek[i] = 0
		}
		if err != nil {
			slog.Warn("[registry] credential decryption failed", "server", s.Name, "err", err)
			continue
		}
		var cred servers.CredentialPayload
		if err := json.Unmarshal(plainJSON, &cred); err != nil {
			for i := range plainJSON {
				plainJSON[i] = 0
			}
			slog.Warn("[registry] credential JSON unmarshal failed", "server", s.Name, "err", err)
			continue
		}
		for i := range plainJSON {
			plainJSON[i] = 0
		}
		if strings.TrimSpace(cred.Password) == "" {
			slog.Warn("[registry] skipping server with empty password", "server", s.Name)
			continue
		}

		database := strings.TrimSpace(cred.Database)
		if database == "" {
			switch strings.ToLower(string(s2.DBType)) {
			case "postgres":
				database = "postgres"
			case "sqlserver":
				database = "master"
			}
		}

		inst := config.Instance{
			ServerID:               s2.ID,
			Name:                   s2.Name,
			Type:                   string(s2.DBType),
			Host:                   s2.Host,
			Port:                   s2.Port,
			User:                   s2.Username,
			MonitoringUser:         s2.Username,
			Password:               cred.Password,
			SSLMode:                strings.TrimSpace(string(s2.SSLMode)),
			Database:               database,
			TrustServerCertificate: cred.TrustServerCertificate,
		}
		out = append(out, inst)
	}
	return out, nil
}
