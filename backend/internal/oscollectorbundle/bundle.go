// Package oscollectorbundle builds downloadable OS collector deployment zips.
//
// SPDX-License-Identifier: MIT
package oscollectorbundle

import (
	"archive/zip"
	"bytes"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strings"
	"time"
)

//go:embed assets/sql-optima-os-collector.sh assets/systemd/sql-optima-os-collector.service
var embedded embed.FS

// Params customizes the deployment bundle for a monitored instance.
type Params struct {
	InstanceName string
	ServerID     string
	AppURL       string // SQL Optima UI origin (no trailing slash)
	MetricsURL   string // POST /api/os/metrics
	GeneratedAt  time.Time
}

// BuildZip returns a zip archive with pre-configured agent, install helper, and docs.
func BuildZip(p Params) ([]byte, error) {
	if strings.TrimSpace(p.InstanceName) == "" {
		return nil, fmt.Errorf("instance_name is required")
	}
	metricsURL := strings.TrimSpace(p.MetricsURL)
	if metricsURL == "" {
		metricsURL = "https://YOUR_SQL_OPTIMA_HOST/api/os/metrics"
	}
	appURL := strings.TrimSpace(p.AppURL)
	if appURL == "" {
		appURL = strings.TrimSuffix(metricsURL, "/api/os/metrics")
	}
	serverID := strings.TrimSpace(p.ServerID)
	if serverID == "" {
		serverID = "unknown"
	}

	scriptRaw := mustReadEmbed("assets/sql-optima-os-collector.sh")
	script := personalizeScript(scriptRaw, p)

	bundledEnv := fmt.Sprintf(`# SQL Optima OS Collector — pre-filled from UI download
# Run: chmod +x sql-optima-os-collector.sh && ./sql-optima-os-collector.sh install
# Values are shell-quoted so instance names may contain spaces.

SQL_OPTIMA_BACKEND_URL=%s
SQL_OPTIMA_APP_URL=%s
SQL_OPTIMA_INSTANCE_NAME=%s
SQL_OPTIMA_SERVER_ID=%s
SQL_OPTIMA_API_KEY=%s
SQL_OPTIMA_CRON_INTERVAL_MIN=5
`, shellQuote(metricsURL), shellQuote(appURL), shellQuote(p.InstanceName), shellQuote(serverID), shellQuote("PASTE_ADMIN_JWT_HERE"))

	quickInstall := fmt.Sprintf(`#!/usr/bin/env bash
# Quick setup — run on the PostgreSQL Linux host after unzip
set -euo pipefail
cd "$(dirname "$0")"
chmod +x sql-optima-os-collector.sh
echo "SQL Optima OS Collector — instance: %s"
echo "Server ID: %s"
echo "App URL:   %s"
echo ""
exec ./sql-optima-os-collector.sh install "$@"
`, p.InstanceName, serverID, appURL)

	installContent := buildInstallGuide(p)
	readmeContent := buildReadme(p)

	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)

	files := []struct {
		name    string
		content []byte
		mode    int
	}{
		{"sql-optima-os-collector.sh", script, 0755},
		{"quick-install.sh", []byte(quickInstall), 0755},
		{"bundled-config.env", []byte(bundledEnv), 0644},
		{"systemd/sql-optima-os-collector.service", mustReadEmbed("assets/systemd/sql-optima-os-collector.service"), 0644},
		{"INSTALL.txt", []byte(installContent), 0644},
		{"README.txt", []byte(readmeContent), 0644},
	}

	for _, f := range files {
		if err := writeZipFile(zw, f.name, f.content, f.mode); err != nil {
			_ = zw.Close()
			return nil, err
		}
	}

	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func personalizeScript(script []byte, p Params) []byte {
	s := string(script)
	repl := map[string]string{
		"__BUNDLE_BACKEND_URL__":  strings.TrimSpace(p.MetricsURL),
		"__BUNDLE_INSTANCE_NAME__": strings.TrimSpace(p.InstanceName),
		"__BUNDLE_SERVER_ID__":    strings.TrimSpace(p.ServerID),
		"__BUNDLE_APP_URL__":      strings.TrimSpace(p.AppURL),
	}
	for old, val := range repl {
		if val == "" {
			continue
		}
		s = strings.ReplaceAll(s, old, val)
	}
	return []byte(s)
}

func mustReadEmbed(name string) []byte {
	b, err := embedded.ReadFile(path.Clean(name))
	if err != nil {
		panic("oscollectorbundle: missing embedded asset " + name + ": " + err.Error())
	}
	return b
}

func writeZipFile(zw *zip.Writer, name string, content []byte, mode int) error {
	h := &zip.FileHeader{
		Name:   name,
		Method: zip.Deflate,
	}
	h.SetMode(fs.FileMode(mode))
	h.Modified = time.Now().UTC()
	w, err := zw.CreateHeader(h)
	if err != nil {
		return err
	}
	_, err = io.Copy(w, bytes.NewReader(content))
	return err
}

func buildInstallGuide(p Params) string {
	gen := p.GeneratedAt.UTC().Format(time.RFC3339)
	if gen == "" {
		gen = time.Now().UTC().Format(time.RFC3339)
	}
	return fmt.Sprintf(`SQL Optima OS Collector — Quick Install
Generated: %s

Pre-configured for this server:
  Instance name: %s
  Server ID:     %s
  App URL:       %s
  Metrics URL:   %s

FASTEST SETUP (on the PostgreSQL Linux host)
  1. Unzip this folder
  2. chmod +x quick-install.sh sql-optima-os-collector.sh
  3. ./quick-install.sh
     (prompts for admin JWT once, installs a cron job every 5 minutes)

Alternative — systemd (30s loop, requires root):
  sudo ./sql-optima-os-collector.sh install --systemd

Manual test (one push):
  source bundled-config.env   # add your JWT first
  ./sql-optima-os-collector.sh --once

MONITORING SERVER (one-time)
  OS_METRICS_INGEST_ENABLED=1 on the SQL Optima API, then restart.

VERIFY
  SQL Optima → PostgreSQL → Memory shows "OS Collector Active"
`, gen, p.InstanceName, p.ServerID, p.AppURL, p.MetricsURL)
}

func buildReadme(p Params) string {
	return fmt.Sprintf(`SQL Optima OS Collector
Instance: %s | Server ID: %s

Quick start: ./quick-install.sh

Scheduling:
  - Default install uses cron (every 5 min). Cron cannot run every 30s.
  - For 30s pushes use: sudo ./sql-optima-os-collector.sh install --systemd
  - Foreground loop: ./sql-optima-os-collector.sh -interval 30s

Config file: bundled-config.env (edit SQL_OPTIMA_API_KEY before install)
`, p.InstanceName, p.ServerID)
}

// shellQuote wraps s in single quotes for safe use in bash env files (source/set -a).
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

// SanitizeFilename returns a safe zip download filename segment.
func SanitizeFilename(instance string) string {
	s := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return '_'
		}
	}, strings.TrimSpace(instance))
	if s == "" {
		s = "instance"
	}
	return s
}
