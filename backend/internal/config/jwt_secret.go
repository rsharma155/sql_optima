package config

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const jwtSecretPersistFile = "jwt_secret"

func jwtSecretPath(configPath string) string {
	base := filepath.Clean(filepath.Join(filepath.Dir(configPath), "data"))
	return filepath.Join(base, jwtSecretPersistFile)
}

// ResolveJWTSecret returns a valid JWT secret.
// Priority:
// 1) JWT_SECRET env var (must be >= 32 chars)
// 2) persisted data/jwt_secret (auto-created if missing)
func ResolveJWTSecret(configPath string) ([]byte, error) {
	if env := strings.TrimSpace(os.Getenv("JWT_SECRET")); env != "" {
		if len(env) < 32 {
			return nil, fmt.Errorf("JWT_SECRET must be at least 32 characters when provided")
		}
		return []byte(env), nil
	}

	p := jwtSecretPath(configPath)
	if b, err := os.ReadFile(p); err == nil {
		s := strings.TrimSpace(string(b))
		if len(s) < 32 {
			return nil, fmt.Errorf("persisted JWT secret at %s is too short; delete it and restart to regenerate", p)
		}
		return []byte(s), nil
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return nil, err
	}

	rnd := make([]byte, 48)
	if _, err := rand.Read(rnd); err != nil {
		return nil, err
	}
	secret := base64.RawStdEncoding.EncodeToString(rnd)

	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, []byte(secret), 0600); err != nil {
		return nil, err
	}
	if err := os.Rename(tmp, p); err != nil {
		return nil, err
	}

	return []byte(secret), nil
}
