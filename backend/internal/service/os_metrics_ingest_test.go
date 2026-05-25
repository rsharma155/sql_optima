package service

import (
	"context"
	"os"
	"testing"
)

func TestOSMetricsIngestInfo_EnvOverride(t *testing.T) {
	t.Setenv("OS_METRICS_INGEST_ENABLED", "1")
	svc := &MetricsService{}
	info := svc.OSMetricsIngestInfo(context.Background())
	if !info.Enabled || info.Source != "env" || info.Configurable {
		t.Fatalf("got %+v", info)
	}

	t.Setenv("OS_METRICS_INGEST_ENABLED", "0")
	info = svc.OSMetricsIngestInfo(context.Background())
	if info.Enabled || info.Configurable {
		t.Fatalf("got %+v", info)
	}
}

func TestSetOSMetricsIngestEnabled_EnvLocked(t *testing.T) {
	t.Setenv("OS_METRICS_INGEST_ENABLED", "1")
	svc := &MetricsService{platformSettings: nil}
	err := svc.SetOSMetricsIngestEnabled(context.Background(), false, "test")
	if err != ErrOSMetricsIngestEnvLocked {
		t.Fatalf("expected env locked, got %v", err)
	}
}

func TestOSMetricsIngestEnvTrim(t *testing.T) {
	os.Setenv("OS_METRICS_INGEST_ENABLED", " 1 ")
	if osMetricsIngestEnv() != "1" {
		t.Fatalf("trim failed: %q", osMetricsIngestEnv())
	}
	os.Unsetenv("OS_METRICS_INGEST_ENABLED")
}
