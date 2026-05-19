// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: TimescaleDB setup, connection persistence, and initialization logic.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package hot

import (
	"log/slog"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/security"
)

type persistedTimescale struct {
	Host     string `json:"host"`
	Port     string `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	Database string `json:"database"`
	SSLMode  string `json:"ssl_mode"`
}

func derivePersistKey(secret []byte) []byte {
	sum := sha256.Sum256(secret)
	out := append([]byte(nil), sum[:]...)
	return out
}

// LoadPersistedTimescaleConfig reads and decrypts the persisted Timescale connection.
func LoadPersistedTimescaleConfig(configPath string, secretMaterial []byte) (*Config, error) {
	p := config.TimescalePersistPath(configPath)
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

	maxConnsStr := os.Getenv("TIMESCALEDB_MAX_CONNS")
	if maxConnsStr == "" {
		maxConnsStr = "25"
	}
	maxConns, err := strconv.Atoi(maxConnsStr)
	if err != nil {
		maxConns = 25
	}

	return &Config{
		Host:     strings.TrimSpace(ptc.Host),
		Port:     strings.TrimSpace(ptc.Port),
		User:     strings.TrimSpace(ptc.User),
		Password: ptc.Password,
		Database: strings.TrimSpace(ptc.Database),
		SSLMode:  strings.TrimSpace(ptc.SSLMode),
		MaxConns: int32(maxConns),
	}, nil
}

// SavePersistedTimescaleConfig encrypts and writes Timescale connection details.
func SavePersistedTimescaleConfig(configPath string, secretMaterial []byte, c *Config) error {
	if c == nil {
		return errors.New("config is nil")
	}
	p := config.TimescalePersistPath(configPath)
	dir := filepath.Dir(p)
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
	if err != nil {
		return err
	}
	return os.WriteFile(p, enc, 0600)
}

// ConnectMetricsTimescale connects to the TimescaleDB metrics repository.
func ConnectMetricsTimescale(configPath string, jwtSecret []byte) (ts *HotStorage, usingEnvTimescale bool, err error) {
	dockerMode := config.DeploymentIsDocker()
	envOnly := dockerMode || strings.TrimSpace(os.Getenv("TIMESCALE_USE_ENV_ONLY")) == "1"

	persistedTS, perr := LoadPersistedTimescaleConfig(configPath, jwtSecret)
	if perr != nil {
		slog.Info("[timescale] persisted config read", "err", perr)
	}
	if persistedTS != nil {
		var e error
		ts, e = New(persistedTS)
		if e != nil {
			slog.Error("[WARNING] persisted Timescale connection failed", "err", e)
			ts = nil
		} else {
			slog.Info("[Info] Connected to TimescaleDB using UI-persisted credentials")
		}
	}

	if ts == nil && envOnly {
		ts, err = New(nil)
		if err != nil {
			return nil, false, fmt.Errorf("timescale env connection: %w", err)
		}
		slog.Info("[Info] Connected to TimescaleDB using environment variables")
		return ts, true, nil
	}

	return ts, false, nil
}
