// File: internal/reporter/markdown.go
// Purpose: Markdown report generator
// Package: github.com/rsharma155/sqlplan-analyzer/internal/reporter
package reporter

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/rsharma155/sqlplan-analyzer/models"
)

type MarkdownReporter struct{}

func NewMarkdownReporter() *MarkdownReporter {
	return &MarkdownReporter{}
}

func (r *MarkdownReporter) Generate(plan *models.PlanAnalysis) string {
	var sb strings.Builder

	sb.WriteString(r.renderHeader(plan))
	sb.WriteString(r.renderExecutiveSummary(plan))
	sb.WriteString(r.renderHealthScore(plan))
	sb.WriteString(r.renderFindings(plan))
	sb.WriteString(r.renderRecommendations(plan))
	sb.WriteString(r.renderMissingIndexes(plan))
	sb.WriteString(r.renderOperators(plan))
	sb.WriteString(r.renderWarnings(plan))

	return sb.String()
}

func (r *MarkdownReporter) renderHeader(plan *models.PlanAnalysis) string {
	return fmt.Sprintf(`# SQL Server Execution Plan Analysis

**Generated:** %s

---

`,
		plan.Timestamp.Format("2006-01-02 15:04:05"))
}

func (r *MarkdownReporter) renderExecutiveSummary(plan *models.PlanAnalysis) string {
	var sb strings.Builder
	plain := plan.ExecutiveSummary.PlainEnglish

	sb.WriteString("## Executive Summary\n\n")

	sb.WriteString(fmt.Sprintf("**Health Score:** %d (%s)\n\n",
		plan.HealthScore.OverallScore,
		plan.ExecutiveSummary.TrafficLight,
	))

	if len(plan.ExecutiveSummary.PrimaryProblems) > 0 {
		sb.WriteString("### Primary Problems\n\n")
		for _, problem := range plan.ExecutiveSummary.PrimaryProblems {
			sb.WriteString(fmt.Sprintf("- %s\n", problem))
		}
		sb.WriteString("\n")
	}

	if plain.Summary != "" {
		sb.WriteString(fmt.Sprintf("**Summary:** %s\n\n", plain.Summary))
	}

	if plain.Analogy != "" {
		sb.WriteString(fmt.Sprintf("> **Analogy:** %s\n\n", plain.Analogy))
	}

	if plain.Impact != "" {
		sb.WriteString(fmt.Sprintf("**Business Impact:** %s\n\n", plain.Impact))
	}

	sb.WriteString("---\n\n")

	return sb.String()
}

func (r *MarkdownReporter) renderHealthScore(plan *models.PlanAnalysis) string {
	breakdown := plan.HealthScore.Breakdown

	return fmt.Sprintf(`## Health Score Breakdown

| Category | Score |
|----------|-------|
| Access Methods | %d/%d |
| Memory Usage | %d/%d |
| Join Strategy | %d/%d |
| Parallelism | %d/%d |
| Cardinality | %d/%d |
| **Total** | **%d/100** |

---

`,
		breakdown["AccessMethods"],
		40,
		breakdown["MemoryUsage"],
		20,
		breakdown["JoinStrategy"],
		20,
		breakdown["Parallelism"],
		10,
		breakdown["Cardinality"],
		10,
		plan.HealthScore.OverallScore,
	)
}

func (r *MarkdownReporter) renderFindings(plan *models.PlanAnalysis) string {
	var sb strings.Builder

	sb.WriteString("## Findings\n\n")

	if len(plan.Findings) == 0 {
		sb.WriteString("No performance issues detected.\n\n")
		sb.WriteString("---\n\n")
		return sb.String()
	}

	bySeverity := make(map[models.Severity][]models.Finding)
	for _, f := range plan.Findings {
		bySeverity[f.Severity] = append(bySeverity[f.Severity], f)
	}

	order := []models.Severity{models.SeverityCritical, models.SeverityHigh, models.SeverityMedium, models.SeverityLow, models.SeverityInfo}

	for _, sev := range order {
		findings, ok := bySeverity[sev]
		if !ok || len(findings) == 0 {
			continue
		}

		sb.WriteString(fmt.Sprintf("### %s Severity\n\n", sev))

		for _, f := range findings {
			icon := r.getSeverityIcon(sev)
			sb.WriteString(fmt.Sprintf("%s **%s**\n\n", icon, f.Title))
			sb.WriteString(fmt.Sprintf("- Type: `%s`\n", f.FindingType))

			if f.OperatorName != "" {
				sb.WriteString(fmt.Sprintf("- Operator: `%s` (ID: %d)\n", f.OperatorName, f.OperatorID))
			}

			sb.WriteString(fmt.Sprintf("- Severity: %s\n", sev))

			if f.Confidence > 0 {
				sb.WriteString(fmt.Sprintf("- Confidence: %.0f%%\n", f.Confidence*100))
			}

			if f.TechnicalExplanation != "" {
				sb.WriteString(fmt.Sprintf("- Technical: %s\n", f.TechnicalExplanation))
			}

			if f.BusinessExplanation != "" {
				sb.WriteString(fmt.Sprintf("- Business: %s\n", f.BusinessExplanation))
			}

			if f.Recommendation != "" {
				sb.WriteString(fmt.Sprintf("- **Recommendation:** %s\n", f.Recommendation))
			}

			sb.WriteString("\n")
		}
	}

	sb.WriteString("---\n\n")
	return sb.String()
}

func (r *MarkdownReporter) getSeverityIcon(sev models.Severity) string {
	switch sev {
	case models.SeverityCritical:
		return "🔴"
	case models.SeverityHigh:
		return "🟠"
	case models.SeverityMedium:
		return "🟡"
	case models.SeverityLow:
		return "🟢"
	default:
		return "ℹ️"
	}
}

func (r *MarkdownReporter) renderRecommendations(plan *models.PlanAnalysis) string {
	var sb strings.Builder

	sb.WriteString("## Recommendations\n\n")

	if len(plan.Recommendations) == 0 {
		sb.WriteString("No recommendations at this time.\n\n")
		sb.WriteString("---\n\n")
		return sb.String()
	}

	for _, rec := range plan.Recommendations {
		sb.WriteString(fmt.Sprintf("### %d. %s\n\n", rec.Priority, rec.Title))
		sb.WriteString(fmt.Sprintf("- **Type:** %s\n", rec.Type))
		sb.WriteString(fmt.Sprintf("- **Severity:** %s\n", rec.Severity))
		sb.WriteString(fmt.Sprintf("- **Effort:** %s\n", rec.Effort))

		if rec.Description != "" {
			sb.WriteString(fmt.Sprintf("- **Description:** %s\n", rec.Description))
		}

		if rec.Impact != "" {
			sb.WriteString(fmt.Sprintf("- **Impact:** %s\n", rec.Impact))
		}

		if rec.SQL != "" {
			sb.WriteString(fmt.Sprintf("\n```sql\n%s\n```\n\n", rec.SQL))
		}

		sb.WriteString("\n")
	}

	sb.WriteString("---\n\n")
	return sb.String()
}

func (r *MarkdownReporter) renderMissingIndexes(plan *models.PlanAnalysis) string {
	var sb strings.Builder

	sb.WriteString("## Missing Indexes\n\n")

	if len(plan.MissingIndexes) == 0 {
		sb.WriteString("No missing indexes detected.\n\n")
		sb.WriteString("---\n\n")
		return sb.String()
	}

	sb.WriteString("| Database | Table | Score |\n")
	sb.WriteString("|----------|-------|-------|\n")

	for _, mi := range plan.MissingIndexes {
		sb.WriteString(fmt.Sprintf("| %s | %s | %d |\n",
			mi.Database, mi.Table, mi.Score))
	}

	sb.WriteString("\n---\n\n")
	return sb.String()
}

func (r *MarkdownReporter) renderOperators(plan *models.PlanAnalysis) string {
	var sb strings.Builder

	sb.WriteString("## Top Operators\n\n")

	if len(plan.Operators) == 0 {
		sb.WriteString("No operators found.\n\n")
		sb.WriteString("---\n\n")
		return sb.String()
	}

	sb.WriteString("| ID | Operation | Est. Cost | Est. Rows | Actual Rows |\n")
	sb.WriteString("|----|-----------|-----------|----------|-------------|\n")

	for i, op := range plan.Operators[:GetMinInt(20, len(plan.Operators))] {
		estRows := fmt.Sprintf("%.0f", op.EstimateRows)
		actRows := strconv.FormatInt(op.ActualRows, 10)

		sb.WriteString(fmt.Sprintf("| %d | %s | %.4f | %s | %s |\n",
			i+1,
			op.PhysicalOp,
			op.EstimatedTotalSubtreeCost,
			estRows,
			actRows,
		))
	}

	sb.WriteString("\n---\n\n")
	return sb.String()
}

func (r *MarkdownReporter) renderWarnings(plan *models.PlanAnalysis) string {
	var sb strings.Builder

	sb.WriteString("## Warnings\n\n")

	if len(plan.Warnings) == 0 {
		sb.WriteString("No warnings.\n\n")
		sb.WriteString("---\n\n")
		return sb.String()
	}

	sb.WriteString("| Type | Message | Severity |\n")
	sb.WriteString("|------|---------|----------|\n")

	for _, w := range plan.Warnings {
		sb.WriteString(fmt.Sprintf("| %s | %s | %s |\n",
			w.Type, w.Message, w.Severity))
	}

	sb.WriteString("\n---\n\n")
	return sb.String()
}

func GetMinInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
