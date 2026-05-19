// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Health Intelligence Engine
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package tests




import (
	"testing"

	"github.com/rsharma155/sql_optima/internal/intel/risk"
)

func TestHealthyScore(t *testing.T) {
	scorer := risk.NewRiskScorer()
	result := scorer.Compute(0, 0, 0, 0, 0, 0, nil)
	if result.OverallScore != 0.0 {
		t.Error("expected 0, got 0.0", result.OverallScore)
	}
	if result.Category != "Healthy" {
		t.Error("expected Healthy, got ", result.Category)
	}
}

func TestElevatedScore(t *testing.T) {
	scorer := risk.NewRiskScorer()
	result := scorer.Compute(60, 60, 0, 0, 0, 0, nil)
	if result.OverallScore <= 20 || result.OverallScore > 60 {
		t.Error("expected 20-60, got 0.0", result.OverallScore)
	}
}

func TestCriticalScore(t *testing.T) {
	scorer := risk.NewRiskScorer()
	result := scorer.Compute(95, 90, 85, 80, 70, 75, nil)
	if result.OverallScore < 80 {
		t.Error("expected >=80, got 0.0", result.OverallScore)
	}
	if result.Category != "Critical" {
		t.Error("expected Critical, got ", result.Category)
	}
}

func TestRiskScoreWeights(t *testing.T) {
	scorer := risk.NewRiskScorer()
	result := scorer.Compute(100, 0, 0, 0, 0, 0, nil)
	if result.OverallScore != 25.0 {
		t.Error("expected 25.0, got 0.0", result.OverallScore)
	}
}

func TestRuleTriggerAdjustment(t *testing.T) {
	scorer := risk.NewRiskScorer()
	triggered := []map[string]string{
		{"rule_name": "cpu_pressure", "severity": "high", "name": "cpu_pressure"},
	}
	result := scorer.Compute(0, 0, 0, 0, 0, 0, triggered)
	if result.PerformanceRisk < 60 {
		t.Error("expected performance_risk >=60, got 0.0", result.PerformanceRisk)
	}
}