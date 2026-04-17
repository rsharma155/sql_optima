package handlers

import (
	"strings"
	"testing"

	"github.com/rsharma155/sql_optima/internal/ruleengine/models"
)

// ---------------------------------------------------------------------------
// buildRawRulePayload + buildRuleEvidence
// ---------------------------------------------------------------------------

func TestBuildRawRulePayloadAndEvidence(t *testing.T) {
	payload := buildRawRulePayload([]map[string]interface{}{
		{
			"failing_jobs_24h": 2,
			"sample_jobs":      "Nightly Backups, IndexOptimize",
		},
	}, "2", "0", "WARNING")

	evidence := buildRuleEvidence(payload)
	if !strings.Contains(evidence, "Failing Jobs 24h: 2") {
		t.Fatalf("expected failing jobs evidence, got %q", evidence)
	}
	if !strings.Contains(evidence, "Sample Jobs: Nightly Backups, IndexOptimize") {
		t.Fatalf("expected sample job evidence, got %q", evidence)
	}
}

func TestBuildRuleEvidence_Empty(t *testing.T) {
	if got := buildRuleEvidence(nil); got != "" {
		t.Fatalf("expected empty string for nil payload, got %q", got)
	}
	if got := buildRuleEvidence([]byte{}); got != "" {
		t.Fatalf("expected empty string for empty payload, got %q", got)
	}
}

func TestBuildRuleEvidence_UsesEvidenceSummaryField(t *testing.T) {
	payload := buildRawRulePayload([]map[string]interface{}{
		{"disabled_count": float64(5), "sample_jobs": "JobA, JobB"},
	}, "5", "0", "WARNING")

	got := buildRuleEvidence(payload)
	if !strings.Contains(got, "Disabled Count: 5") {
		t.Fatalf("expected disabled count in evidence, got %q", got)
	}
	if !strings.Contains(got, "Sample Jobs: JobA, JobB") {
		t.Fatalf("expected sample jobs in evidence, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// renderRuleRemediation
// ---------------------------------------------------------------------------

func TestRenderRuleRemediationReplacesPlaceholders(t *testing.T) {
	got := renderRuleRemediation("EXEC sp_configure 'max server memory (MB)', <RecommendedMB>;", "8192")
	want := "EXEC sp_configure 'max server memory (MB)', 8192;"
	if got != want {
		t.Fatalf("renderRuleRemediation() = %q, want %q", got, want)
	}
}

func TestRenderRuleRemediation_ReplacesAllPlaceholderVariants(t *testing.T) {
	cases := []struct {
		fix  string
		rec  string
		want string
	}{
		{"SET config = <Recommended>;", "64", "SET config = 64;"},
		{"SET value = <Value>;", "ON", "SET value = ON;"},
		{"SET mem = <RecommendedMB>;", "4096", "SET mem = 4096;"},
	}
	for _, tc := range cases {
		got := renderRuleRemediation(tc.fix, tc.rec)
		if got != tc.want {
			t.Errorf("renderRuleRemediation(%q, %q) = %q, want %q", tc.fix, tc.rec, got, tc.want)
		}
	}
}

func TestRenderRuleRemediation_NoFixScript(t *testing.T) {
	got := renderRuleRemediation("", "CHECKSUM")
	if !strings.Contains(got, "CHECKSUM") {
		t.Fatalf("expected recommended value in fallback remediation, got %q", got)
	}
}

func TestRenderRuleRemediation_BothEmpty(t *testing.T) {
	if got := renderRuleRemediation("", ""); got != "" {
		t.Fatalf("expected empty string when both inputs empty, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// fallbackRuleEvidence
// ---------------------------------------------------------------------------

func TestFallbackRuleEvidence(t *testing.T) {
	entry := models.DashboardEntry{
		CurrentValue:     "3",
		RecommendedValue: "0",
	}

	got := fallbackRuleEvidence(entry)
	if !strings.Contains(got, "Current value observed: 3") {
		t.Fatalf("expected current value in fallback evidence, got %q", got)
	}
	if !strings.Contains(got, "Recommended baseline: 0") {
		t.Fatalf("expected recommended baseline in fallback evidence, got %q", got)
	}
}

func TestFallbackRuleEvidence_OnlyCurrentValue(t *testing.T) {
	got := fallbackRuleEvidence(models.DashboardEntry{CurrentValue: "5"})
	if !strings.Contains(got, "5") {
		t.Fatalf("expected current value in fallback, got %q", got)
	}
}

func TestFallbackRuleEvidence_BothEmpty(t *testing.T) {
	got := fallbackRuleEvidence(models.DashboardEntry{})
	if got != "" {
		t.Fatalf("expected empty string when both fields empty, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// summariseEvidenceRows — first-batch rule keys
// ---------------------------------------------------------------------------

func TestSummariseEvidenceRows_Empty(t *testing.T) {
	if got := summariseEvidenceRows(nil); got != "" {
		t.Fatalf("expected empty string for nil rows, got %q", got)
	}
}

func TestSummariseEvidenceRows_FailedJobs(t *testing.T) {
	rows := []map[string]interface{}{
		{"failing_jobs_24h": float64(3), "sample_jobs": "BackupJob, MaintenanceJob, StatsJob"},
	}
	got := summariseEvidenceRows(rows)
	if !strings.Contains(got, "3") {
		t.Errorf("expected failing job count in evidence, got %q", got)
	}
	if !strings.Contains(got, "BackupJob") {
		t.Errorf("expected sample job name in evidence, got %q", got)
	}
}

func TestSummariseEvidenceRows_DisabledJobs(t *testing.T) {
	rows := []map[string]interface{}{
		{"disabled_count": float64(2), "sample_jobs": "OldJob, AnotherJob"},
	}
	got := summariseEvidenceRows(rows)
	if !strings.Contains(got, "2") {
		t.Errorf("expected disabled count in evidence, got %q", got)
	}
}

func TestSummariseEvidenceRows_PageVerify(t *testing.T) {
	rows := []map[string]interface{}{
		{
			"affected_databases": float64(2),
			"sample_databases":   "userdb1, userdb2",
			"page_verify_mode":   "NONE, TORN_PAGE_DETECTION",
		},
	}
	got := summariseEvidenceRows(rows)
	if !strings.Contains(got, "2") {
		t.Errorf("expected affected database count in evidence, got %q", got)
	}
	if !strings.Contains(got, "userdb") {
		t.Errorf("expected sample database name in evidence, got %q", got)
	}
}

func TestSummariseEvidenceRows_AGReplicaHealth(t *testing.T) {
	rows := []map[string]interface{}{
		{
			"unhealthy_replicas": float64(1),
			"sample_replicas":    "SQLNODE02",
			"health_states":      "NOT_HEALTHY",
		},
	}
	got := summariseEvidenceRows(rows)
	if !strings.Contains(got, "1") {
		t.Errorf("expected unhealthy replica count in evidence, got %q", got)
	}
	if !strings.Contains(got, "SQLNODE02") {
		t.Errorf("expected replica name in evidence, got %q", got)
	}
}

func TestSummariseEvidenceRows_SingleUsePlanCache(t *testing.T) {
	rows := []map[string]interface{}{
		{
			"single_use_pct":       float64(45.5),
			"single_use_plans":     float64(9100),
			"total_compiled_plans": float64(20000),
		},
	}
	got := summariseEvidenceRows(rows)
	if !strings.Contains(got, "45.5") {
		t.Errorf("expected single_use_pct in evidence, got %q", got)
	}
}

func TestSummariseEvidenceRows_FallbackUnknownKeys(t *testing.T) {
	rows := []map[string]interface{}{
		{"some_custom_metric": float64(99), "another_field": "value_here"},
	}
	got := summariseEvidenceRows(rows)
	// Should still produce some evidence via fallback path
	if got == "" {
		t.Fatal("expected non-empty evidence from fallback unknown-key path")
	}
}

// ---------------------------------------------------------------------------
// humanizeEvidenceKey
// ---------------------------------------------------------------------------

func TestHumanizeEvidenceKey(t *testing.T) {
	cases := []struct{ in, want string }{
		{"failing_jobs_24h", "Failing Jobs 24h"},
		{"affected_databases", "Affected Databases"},
		{"unhealthy_replicas", "Unhealthy Replicas"},
		{"single_use_pct", "Single Use Pct"},
		{"health_states", "Health States"},
		{"sample_replicas", "Sample Replicas"},
		{"", ""},
	}
	for _, tc := range cases {
		got := humanizeEvidenceKey(tc.in)
		if got != tc.want {
			t.Errorf("humanizeEvidenceKey(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// formatEvidenceValue
// ---------------------------------------------------------------------------

func TestFormatEvidenceValue(t *testing.T) {
	cases := []struct {
		in   interface{}
		want string
	}{
		{nil, ""},
		{"hello", "hello"},
		{float64(3), "3"},
		{float64(45.50), "45.5"},
		{float64(45.55), "45.55"},
		{[]byte("rawbytes"), "rawbytes"},
	}
	for _, tc := range cases {
		got := formatEvidenceValue(tc.in)
		if got != tc.want {
			t.Errorf("formatEvidenceValue(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
