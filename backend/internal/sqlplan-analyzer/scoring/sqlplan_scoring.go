// File: internal/scoring/scoring.go
// Purpose: Health score calculation engine
// Package: github.com/rsharma155/sqlplan-analyzer/internal/scoring
package scoring

import (
	"math"
	"strings"

	"github.com/rsharma155/sqlplan-analyzer/models"
)

const (
	MaxAccessMethodsScore = 30
	MaxCardinalityScore   = 25
	MaxMemoryUsageScore   = 20
	MaxParallelismScore   = 10
	MaxIndexingScore      = 15
	MaxTotalScore         = 100
	EvidenceThreshold     = 0.10
)

type Calculator struct{}

func NewCalculator() *Calculator {
	return &Calculator{}
}

func (c *Calculator) Calculate(plan *models.PlanAnalysis) models.HealthScore {
	coverage := c.calculateEvidenceCoverage(plan)

	if coverage < EvidenceThreshold && len(plan.Operators) > 0 {
		// Even with low coverage, if we have operators, we should provide a baseline score
		// based on what we CAN see (e.g. scans, lookups detected via static analysis)
	}

	score := models.HealthScore{
		OverallScore:       0,
		AccessMethodsScore: c.calculateAccessMethodsScore(plan),
		MemoryUsageScore:   c.calculateMemoryUsageScore(plan),
		JoinStrategyScore:  c.calculateIndexingScore(plan),
		ParallelismScore:   c.calculateParallelismScore(plan),
		CardinalityScore:   c.calculateCardinalityScore(plan),
		Breakdown:          make(map[string]int),
	}

	score.OverallScore = score.AccessMethodsScore +
		score.CardinalityScore +
		score.MemoryUsageScore +
		score.ParallelismScore +
		score.JoinStrategyScore

	score.Breakdown["AccessMethods"] = score.AccessMethodsScore
	score.Breakdown["Cardinality"] = score.CardinalityScore
	score.Breakdown["MemoryUsage"] = score.MemoryUsageScore
	score.Breakdown["Parallelism"] = score.ParallelismScore
	score.Breakdown["Indexing"] = score.JoinStrategyScore

	return score
}

func (c *Calculator) calculateEvidenceCoverage(plan *models.PlanAnalysis) float64 {
	if len(plan.Operators) == 0 {
		return 0
	}

	opsWithEvidence := 0
	totalOps := len(plan.Operators)

	for _, op := range plan.Operators {
		hasEvidence := false

		if op.ActualRows > 0 || op.ActualExecutions > 0 {
			hasEvidence = true
		}
		if op.ActualCPUms > 0 || op.ActualElapsedms > 0 {
			hasEvidence = true
		}
		if op.ActualLogicalReads > 0 || op.ActualPhysicalReads > 0 {
			hasEvidence = true
		}
		if len(op.RuntimeCounters) > 0 {
			hasEvidence = true
		}
		if op.EstimatedTotalSubtreeCost > 0 {
			hasEvidence = true
		}

		if hasEvidence {
			opsWithEvidence++
		}
	}

	if totalOps == 0 {
		return 0
	}
	return float64(opsWithEvidence) / float64(totalOps)
}

func (c *Calculator) calculateAccessMethodsScore(plan *models.PlanAnalysis) int {
	score := MaxAccessMethodsScore

	for _, op := range plan.Operators {
		if op.TableScan != nil {
			score -= 15
		}
		if op.IndexScan != nil {
			scanType := op.IndexScan.ScanType
			if scanType == "Full" && !strings.Contains(op.PhysicalOp, "Seek") {
				score -= 8
			}
		}

		physicalOp := op.PhysicalOp
		if contains(physicalOp, "Key Lookup") || contains(physicalOp, "RID Lookup") {
			score -= 10
		}
	}

	if score < 0 {
		score = 0
	}
	return score
}

func (c *Calculator) calculateMemoryUsageScore(plan *models.PlanAnalysis) int {
	score := MaxMemoryUsageScore

	for _, warning := range plan.Warnings {
		if warning.Type == models.WarningTypeSpillToTempDB {
			score -= 15
			break
		}
	}

	if plan.QueryPlan != nil && plan.QueryPlan.HasMemoryGrant {
		grant := plan.QueryPlan.MemoryGrantInfo
		if grant.GrantedMemory > 0 {
			util := float64(grant.MaxUsedMemory) / float64(grant.GrantedMemory)
			if util > 0.95 {
				score -= 10
			} else if util < 0.3 && grant.GrantedMemory > 1024*1024 {
				score -= 5
			}
		}
	}

	for _, finding := range plan.Findings {
		if finding.Category == "Memory" && finding.Severity == models.SeverityHigh {
			score -= 5
		}
	}

	if score < 0 {
		score = 0
	}
	return score
}

func (c *Calculator) calculateIndexingScore(plan *models.PlanAnalysis) int {
	score := MaxIndexingScore

	for _, finding := range plan.Findings {
		if finding.Category == "Indexing" {
			deduction := 5
			if finding.Severity == models.SeverityHigh {
				deduction = 8
			} else if finding.Severity == models.SeverityCritical {
				deduction = 10
			}
			score -= deduction
		}
	}

	if len(plan.MissingIndexes) > 0 {
		score -= 3 * len(plan.MissingIndexes)
	}

	if score < 0 {
		score = 0
	}
	return score
}

func (c *Calculator) calculateParallelismScore(plan *models.PlanAnalysis) int {
	score := MaxParallelismScore

	for _, finding := range plan.Findings {
		if finding.Category == "Parallelism" {
			score -= 3
		}
	}

	if plan.QueryPlan != nil && plan.QueryPlan.DegreeOfParallelism <= 1 {
		hasComplexOp := false
		for _, op := range plan.Operators {
			if strings.Contains(op.PhysicalOp, "Hash") || strings.Contains(op.PhysicalOp, "Sort") {
				hasComplexOp = true
				break
			}
		}
		if !hasComplexOp {
			score = MaxParallelismScore
		}
	}

	if score < 0 {
		score = 0
	}
	return score
}

func (c *Calculator) calculateCardinalityScore(plan *models.PlanAnalysis) int {
	score := MaxCardinalityScore

	varianceSum := 0.0
	varianceCount := 0
	for _, op := range plan.Operators {
		if op.EstimateRows > 0 && op.ActualRows > 0 {
			ratio := math.Abs(float64(op.ActualRows)/op.EstimateRows - 1)
			varianceSum += ratio
			varianceCount++
		}
	}

	if varianceCount > 0 {
		avgVariance := varianceSum / float64(varianceCount)
		if avgVariance > 100 {
			score -= 20
		} else if avgVariance > 10 {
			score -= 15
		} else if avgVariance > 2 {
			score -= 8
		} else if avgVariance > 1 {
			score -= 3
		}
	}

	for _, finding := range plan.Findings {
		if finding.Category == "Cardinality" {
			severityPenalty := 0
			switch finding.Severity {
			case models.SeverityCritical:
				severityPenalty = 8
			case models.SeverityHigh:
				severityPenalty = 5
			case models.SeverityMedium:
				severityPenalty = 3
			case models.SeverityLow:
				severityPenalty = 1
			}
			score -= severityPenalty
		}
	}

	if score < 0 {
		score = 0
	}
	return score
}

func (c *Calculator) DetermineStatus(score models.HealthScore) string {
	if score.Breakdown["InsufficientRuntimeData"] > 0 {
		return "Insufficient Runtime Data"
	}
	if score.OverallScore >= 80 {
		return "Green"
	}
	if score.OverallScore >= 50 {
		return "Yellow"
	}
	return "Red"
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr ||
			(len(s) > len(substr) &&
				(s[:len(substr)] == substr || containsAt(s, substr))))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
