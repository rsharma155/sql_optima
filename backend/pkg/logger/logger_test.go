package logger

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestInit_JSONOutput(t *testing.T) {
	Reset()
	var buf bytes.Buffer
	l := Init(&buf)
	l.Info("test message", "key", "value")

	var m map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("expected JSON output, got: %s", buf.String())
	}
	if m["msg"] != "test message" {
		t.Fatalf("unexpected msg: %v", m["msg"])
	}
	if m["key"] != "value" {
		t.Fatalf("unexpected key: %v", m["key"])
	}
}

func TestInit_RedactsPostgresURL(t *testing.T) {
	Reset()
	var buf bytes.Buffer
	l := Init(&buf)
	l.Error("connection failed", "dsn", "postgres://admin:s3cret@db:5432/app")

	out := buf.String()
	if strings.Contains(out, "s3cret") {
		t.Fatalf("credential leaked in log output: %s", out)
	}
	if !strings.Contains(out, "REDACTED") {
		t.Fatalf("expected REDACTED in log output: %s", out)
	}
}

func TestInit_RedactsPasswordKV(t *testing.T) {
	Reset()
	var buf bytes.Buffer
	l := Init(&buf)
	l.Warn("config loaded", "detail", "password=hunter2;host=db")

	out := buf.String()
	if strings.Contains(out, "hunter2") {
		t.Fatalf("password leaked: %s", out)
	}
	if !strings.Contains(out, "REDACTED") {
		t.Fatalf("expected REDACTED: %s", out)
	}
}

func TestInit_RedactsMessageBody(t *testing.T) {
	Reset()
	var buf bytes.Buffer
	l := Init(&buf)
	l.Info("postgres://user:pass@host/db error occurred")

	out := buf.String()
	if strings.Contains(out, "user:pass") {
		t.Fatalf("credential leaked in message: %s", out)
	}
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input string
		want  slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"DEBUG", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{"error", slog.LevelError},
		{"", slog.LevelInfo},
		{"unknown", slog.LevelInfo},
	}
	for _, tt := range tests {
		got := parseLevel(tt.input)
		if got != tt.want {
			t.Errorf("parseLevel(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestLevelFiltering(t *testing.T) {
	Reset()
	t.Setenv("LOG_LEVEL", "error")
	var buf bytes.Buffer
	l := Init(&buf)
	l.Info("should be filtered")
	if buf.Len() != 0 {
		t.Fatalf("info log should be filtered at error level, got: %s", buf.String())
	}
	l.Error("should appear")
	if buf.Len() == 0 {
		t.Fatal("error log should appear at error level")
	}
}

func TestWith_ChildLogger(t *testing.T) {
	Reset()
	var buf bytes.Buffer
	Init(&buf)
	child := With("component", "pgss")
	child.Info("test")

	var m map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("expected JSON: %s", buf.String())
	}
	if m["component"] != "pgss" {
		t.Fatalf("expected component=pgss, got %v", m["component"])
	}
}

func TestGet_LazyInit(t *testing.T) {
	Reset()
	l := Get()
	if l == nil {
		t.Fatal("Get() should never return nil")
	}
}

func TestRedactAttr_GroupValues(t *testing.T) {
	Reset()
	var buf bytes.Buffer
	l := Init(&buf)
	l.Info("grouped",
		slog.Group("conn",
			slog.String("dsn", "postgres://u:p@h/d"),
			slog.String("host", "safe-host"),
		),
	)
	out := buf.String()
	if strings.Contains(out, "u:p@h") {
		t.Fatalf("credential leaked in group: %s", out)
	}
	if !strings.Contains(out, "safe-host") {
		t.Fatalf("safe value should remain: %s", out)
	}
}
