// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Shared KMS bootstrap for decrypting server registry credentials (API, worker).
package config

import (
	"log"
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
		vTok := strings.TrimSpace(os.Getenv("VAULT_TOKEN"))
		vKey := strings.TrimSpace(os.Getenv("VAULT_TRANSIT_KEY"))
		vNs := strings.TrimSpace(os.Getenv("VAULT_NAMESPACE"))
		vMount := strings.TrimSpace(os.Getenv("VAULT_TRANSIT_MOUNT"))
		k, kerr := security.InitVaultClient(security.VaultConfig{Addr: vaultAddr, Token: vTok, Namespace: vNs, TransitMount: vMount, TransitKey: vKey})
		if kerr == nil {
			log.Printf("[vault] KMS enabled (Transit)")
			return k, false
		}
		log.Printf("[vault] KMS init failed: %v", kerr)
	}
	if lk, kerr := security.NewLocalEnvelopeKMS(jwtSecret); kerr == nil {
		return lk, true
	} else {
		log.Printf("[kms] local envelope KMS unavailable: %v", kerr)
	}
	return nil, false
}
