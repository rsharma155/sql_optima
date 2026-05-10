// File: internal/sqlplan_rules/engine.go
// Purpose: Comprehensive rule engine based on Grant Fritchey SQL Server Execution Plans book
// Package: github.com/rsharma155/sqlplan-analyzer/rules
package rules

import (
	"github.com/rsharma155/sqlplan-analyzer/models"
)

type RuleEngine struct {
	rules        []RuleDefinition
	ruleRegistry map[string]bool
}

type RuleDefinition struct {
	Name                   string
	Description             string
	Category                string
	Severity                models.Severity
	RuleType                RuleType
	Condition               func(*models.PlanAnalysis, *models.Operator) bool
	ExtractFinding          func(*models.PlanAnalysis, *models.Operator) models.Finding
	Priority                int
	BookChapter             string
	Tags                    []string
}

func NewRuleEngine() *RuleEngine {
	engine := &RuleEngine{
		rules:        make([]RuleDefinition, 0),
		ruleRegistry: make(map[string]bool),
	}
	engine.registerAllRules()
	return engine
}

func (e *RuleEngine) registerAllRules() {
	e.registerAccessMethodRules()
	e.registerJoinRules()
	e.registerMemoryRules()
	e.registerParallelismRules()
	e.registerPredicateRules()
	e.registerCardinalityRules()
	e.registerIndexRules()
	e.registerBlockingOperatorRules()
	e.registerWarningRules()
}

func (e *RuleEngine) Evaluate(plan *models.PlanAnalysis) []models.Finding {
	findings := make([]models.Finding, 0)

	for _, rule := range e.rules {
		if !e.isRuleEnabled(rule.Name) {
			continue
		}

		for _, op := range plan.Operators {
			if rule.Condition(plan, &op) {
				finding := rule.ExtractFinding(plan, &op)
				if finding.Title != "" {
					finding.RuleName = rule.Name
					finding.Category = rule.Category
					if finding.FindingType == "" {
						finding.FindingType = rule.Category
					}
					findings = append(findings, finding)
				}
			}
		}
	}

	return findings
}

func (e *RuleEngine) isRuleEnabled(name string) bool {
	if enabled, ok := e.ruleRegistry[name]; ok {
		return enabled
	}
	return true
}

func (e *RuleEngine) EnableRule(name string) {
	e.ruleRegistry[name] = true
}

func (e *RuleEngine) DisableRule(name string) {
	e.ruleRegistry[name] = false
}

func (e *RuleEngine) GetRules() []RuleDefinition {
	return e.rules
}

func (e *RuleEngine) GetRulesByCategory(category string) []RuleDefinition {
	result := make([]RuleDefinition, 0)
	for _, rule := range e.rules {
		if rule.Category == category {
			result = append(result, rule)
		}
	}
	return result
}

func (e *RuleEngine) GetRulesBySeverity(severity models.Severity) []RuleDefinition {
	result := make([]RuleDefinition, 0)
	for _, rule := range e.rules {
		if rule.Severity == severity {
			result = append(result, rule)
		}
	}
	return result
}

func (e *RuleEngine) AddRule(rule RuleDefinition) {
	e.rules = append(e.rules, rule)
}

func (e *RuleEngine) GetRuleByName(name string) *RuleDefinition {
	for i := range e.rules {
		if e.rules[i].Name == name {
			return &e.rules[i]
		}
	}
	return nil
}

func (e *RuleEngine) RemoveRule(name string) {
	for i := 0; i < len(e.rules); i++ {
		if e.rules[i].Name == name {
			e.rules = append(e.rules[:i], e.rules[i+1:]...)
			return
		}
	}
}

func makeBaseFinding(op *models.Operator, title, explanation, recommendation, impact string, severity models.Severity) models.Finding {
	finding := models.Finding{
		Severity:             severity,
		OperatorID:          op.ID,
		OperatorName:        op.PhysicalOp,
		Title:               title,
		Explanation:         explanation,
		Recommendation:       recommendation,
		Impact:              impact,
		EstimatedCost:       op.EstimatedTotalSubtreeCost,
		AffectedRows:        op.EstimateRows,
		QueryPlanNode:       op,
		Confidence:          calculateConfidence(op, severity),
		EvidenceTrace:       buildEvidenceTrace(op),
		OperatorIDs:         []int{op.ID},
	}
	if finding.OperatorName == "" {
		finding.OperatorName = op.LogicalOp
	}
	return finding
}

func calculateConfidence(op *models.Operator, severity models.Severity) float64 {
	confidence := 0.3

	if op.ActualRows > 0 || op.ActualExecutions > 0 {
		confidence += 0.3
	}
	if op.ActualCPUms > 0 || op.ActualElapsedms > 0 {
		confidence += 0.2
	}
	if op.ActualLogicalReads > 0 {
		confidence += 0.1
	}
	if len(op.RuntimeCounters) > 0 {
		confidence += 0.1
	}

	if severity == models.SeverityCritical || severity == models.SeverityHigh {
		confidence += 0.1
	}

	if confidence > 1.0 {
		confidence = 1.0
	}
	return confidence
}

func buildEvidenceTrace(op *models.Operator) []models.EvidenceDetail {
	trace := make([]models.EvidenceDetail, 0)

	if op.ActualRows > 0 || op.ActualExecutions > 0 {
		trace = append(trace, models.EvidenceDetail{
			Source:      models.EvidenceRuntimeCounters,
			Value:       "runtime_counters",
			Description: "Actual row and execution counts from runtime counters",
			OperatorIDs: []int{op.ID},
		})
	}

	if op.EstimatedTotalSubtreeCost > 0 {
		trace = append(trace, models.EvidenceDetail{
			Source:      models.EvidenceOperator,
			Value:       "operator_cost",
			Description: "Estimated subtree cost indicates resource usage",
			OperatorIDs: []int{op.ID},
		})
	}

	if op.IndexScan != nil || op.TableScan != nil {
		trace = append(trace, models.EvidenceDetail{
			Source:      models.EvidenceOperator,
			Value:       "access_method",
			Description: "Access method detected from operator scan type",
			OperatorIDs: []int{op.ID},
		})
	}

	return trace
}

func getTableName(op *models.Operator) string {
	if op.TableScan != nil {
		return op.TableScan.Object.Table
	}
	if op.IndexScan != nil {
		return op.IndexScan.Object.Table
	}
	return ""
}

func getIndexName(op *models.Operator) string {
	if op.IndexScan != nil {
		return op.IndexScan.Object.Index
	}
	return ""
}
