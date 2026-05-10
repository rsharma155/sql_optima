// File: internal/narrative/narrative.go
// Purpose: Query narrative generator for business-friendly explanations
// Package: github.com/rsharma155/sqlplan-analyzer/internal/narrative
package narrative

import (
	"fmt"
	"sort"
	"strings"

	"github.com/rsharma155/sqlplan-analyzer/models"
)

type Generator struct {
	operators []models.Operator
}

func NewGenerator() *Generator {
	return &Generator{}
}

func (g *Generator) Generate(plan *models.PlanAnalysis) []string {
	narrative := make([]string, 0)

	if plan.QueryPlan == nil || plan.QueryPlan.RelOp == nil {
		return []string{"Unable to generate narrative: no query plan available"}
	}

	g.operators = plan.Operators

	sortedOps := g.sortByExecutionOrder(plan.Operators)

	for i, op := range sortedOps {
		step := g.generateStep(i+1, op)
		if step != "" {
			narrative = append(narrative, step)
		}
	}

	if len(narrative) == 0 {
		narrative = append(narrative, "Query execution plan is efficient with no significant issues.")
	}

	return narrative
}

func (g *Generator) sortByExecutionOrder(ops []models.Operator) []models.Operator {
	sorted := make([]models.Operator, len(ops))
	copy(sorted, ops)

	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Depth != sorted[j].Depth {
			return sorted[i].Depth < sorted[j].Depth
		}
		return sorted[i].EstimatedTotalSubtreeCost > sorted[j].EstimatedTotalSubtreeCost
	})

	return sorted[:min(10, len(sorted))]
}

func (g *Generator) generateStep(stepNum int, op models.Operator) string {
	switch op.PhysicalOp {
	case "Table Scan":
		return fmt.Sprintf("Step %d: SQL Server scanned the entire %s table, reading every row sequentially.", stepNum, op.TableScan.Object.Table)
	case "Index Seek":
		return fmt.Sprintf("Step %d: Using the index on %s, SQL Server directly located the relevant rows.", stepNum, op.IndexScan.Object.Table)
	case "Index Scan":
		return fmt.Sprintf("Step %d: SQL Server scanned the index on %s to find matching rows.", stepNum, op.IndexScan.Object.Table)
	case "Nested Loops":
		return fmt.Sprintf("Step %d: SQL Server joined results using a nested loop, checking each row from the outer table against the inner table.", stepNum)
	case "Hash Match":
		return fmt.Sprintf("Step %d: SQL Server used a hash join to combine data from both tables.", stepNum)
	case "Merge Join":
		return fmt.Sprintf("Step %d: SQL Server performed a merge join on sorted data from both tables.", stepNum)
	case "Sort":
		return fmt.Sprintf("Step %d: SQL Server sorted the results in memory.", stepNum)
	case "Parallelism":
		return fmt.Sprintf("Step %d: SQL Server distributed the work across multiple threads for parallel processing.", stepNum)
	default:
		if len(op.PhysicalOp) > 0 {
			return fmt.Sprintf("Step %d: SQL Server performed a %s operation.", stepNum, op.PhysicalOp)
		}
	}
	return ""
}

func (g *Generator) GeneratePlainEnglish(plan *models.PlanAnalysis) models.BusinessExplanation {
	exp := models.BusinessExplanation{
		Status:      g.DetermineStatus(plan),
		Problems:    make([]string, 0),
		ActionItems: make([]string, 0),
	}

	findings := g.categorizeFindings(plan.Findings)

	if len(findings["critical"]) > 0 {
		exp.Problems = append(exp.Problems, "The query has critical performance issues requiring immediate attention.")
		exp.ActionItems = append(exp.ActionItems, "Review and optimize the critical bottlenecks")
	}

	if len(findings["high"]) > 0 {
		exp.Problems = append(exp.Problems, "High severity issues detected that impact query performance.")
		exp.ActionItems = append(exp.ActionItems, "Create missing indexes and optimize joins")
	}

	exp.Summary = g.generateSummary(plan, findings)

	exp.Analogy = g.generateAnalogy(plan)

	exp.Impact = g.generateBusinessImpact(plan)

	return exp
}

func (g *Generator) DetermineStatus(plan *models.PlanAnalysis) string {
	hasCritical := false
	hasHigh := false
	highConfidenceCritical := false
	highConfidenceHigh := false

	for _, finding := range plan.Findings {
		if finding.Severity == models.SeverityCritical {
			hasCritical = true
			if finding.Confidence >= 0.7 {
				highConfidenceCritical = true
			}
		}
		if finding.Severity == models.SeverityHigh {
			hasHigh = true
			if finding.Confidence >= 0.7 {
				highConfidenceHigh = true
			}
		}
	}

	if hasCritical && highConfidenceCritical {
		return "Possible Critical Performance Issues"
	}
	if hasCritical {
		return "Potential Critical Issues - Needs Verification"
	}
	if hasHigh && highConfidenceHigh {
		return "Possible Performance Optimization Opportunity"
	}
	if hasHigh {
		return "Potential Optimization Area - Needs Investigation"
	}
	return "Query Appears Healthy"
}

func (g *Generator) categorizeFindings(findings []models.Finding) map[string][]models.Finding {
	result := map[string][]models.Finding{
		"critical": make([]models.Finding, 0),
		"high":     make([]models.Finding, 0),
		"medium":   make([]models.Finding, 0),
		"low":      make([]models.Finding, 0),
	}

	for _, f := range findings {
		switch f.Severity {
		case models.SeverityCritical:
			result["critical"] = append(result["critical"], f)
		case models.SeverityHigh:
			result["high"] = append(result["high"], f)
		case models.SeverityMedium:
			result["medium"] = append(result["medium"], f)
		default:
			result["low"] = append(result["low"], f)
		}
	}

	return result
}

func (g *Generator) generateSummary(plan *models.PlanAnalysis, findings map[string][]models.Finding) string {
	var summary strings.Builder

	criticalCount := len(findings["critical"])
	highCount := len(findings["high"])
	mediumCount := len(findings["medium"])

	if criticalCount > 0 {
		hasStrongEvidence := false
		for _, f := range findings["critical"] {
			if f.Confidence >= 0.7 {
				hasStrongEvidence = true
				break
			}
		}
		if hasStrongEvidence {
			summary.WriteString(fmt.Sprintf("Found %d critical performance issue(s) with strong evidence. ", criticalCount))
		} else {
			summary.WriteString(fmt.Sprintf("Found %d possible performance issue(s) requiring further investigation. ", criticalCount))
		}
	} else if highCount > 0 {
		hasStrongEvidence := false
		for _, f := range findings["high"] {
			if f.Confidence >= 0.7 {
				hasStrongEvidence = true
				break
			}
		}
		if hasStrongEvidence {
			summary.WriteString(fmt.Sprintf("Found %d high severity issue(s) with supporting evidence. ", highCount))
		} else {
			summary.WriteString(fmt.Sprintf("Found %d possible optimization opportunity(s). ", highCount))
		}
	} else if mediumCount > 0 {
		summary.WriteString(fmt.Sprintf("Found %d medium severity observation(s) that may benefit from optimization. ", mediumCount))
	} else {
		summary.WriteString("No significant performance issues detected. ")
	}

	for _, op := range plan.Operators {
		if op.TableScan != nil {
			summary.WriteString("The query performs full table scans which can be slow on large tables.")
			break
		}
	}

	return summary.String()
}

func (g *Generator) generateAnalogy(plan *models.PlanAnalysis) string {
	if len(plan.Findings) == 0 {
		return "The database query is well-optimized with no significant performance bottlenecks."
	}

	for _, f := range plan.Findings {
		if f.Category == "Warnings" && strings.Contains(f.Title, "Cartesian") {
			return "This is like accidentally multiplying every row by every other row, creating an enormous result set."
		}
	}

	for _, op := range plan.Operators {
		if op.TableScan != nil {
			return "This is like reading every page in a book to find a single word instead of using the index at the back of the book."
		}
		if strings.Contains(op.PhysicalOp, "Key Lookup") {
			return "This is like having to walk across a warehouse for each item on your shopping list."
		}
	}

	for _, f := range plan.Findings {
		if strings.Contains(f.Impact, "spill") || strings.Contains(f.Impact, "tempdb") {
			return "This is like running out of desk space and having to spread papers on the floor."
		}
		if f.Category == "Cardinality" {
			return "This is like using a road map that hasn't been updated in years - the estimated travel times are way off."
		}
		if f.Category == "Memory" {
			return "This is like trying to carry too many groceries without enough bags - things keep spilling."
		}
	}

	return "The database query may have performance considerations that should be reviewed."
}

func (g *Generator) generateBusinessImpact(plan *models.PlanAnalysis) string {
	hasCritical := false
	hasHigh := false
	for _, f := range plan.Findings {
		if f.Severity == models.SeverityCritical {
			hasCritical = true
		}
		if f.Severity == models.SeverityHigh {
			hasHigh = true
		}
	}

	mediumCount := 0
	for _, f := range plan.Findings {
		if f.Severity == models.SeverityMedium {
			mediumCount++
		}
	}

	if hasCritical {
		return "Critical performance bottlenecks detected - query may timeout or cause application errors under load."
	}
	if hasHigh && len(plan.Findings) > 5 {
		return "Multiple performance issues detected - query consumes significant server resources and may impact other workloads."
	}
	if hasHigh {
		return "Query has performance issues that may cause slowdowns as data volumes grow."
	}
	if mediumCount > 5 {
		return "Query has moderate performance issues that would benefit from optimization to ensure scalability as data volumes grow."
	}
	if plan.HealthScore.OverallScore < 70 {
		return "This query has moderate resource usage that may benefit from optimization."
	}
	return "This query runs efficiently without significant resource concerns."
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

