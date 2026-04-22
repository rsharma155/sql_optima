package rules

import (
	"strings"
	"testing"
)

func TestGenerateWhyThisMatters_MaxDOP(t *testing.T) {
	why := GenerateWhyThisMatters("maxdop")

	if !strings.Contains(why, "CPU") {
		t.Errorf("Expected CPU mention in why, got: %s", why)
	}
	if !strings.Contains(why, "parallelism") {
		t.Errorf("Expected parallelism mention, got: %s", why)
	}
}

func TestGenerateWhyThisMatters_PostgresAutovacuum(t *testing.T) {
	why := GenerateWhyThisMatters("pg_autovacuum")

	if !strings.Contains(why, "storage") {
		t.Errorf("Expected storage mention, got: %s", why)
	}
	if !strings.Contains(why, "vacuum") {
		t.Errorf("Expected vacuum mention, got: %s", why)
	}
}

func TestGenerateImpact(t *testing.T) {
	impact := GenerateImpact("maxdop")

	if !strings.Contains(impact, "CPU") {
		t.Errorf("Expected CPU mention in impact, got: %s", impact)
	}
	if !strings.Contains(impact, "latency") {
		t.Errorf("Expected latency mention, got: %s", impact)
	}
}

func TestGetRiskLevel(t *testing.T) {
	tests := []struct {
		ruleID    string
		expected string
	}{
		{"max_server_memory", "HIGH"},
		{"pg_autovacuum", "HIGH"},
		{"pg_vacuum_health", "HIGH"},
		{"maxdop", "MEDIUM"},
		{"pg_cache_hit", "MEDIUM"},
		{"unknown_rule", "MEDIUM"},
	}

	for _, tt := range tests {
		t.Run(tt.ruleID, func(t *testing.T) {
			risk := GetRiskLevel(tt.ruleID)
			if risk != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, risk)
			}
		})
	}
}

func TestGetConfidenceNote(t *testing.T) {
	note := GetConfidenceNote("maxdop")

	if note == "" {
		t.Errorf("Expected non-empty confidence note")
	}
	if !strings.Contains(note, "MAXDOP") && !strings.Contains(note, "parallel") && !strings.Contains(note, "CXPACKET") && !strings.Contains(note, "confidence") {
		t.Errorf("Expected meaningful note, got: %s", note)
	}
}

func TestJoinWithAnd(t *testing.T) {
	tests := []struct {
		input    []string
		expected string
	}{
		{[]string{"a"}, "a"},
		{[]string{"a", "b"}, "a and b"},
		{[]string{"a", "b", "c"}, "a, b and c"},
		{[]string{}, ""},
	}

	for _, tt := range tests {
		result := joinWithAnd(tt.input)
		if result != tt.expected {
			t.Errorf("Expected %s, got %s", tt.expected, result)
		}
	}
}