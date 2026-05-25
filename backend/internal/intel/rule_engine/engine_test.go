// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Unit tests for YAML Rule Engine.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package rule_engine

import (
	"testing"
)

func TestLoadRulePacks(t *testing.T) {
	rules, err := LoadRulePacks()
	if err != nil {
		t.Fatalf("failed to load rule packs: %v", err)
	}
	// Packs may be comment-only; latch_contention in maintenance.yaml is optional.
	if len(rules) == 0 {
		t.Log("no YAML rules loaded — threshold rules are dynamic-only (expected after YAML dedup)")
	}
}

func TestEvaluateSingleRule(t *testing.T) {
	rule := CompiledRule{
		Name: "test_rule",
		Conditions: []RuleCondition{
			{Metric: "cpu_load", Operator: ">", Value: 80},
		},
		Logic:    "AND",
		Severity: "high",
		Message:  "CPU load is {cpu_load}",
	}

	// Match
	metrics := map[string]float64{"cpu_load": 85}
	result := evaluateSingleRule(rule, metrics)
	if result == nil {
		t.Error("expected rule to trigger")
	} else {
		if result.RuleName != "test_rule" {
			t.Errorf("expected rule name test_rule, got %s", result.RuleName)
		}
		if result.Message != "CPU load is 85.0" {
			t.Errorf("expected formatted message, got %s", result.Message)
		}
	}

	// No match
	metrics["cpu_load"] = 70
	result = evaluateSingleRule(rule, metrics)
	if result != nil {
		t.Error("expected rule NOT to trigger")
	}
}

func TestEvaluateRulesFromRaw(t *testing.T) {
	rules := []CompiledRule{
		{
			Name: "high_tempdb",
			Conditions: []RuleCondition{
				{Metric: "tempdb_used_pct", Operator: ">", Value: 80},
			},
			Logic:    "AND",
			Severity: "medium",
			Message:  "TempDB is high",
		},
	}

	raw := map[string]interface{}{
		"tempdb_used_percent": 85.0,
	}

	triggered := EvaluateRulesFromRaw(rules, raw)
	if len(triggered) != 1 {
		t.Fatalf("expected 1 triggered rule, got %d", len(triggered))
	}
	if triggered[0].RuleName != "high_tempdb" {
		t.Errorf("expected high_tempdb, got %s", triggered[0].RuleName)
	}
}
