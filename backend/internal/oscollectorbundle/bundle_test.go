package oscollectorbundle

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestBuildZip(t *testing.T) {
	data, err := BuildZip(Params{
		InstanceName: "my-pg-prod",
		ServerID:     "550e8400-e29b-41d4-a716-446655440000",
		AppURL:       "https://monitor.example.com",
		MetricsURL:   "https://monitor.example.com/api/os/metrics",
		GeneratedAt:  time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	names := make(map[string]bool)
	for _, f := range zr.File {
		names[f.Name] = true
	}
	for _, want := range []string{
		"sql-optima-os-collector.sh",
		"quick-install.sh",
		"bundled-config.env",
		"systemd/sql-optima-os-collector.service",
		"INSTALL.txt",
		"README.txt",
	} {
		if !names[want] {
			t.Fatalf("missing zip entry %q, got %v", want, names)
		}
	}
	var envBuf bytes.Buffer
	var scriptBuf bytes.Buffer
	for _, f := range zr.File {
		switch f.Name {
		case "bundled-config.env":
			rc, _ := f.Open()
			_, _ = envBuf.ReadFrom(rc)
			_ = rc.Close()
		case "sql-optima-os-collector.sh":
			rc, _ := f.Open()
			_, _ = scriptBuf.ReadFrom(rc)
			_ = rc.Close()
		}
	}
	env := envBuf.String()
	if !strings.Contains(env, "SQL_OPTIMA_INSTANCE_NAME='my-pg-prod'") {
		t.Fatalf("env missing instance: %s", env)
	}
	if !strings.Contains(env, "550e8400-e29b-41d4-a716-446655440000") {
		t.Fatalf("env missing server_id: %s", env)
	}
	if !strings.Contains(scriptBuf.String(), "https://monitor.example.com/api/os/metrics") {
		t.Fatal("script missing baked metrics URL")
	}
}

func TestBuildZipInstanceNameWithSpaces(t *testing.T) {
	data, err := BuildZip(Params{
		InstanceName: "Postgres Local",
		ServerID:     "ee40f6f9-74aa-45c8-bc2e-b92f592a7bb2",
		AppURL:       "http://192.168.1.75:8080",
		MetricsURL:   "http://192.168.1.75:8080/api/os/metrics",
	})
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range zr.File {
		if f.Name != "bundled-config.env" {
			continue
		}
		rc, _ := f.Open()
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(rc)
		_ = rc.Close()
		env := buf.String()
		if !strings.Contains(env, "SQL_OPTIMA_INSTANCE_NAME='Postgres Local'") {
			t.Fatalf("expected quoted instance name, got:\n%s", env)
		}
		return
	}
	t.Fatal("bundled-config.env missing")
}

func TestShellQuote(t *testing.T) {
	if shellQuote(`postgres local`) != `'postgres local'` {
		t.Fatalf("got %s", shellQuote(`postgres local`))
	}
	if shellQuote(`it's`) != `'it'"'"'s'` {
		t.Fatalf("got %s", shellQuote(`it's`))
	}
}

func TestSanitizeFilename(t *testing.T) {
	if SanitizeFilename("prod/db#1") != "prod_db_1" {
		t.Fatalf("got %q", SanitizeFilename("prod/db#1"))
	}
}
