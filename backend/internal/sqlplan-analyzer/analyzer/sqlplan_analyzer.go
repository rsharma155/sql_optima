// File: internal/analyzer/analyzer.go
// Purpose: Core analyzer orchestration
// Package: github.com/rsharma155/sqlplan-analyzer/internal/analyzer
package analyzer

import (
	"fmt"
	"strings"
	"time"

	"github.com/rsharma155/sqlplan-analyzer/models"
	"github.com/rsharma155/sqlplan-analyzer/narrative"
	"github.com/rsharma155/sqlplan-analyzer/parser"
	"github.com/rsharma155/sqlplan-analyzer/rules"
	"github.com/rsharma155/sqlplan-analyzer/scoring"
)

type Config struct {
	EnableRules         bool
	EnableScoring       bool
	EnableNarrative     bool
	EnableCostAnalysis  bool
	MaxOperators        int
}

type Analyzer struct {
	config      Config
	ruleEngine *rules.RuleEngine
	scoring    *scoring.Calculator
	narrator   *narrative.Generator
	parser     *parser.Parser
}

func New(cfg Config) *Analyzer {
	ruleEngine := rules.NewRuleEngine()

	return &Analyzer{
		config:      cfg,
		ruleEngine: ruleEngine,
		scoring:    scoring.NewCalculator(),
		narrator:   narrative.NewGenerator(),
		parser:     parser.New(parser.Config{EnableStreaming: true}),
	}
}

func (a *Analyzer) Analyze(plan *models.PlanAnalysis) *models.PlanAnalysis {
	plan.Timestamp = time.Now()

	if a.config.EnableCostAnalysis {
		a.analyzeCosts(plan)
	}

	if a.config.EnableRules {
		a.analyzeRules(plan)
	}

	// Deduplicate warnings that might have been collected multiple times by the parser
	plan.Warnings = deduplicateWarnings(plan.Warnings)

	if a.config.EnableScoring {
		a.analyzeScoring(plan)
	}

	if a.config.EnableNarrative {
		a.analyzeNarrative(plan)
	}

	a.generateRecommendations(plan)

	return plan
}

func deduplicateWarnings(warnings []models.Warning) []models.Warning {
	if len(warnings) == 0 {
		return warnings
	}

	seen := make(map[string]bool)
	unique := make([]models.Warning, 0, len(warnings))

	for _, w := range warnings {
		// Create a key based on type and message to identify duplicates
		key := string(w.Type) + "|" + w.Message
		if !seen[key] {
			seen[key] = true
			unique = append(unique, w)
		}
	}
	return unique
}

func (a *Analyzer) analyzeCosts(plan *models.PlanAnalysis) {
	costSummary := models.CostSummary{
		OperatorCount: len(plan.Operators),
	}

	var totalCost float64
	var cpuCost float64
	var ioCost float64

	topOps := make([]models.Operator, 0)
	for _, op := range plan.Operators {
		totalCost += op.EstimatedTotalSubtreeCost
		cpuCost += op.EstimateCPUms
		ioCost += op.EstimatedIOs
		topOps = append(topOps, op)
	}

	for i := 0; i < len(topOps)-1; i++ {
		for j := i + 1; j < len(topOps); j++ {
			if topOps[j].EstimatedTotalSubtreeCost > topOps[i].EstimatedTotalSubtreeCost {
				topOps[i], topOps[j] = topOps[j], topOps[i]
			}
		}
	}

	if len(topOps) > 5 {
		topOps = topOps[:5]
	}

	costSummary.TotalEstimatedCost = totalCost
	costSummary.CPUCost = cpuCost
	costSummary.IOCost = ioCost
	costSummary.TopOperators = make([]models.OperatorCost, len(topOps))

	for i, op := range topOps {
		costSummary.TopOperators[i] = models.OperatorCost{
			ID:           op.ID,
			Name:        op.PhysicalOp,
			CPUCost:     op.EstimateCPUms,
			IOCost:      op.EstimatedIOs,
			TotalCost:   op.EstimatedTotalSubtreeCost,
			RowEstimate: int64(op.EstimateRows),
			ActualRows:  op.ActualRows,
		}
	}

	plan.CostSummary = costSummary
}

func (a *Analyzer) analyzeRules(plan *models.PlanAnalysis) {
	plan.Findings = a.ruleEngine.Evaluate(plan)
	plan.Findings = deduplicateFindingsByGroup(plan.Findings)
	plan.Findings = removeWeakFindings(plan.Findings)
}

func deduplicateFindingsByGroup(findings []models.Finding) []models.Finding {
	type groupKey struct {
		findingType  string
		affectedObj  string
		remediation  string
	}

	groups := make(map[groupKey]*models.Finding)
	result := make([]models.Finding, 0, len(findings))

	for _, f := range findings {
		affectedObj := f.OperatorName
		if tbl := extractTableName(f); tbl != "" {
			affectedObj = tbl
		}

		key := groupKey{
			findingType: f.FindingType,
			affectedObj: affectedObj,
			remediation: f.Recommendation,
		}

		if existing, ok := groups[key]; ok {
			existing.OperatorIDs = append(existing.OperatorIDs, f.OperatorID)
			if existing.EstimatedCost < f.EstimatedCost {
				existing.EstimatedCost = f.EstimatedCost
			}
			if f.Confidence > existing.Confidence {
				existing.Confidence = f.Confidence
			}
			existing.AffectedRows += f.AffectedRows
			existing.EvidenceTrace = append(existing.EvidenceTrace, f.EvidenceTrace...)
		} else {
			f.OperatorRefs = make([]models.OperatorRef, 0)
			groups[key] = &f
			result = append(result, f)
		}
	}

	return result
}

func removeWeakFindings(findings []models.Finding) []models.Finding {
	filtered := make([]models.Finding, 0, len(findings))

	weakPatterns := []string{
		"generic operator education",
		"Missing index on",
		"ExcessiveDOP",
		"serial execution",
		"unsupported CPU claims",
		"CPU Claims",
		"Gather Streams",
		"Distribute Streams",
	}

	for _, f := range findings {
		shouldRemove := false

		for _, pattern := range weakPatterns {
			if strings.Contains(strings.ToLower(f.Title), strings.ToLower(pattern)) {
				shouldRemove = true
				break
			}
		}

		if strings.HasPrefix(f.FindingType, "operator_education") {
			shouldRemove = true
		}

		if !shouldRemove {
			filtered = append(filtered, f)
		}
	}

	return filtered
}

func extractTableName(f models.Finding) string {
	if f.QueryPlanNode != nil {
		if f.QueryPlanNode.IndexScan != nil {
			return f.QueryPlanNode.IndexScan.Object.Table
		}
		if f.QueryPlanNode.TableScan != nil {
			return f.QueryPlanNode.TableScan.Object.Table
		}
	}
	return ""
}

func (a *Analyzer) analyzeScoring(plan *models.PlanAnalysis) {
	plan.HealthScore = a.scoring.Calculate(plan)

	plan.ExecutiveSummary = models.ExecutiveSummary{
		HealthScore: plan.HealthScore.OverallScore,
		Status:     a.scoring.DetermineStatus(plan.HealthScore),
	}

	var problems []string
	for _, finding := range plan.Findings {
		if finding.Severity == models.SeverityCritical || finding.Severity == models.SeverityHigh {
			problems = append(problems, finding.Title)
		}
	}
	plan.ExecutiveSummary.PrimaryProblems = problems
	plan.ExecutiveSummary.TrafficLight = plan.ExecutiveSummary.Status
}

func (a *Analyzer) analyzeNarrative(plan *models.PlanAnalysis) {
	plan.QueryNarrative = a.narrator.Generate(plan)
	plan.ExecutiveSummary.PlainEnglish = a.narrator.GeneratePlainEnglish(plan)
}

func (a *Analyzer) generateRecommendations(plan *models.PlanAnalysis) {
	plan.Recommendations = make([]models.Recommendation, 0)

	recMap := make(map[string]bool)
	categoryCount := make(map[string]int)

	for _, finding := range plan.Findings {
		categoryCount[finding.Category]++
	}

	for _, finding := range plan.Findings {
		key := finding.Title + "|" + finding.Category
		if _, exists := recMap[key]; !exists && finding.Recommendation != "" {
			recMap[key] = true

			effort := "Low"
			priority := 4

			switch finding.Severity {
			case models.SeverityCritical:
				priority = 1
				effort = "High"
			case models.SeverityHigh:
				priority = 2
				effort = "Medium"
			case models.SeverityMedium:
				priority = 3
				effort = "Low"
			}

			if finding.Category == "AccessMethods" && finding.Severity == models.SeverityHigh {
				priority = 1
				effort = "Medium"
			}
			if finding.Category == "Memory" && categoryCount["Memory"] > 1 {
				priority = 1
			}

			description := finding.Recommendation
			if finding.Explanation != "" {
				description = fmt.Sprintf("%s\n\nTechnical: %s", finding.Recommendation, finding.Explanation)
			}

			rec := models.Recommendation{
				ID:          fmt.Sprintf("%s_%d", finding.FindingType, len(plan.Recommendations)+1),
				Type:        finding.Category,
				Severity:    finding.Severity,
				Title:       finding.Title,
				Description: description,
				Impact:      finding.Impact,
				Priority:    priority,
				Effort:      effort,
			}

			plan.Recommendations = append(plan.Recommendations, rec)
		}
	}

	for i := 0; i < len(plan.Recommendations)-1; i++ {
		for j := i + 1; j < len(plan.Recommendations); j++ {
			if plan.Recommendations[j].Priority < plan.Recommendations[i].Priority {
				plan.Recommendations[i], plan.Recommendations[j] = plan.Recommendations[j], plan.Recommendations[i]
			}
		}
	}
}

func (a *Analyzer) ParsePlan(data []byte) (*models.PlanAnalysis, error) {
	p := parser.New(parser.Config{EnableStreaming: true})
	return p.ParseBytes(data)
}

func (a *Analyzer) ParseFile(filepath string) (*models.PlanAnalysis, error) {
	p := parser.New(parser.Config{EnableStreaming: true})
	return p.ParseFile(filepath)
}

func DefaultConfig() Config {
	return Config{
		EnableRules:        true,
		EnableScoring:      true,
		EnableNarrative:    true,
		EnableCostAnalysis: true,
		MaxOperators:       1000,
	}
}
