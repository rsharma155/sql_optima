package security

import (
	"os"
	"strings"
)

// ResolveVaultToken returns VAULT_TOKEN, or the contents of VAULT_TOKEN_FILE when the env
// token is empty. Docker Compose writes the init root token to vault_data/.root_token.
func ResolveVaultToken() string {
	// Compose default "root" is not the token created by vault operator init.
	if t := strings.TrimSpace(os.Getenv("VAULT_TOKEN")); t != "" && t != "root" {
		return t
	}
	path := strings.TrimSpace(os.Getenv("VAULT_TOKEN_FILE"))
	if path == "" {
		return ""
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
