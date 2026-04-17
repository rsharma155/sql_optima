package config

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rsharma155/sql_optima/internal/security"
	"github.com/rsharma155/sql_optima/internal/storage/hot"
)

const timescalePersistFile = "timescale_connection.enc.json"

type persistedTimescale struct {
	Host     string `json:"host"`
	Port     string `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	Database string `json:"database"`
	SSLMode  string `json:"ssl_mode"`
}

// TimescalePersistPath returns the path to the encrypted Timescale connection file.
func TimescalePersistPath(configPath string) string {
	base := filepath.Clean(filepath.Join(filepath.Dir(configPath), "data"))
	return filepath.Join(base, timescalePersistFile)
}

func derivePersistKey(secret []byte) []byte {
	sum := sha256.Sum256(secret)
	out := append([]byte(nil), sum[:]...)
	return out
}

// LoadPersistedTimescaleConfig reads and decrypts the persisted Timescale connection.
// Returns (nil, nil) when the file does not exist.
func LoadPersistedTimescaleConfig(configPath string, secretMaterial []byte) (*hot.Config, error) {
	p := TimescalePersistPath(configPath)
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil, nil
	}
	key := derivePersistKey(secretMaterial)
	box := security.NewEnvelopeSecretBox()
	plain, err := box.Decrypt(data, key)
	if err != nil {
		return nil, fmt.Errorf("decrypt timescale config: %w", err)
	}
	var ptc persistedTimescale
	if err := json.Unmarshal(plain, &ptc); err != nil {
		return nil, fmt.Errorf("parse timescale config: %w", err)
	}
	for i := range plain {
		plain[i] = 0
	}
	if strings.TrimSpace(ptc.Host) == "" || strings.TrimSpace(ptc.Port) == "" {
		return nil, errors.New("persisted timescale config incomplete")
	}
	return &hot.Config{
		Host:     strings.TrimSpace(ptc.Host),
		Port:     strings.TrimSpace(ptc.Port),
		User:     strings.TrimSpace(ptc.User),
		Password: ptc.Password,
		Database: strings.TrimSpace(ptc.Database),
		SSLMode:  strings.TrimSpace(ptc.SSLMode),
		MaxConns: 50,
	}, nil
}

// SavePersistedTimescaleConfig encrypts and writes Timescale connection details.
func SavePersistedTimescaleConfig(configPath string, secretMaterial []byte, c *hot.Config) error {
	if c == nil {
		return errors.New("config is nil")
	}
	dir := filepath.Dir(TimescalePersistPath(configPath))
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	ptc := persistedTimescale{
		Host:     c.Host,
		Port:     c.Port,
		User:     c.User,
		Password: c.Password,
		Database: c.Database,
		SSLMode:  c.SSLMode,
	}
	plain, err := json.Marshal(ptc)
	if err != nil {
		return err
	}
	key := derivePersistKey(secretMaterial)
	box := security.NewEnvelopeSecretBox()
	enc, err := box.Encrypt(plain, key)
	for i := range plain {
		plain[i] = 0
	}
	if err != nil {
		return err
	}
	tmp := TimescalePersistPath(configPath) + ".tmp"
	if err := os.WriteFile(tmp, enc, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, TimescalePersistPath(configPath))
}
