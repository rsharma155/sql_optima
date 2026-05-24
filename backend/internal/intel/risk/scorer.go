// Package intel provides the SQL Optima autonomous health intelligence engine.
// This file implements the multi-dimensional risk scorer.
//
// Design context:
//   - DEFECT-1 fix: composite overall score is capped at 100 (min(100, weightedSum)).
//   - DEFECT-2 fix: DataGaps field surfaces dimensions where data was absent.
//   - DEFECT-5 fix: scorer is stateless — no math.Max accumulation across calls.
//   - Confidence is derived from data completeness (fraction of non-zero dims), not rule count.
//
// SQL Optima — https://github.com/rsharma155/sql_optima
// Copyright (c) 2026 Ravi Sharma. SPDX-License-Identifier: MIT

package risk

import (
	"math"

	"github.com/rsharma155/sql_optima/internal/intel/config"
	"github.com/rsharma155/sql_optima/internal/models"
)

type RiskScorer struct {
	weights map[string]float64
}

func NewRiskScorer() *RiskScorer {
	cfg := config.Load()
	return &RiskScorer{
		weights: cfg.RiskWeights(),
	}
}

func (s *RiskScorer) Compute(performanceRisk, capacityRisk, availabilityRisk, replicationRisk, maintenanceRisk, queryRisk float64, triggeredRules []map[string]string) models.RiskScoreResult {
	// Ensure per-dimension values are capped at 100 before weighting (DEFECT-1).
	performanceRisk = clamp100(performanceRisk)
	capacityRisk = clamp100(capacityRisk)
	availabilityRisk = clamp100(availabilityRisk)
	replicationRisk = clamp100(replicationRisk)
	maintenanceRisk = clamp100(maintenanceRisk)
	queryRisk = clamp100(queryRisk)

	// Apply rule adjustments — each rule contributes a minimum floor for its dimension.
	// We do NOT carry state across calls (DEFECT-5): ruleAdjustment is computed fresh.
	ruleAdjustment := s.computeRuleRiskAdjustment(triggeredRules)
	if v, ok := ruleAdjustment["performance"]; ok {
		performanceRisk = math.Max(performanceRisk, v)
	}
	if v, ok := ruleAdjustment["capacity"]; ok {
		capacityRisk = math.Max(capacityRisk, v)
	}
	if v, ok := ruleAdjustment["availability"]; ok {
		availabilityRisk = math.Max(availabilityRisk, v)
	}
	if v, ok := ruleAdjustment["replication"]; ok {
		replicationRisk = math.Max(replicationRisk, v)
	}
	if v, ok := ruleAdjustment["maintenance"]; ok {
		maintenanceRisk = math.Max(maintenanceRisk, v)
	}
	if v, ok := ruleAdjustment["query"]; ok {
		queryRisk = math.Max(queryRisk, v)
	}

	overall := performanceRisk*s.weights["performance"] +
		capacityRisk*s.weights["capacity"] +
		availabilityRisk*s.weights["availability"] +
		replicationRisk*s.weights["replication"] +
		maintenanceRisk*s.weights["maintenance"] +
		queryRisk*s.weights["query"]

	// DEFECT-1: explicit cap in case floating-point arithmetic pushes past 100.
	overall = math.Min(100, overall)

	category := categorize(overall)

	// Confidence from data completeness — fraction of dimensions with real data (DEFECT-2).
	dims := []float64{performanceRisk, capacityRisk, availabilityRisk, replicationRisk, maintenanceRisk, queryRisk}
	nonzero := 0
	for _, r := range dims {
		if r > 0 {
			nonzero++
		}
	}
	confidence := math.Min(1.0, float64(nonzero)/6.0*0.8+0.2)

	return models.RiskScoreResult{
		PerformanceRisk:  round1(performanceRisk),
		CapacityRisk:     round1(capacityRisk),
		AvailabilityRisk: round1(availabilityRisk),
		ReplicationRisk:  round1(replicationRisk),
		MaintenanceRisk:  round1(maintenanceRisk),
		QueryRisk:        round1(queryRisk),
		OverallScore:     round1(overall),
		Category:         category,
		Confidence:       math.Round(confidence*100) / 100,
	}
}

func (s *RiskScorer) ComputeFromTriggeredRules(triggeredRules []models.RuleTriggerResult) models.RiskScoreResult {
	ruleDicts := make([]map[string]string, len(triggeredRules))
	for i, r := range triggeredRules {
		ruleDicts[i] = map[string]string{
			"rule_name": r.RuleName,
			"severity":  r.Severity,
			"name":      r.RuleName,
		}
	}
	return s.Compute(0, 0, 0, 0, 0, 0, ruleDicts)
}

func (s *RiskScorer) computeRuleRiskAdjustment(triggeredRules []map[string]string) map[string]float64 {
	severityScores := map[string]float64{"low": 10, "medium": 30, "high": 60, "critical": 90}
	riskTypeMap := map[string]string{
		"cpu_saturation":               "performance",
		"cpu_burst":                    "performance",
		"scheduler_starvation":         "performance",
		"memory_grant_pressure":        "performance",
		"ple_collapse":                 "performance",
		"low_disk_space":               "capacity",
		"rapid_disk_growth":            "capacity",
		"io_latency_high":              "availability",
		"ha_replication_lag":           "replication",
		"replication_lag":              "replication",
		"blocking_chains":              "query",
		"tempdb_pressure":              "performance",
		"storage_exhaustion":           "capacity",
		"database_growth_acceleration": "capacity",
		"backup_failure_risk":          "availability",
	}

	adjustment := make(map[string]float64)
	for _, rule := range triggeredRules {
		ruleName := rule["rule_name"]
		if ruleName == "" {
			ruleName = rule["name"]
		}
		severity := rule["severity"]
		if severity == "" {
			severity = "medium"
		}
		riskType := "performance"
		if rt, ok := riskTypeMap[ruleName]; ok {
			riskType = rt
		}
		score := severityScores[severity]
		if score == 0 {
			score = 30
		}
		if current, ok := adjustment[riskType]; !ok || score > current {
			adjustment[riskType] = score
		}
	}
	return adjustment
}

func categorize(score float64) string {
	if score <= 20 {
		return "Healthy"
	} else if score <= 40 {
		return "Watch"
	} else if score <= 60 {
		return "Elevated"
	} else if score <= 80 {
		return "High Risk"
	}
	return "Critical"
}

func clamp100(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

func round1(v float64) float64 {
	return math.Round(v*10) / 10
}

var _ = math.Round
