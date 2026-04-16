package security

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	vault "github.com/hashicorp/vault/api"
)

// VaultKMS implements envelope-key operations using Vault Transit.
// It supports token auth (VAULT_TOKEN) for bootstrap; production should prefer AppRole.
type VaultKMS struct {
	client       *vault.Client
	transitMount string
	transitKey   string
}

type VaultConfig struct {
	Addr         string
	Token        string
	Namespace    string
	TransitMount string // secrets engine mount name, default "transit" (env VAULT_TRANSIT_MOUNT)
	TransitKey   string // encryption key name inside that engine (env VAULT_TRANSIT_KEY)
}

// NormalizeVaultTransitMount returns a single-segment Vault mount name (no slashes).
// Default is "transit". Values with unsafe characters fall back to "transit".
func NormalizeVaultTransitMount(s string) string {
	m := strings.TrimSpace(s)
	m = strings.Trim(m, "/")
	if m == "" {
		return "transit"
	}
	if strings.Contains(m, "..") || strings.Contains(m, "/") {
		return "transit"
	}
	for _, r := range m {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			continue
		}
		return "transit"
	}
	return m
}

func InitVaultClient(cfg VaultConfig) (*VaultKMS, error) {
	addr := strings.TrimSpace(cfg.Addr)
	if addr == "" {
		return nil, errors.New("VAULT_ADDR is required")
	}
	c, err := vault.NewClient(&vault.Config{Address: addr})
	if err != nil {
		return nil, err
	}
	if ns := strings.TrimSpace(cfg.Namespace); ns != "" {
		c.SetNamespace(ns)
	}
	if tok := strings.TrimSpace(cfg.Token); tok != "" {
		c.SetToken(tok)
	}
	mount := NormalizeVaultTransitMount(cfg.TransitMount)
	key := strings.TrimSpace(cfg.TransitKey)
	if key == "" {
		key = "sql-optima"
	}
	return &VaultKMS{client: c, transitMount: mount, transitKey: key}, nil
}

func (v *VaultKMS) transitPath(suffix string) string {
	m := "transit"
	if v != nil && v.transitMount != "" {
		m = v.transitMount
	}
	return m + "/" + strings.TrimPrefix(suffix, "/")
}

// GenerateDataKey calls transit/datakey/plaintext/<key> and returns plaintext (decoded) and ciphertext (raw bytes).
func (v *VaultKMS) GenerateDataKey(ctx context.Context) ([]byte, []byte, error) {
	if v == nil || v.client == nil {
		return nil, nil, errors.New("vault not configured")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	path := v.transitPath("datakey/plaintext/") + url.PathEscape(v.transitKey)
	secret, err := v.client.Logical().WriteWithContext(ctx, path, map[string]any{
		"bits": 256,
	})
	if err != nil {
		return nil, nil, err
	}
	if secret == nil || secret.Data == nil {
		return nil, nil, errors.New("vault returned empty response")
	}

	plainB64, _ := secret.Data["plaintext"].(string)
	ciphertext, _ := secret.Data["ciphertext"].(string)
	if plainB64 == "" || ciphertext == "" {
		return nil, nil, errors.New("vault response missing plaintext/ciphertext")
	}
	plain, err := base64.StdEncoding.DecodeString(plainB64)
	if err != nil {
		return nil, nil, fmt.Errorf("decode plaintext: %w", err)
	}
	// Store ciphertext as bytes of the vault string (e.g., vault:v1:...).
	return plain, []byte(ciphertext), nil
}

func (v *VaultKMS) DecryptDataKey(ctx context.Context, encryptedDEK []byte) ([]byte, error) {
	if v == nil || v.client == nil {
		return nil, errors.New("vault not configured")
	}
	ct := strings.TrimSpace(string(encryptedDEK))
	if ct == "" {
		return nil, errors.New("encrypted dek is empty")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	path := v.transitPath("decrypt/") + url.PathEscape(v.transitKey)
	secret, err := v.client.Logical().WriteWithContext(ctx, path, map[string]any{
		"ciphertext": ct,
	})
	if err != nil {
		return nil, err
	}
	if secret == nil || secret.Data == nil {
		return nil, errors.New("vault returned empty response")
	}
	plainB64, _ := secret.Data["plaintext"].(string)
	if plainB64 == "" {
		return nil, errors.New("vault response missing plaintext")
	}
	plain, err := base64.StdEncoding.DecodeString(plainB64)
	if err != nil {
		return nil, fmt.Errorf("decode plaintext: %w", err)
	}
	return plain, nil
}
