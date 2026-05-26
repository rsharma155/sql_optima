// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Shared KMS bootstrap for decrypting server registry credentials (API, worker).
package config

import (
	"log/slog"
	"os"
	"strings"

	"github.com/rsharma155/sql_optima/internal/domain/servers"
	"github.com/rsharma155/sql_optima/internal/security"
)

// InitServerRegistryKMS selects Vault Transit when VAULT_ADDR is set, otherwise a local
// envelope KMS derived from jwtSecret. Returns (nil, false) when neither is usable.
func InitServerRegistryKMS(jwtSecret []byte) (kms servers.KeyManagementService, usingLocalKMS bool) {
	vaultAddr := strings.TrimSpace(os.Getenv("VAULT_ADDR"))
	if vaultAddr != "" {
		vTok := security.ResolveVaultToken()
		vKey := strings.TrimSpace(os.Getenv("VAULT_TRANSIT_KEY"))
		vNs := strings.TrimSpace(os.Getenv("VAULT_NAMESPACE"))
		vMount := strings.TrimSpace(os.Getenv("VAULT_TRANSIT_MOUNT"))
		k, kerr := security.InitVaultClient(security.VaultConfig{Addr: vaultAddr, Token: vTok, Namespace: vNs, TransitMount: vMount, TransitKey: vKey})
		if kerr == nil {
			slog.Info("[vault] KMS enabled (Transit)")
			return k, false
		}
		slog.Error("[vault] KMS init failed", "err", kerr)
	}
	if lk, kerr := security.NewLocalEnvelopeKMS(jwtSecret); kerr == nil {
		return lk, true
	} else {
		slog.Info("[kms] local envelope KMS unavailable", "err", kerr)
	}
	return nil, false
}
