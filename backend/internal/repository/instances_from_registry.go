// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Build config.Instance slices from optima_servers + envelope secrets (API, worker).
package repository

import (
	"context"
	"encoding/json"
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
			continue
		}
		dek, err := kms.DecryptDataKey(ctx, encDEK)
		if err != nil {
			continue
		}
		plainJSON, err := box.Decrypt(encSecret, dek)
		for i := range dek {
			dek[i] = 0
		}
		if err != nil {
			continue
		}
		var cred servers.CredentialPayload
		if err := json.Unmarshal(plainJSON, &cred); err != nil {
			for i := range plainJSON {
				plainJSON[i] = 0
			}
			continue
		}
		for i := range plainJSON {
			plainJSON[i] = 0
		}
		if strings.TrimSpace(cred.Password) == "" {
			continue
		}

		inst := config.Instance{
			Name:                   s2.Name,
			Type:                   string(s2.DBType),
			Host:                   s2.Host,
			Port:                   s2.Port,
			User:                   s2.Username,
			Password:               cred.Password,
			SSLMode:                strings.TrimSpace(string(s2.SSLMode)),
			Database:               strings.TrimSpace(cred.Database),
			TrustServerCertificate: cred.TrustServerCertificate,
		}
		out = append(out, inst)
	}
	return out, nil
}
