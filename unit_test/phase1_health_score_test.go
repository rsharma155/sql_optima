package unit_test

import (
	"encoding/json"
	"testing"

	"github.com/rsharma155/sql_optima/pkg/dashboard"
)

func TestComputeHealthScore_BaseHealthy(t *testing.T) {
	got := dashboard.ComputeHealthScore(dashboard.HealthInputs{})
	if got.Score != 100 {
		t.Fatalf("score=%d want=100", got.Score)
	}
	if got.Severity != dashboard.SeverityOK {
		t.Fatalf("severity=%s want=%s", got.Severity, dashboard.SeverityOK)
	}
	if got.Label != "Healthy" {
		t.Fatalf("label=%q want=%q", got.Label, "Healthy")
	}
}

func TestComputeHealthScore_DeductionsAndClamps(t *testing.T) {
	blocking := 2
	memGrants := 1
	failedLogins := 10
	ple := 100.0
	logPct := 95.0
	tempPct := 99.0
	writeLog := true

	got := dashboard.ComputeHealthScore(dashboard.HealthInputs{
		BlockingSessions:           &blocking,     // -15
		MemoryGrantsPending:        &memGrants,    // -15
		FailedLoginsLast5Min:       &failedLogins, // -10
		PLE:                       &ple,          // -10
		MaxLogUsedPercent:         &logPct,        // -20
		TempdbUsedPercent:         &tempPct,       // -20
		DominantWaitIsWriteLog:    &writeLog,      // -10
	})

	// Total deduction = 100; score should clamp at 0.
	if got.Score != 0 {
		t.Fatalf("score=%d want=0", got.Score)
	}
	if got.Severity != dashboard.SeverityCritical {
		t.Fatalf("severity=%s want=%s", got.Severity, dashboard.SeverityCritical)
	}
	if got.Label != "Critical" {
		t.Fatalf("label=%q want=%q", got.Label, "Critical")
	}
}

func TestHomepageV2_JSONShape(t *testing.T) {
	h := dashboard.HomepageV2{
		InstanceName: "SQL-1",
		Timestamp:    "2026-04-08T00:00:00Z",
		HealthRisk: map[string]any{
			"health_score": dashboard.HealthScore{Score: 90, Severity: dashboard.SeverityOK, Label: "Healthy"},
		},
		WorkloadCapacity: map[string]any{"avg_cpu_load": 12.3},
		RootCause:        map[string]any{},
		MemoryStorage:    map[string]any{},
		LiveDiagnostics:  map[string]any{},
	}

	b, err := json.Marshal(h)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	// Basic shape checks (keys must exist).
	for _, k := range []string{
		"instance_name",
		"timestamp",
		"generated_at",
		"health_risk",
		"workload_capacity",
		"root_cause",
		"memory_storage_internals",
		"live_diagnostics",
	} {
		if _, ok := m[k]; !ok {
			t.Fatalf("missing key %q in JSON payload", k)
		}
	}

	// Ensure compat is optional but if present, it should be an object.
	if v, ok := m["compat"]; ok && v != nil {
		if _, ok := v.(map[string]any); !ok {
			t.Fatalf("compat is not an object: %T", v)
		}
	}
}

