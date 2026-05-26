// File: internal/sqlplan_rules/engine.go
// Purpose: Comprehensive rule engine based on Grant Fritchey SQL Server Execution Plans book
// Package: github.com/rsharma155/sqlplan-analyzer/rules
package rules

import (
	"fmt"
	"strings"

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
	e.registerAdvancedPlanRules()
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

func (e *RuleEngine) registerAdvancedPlanRules() {
	// Key Lookup Hotspot — lookup > 30% of total plan cost
	e.rules = append(e.rules, RuleDefinition{
		Name:        "KeyLookupHotspot",
		Description: "Key or RID Lookup consuming >30% of total plan cost",
		Category:    "AccessMethods",
		Severity:    models.SeverityCritical,
		RuleType:    RuleTypeAccessPath,
		Condition: func(plan *models.PlanAnalysis, op *models.Operator) bool {
			if plan.CostSummary.TotalEstimatedCost <= 0 { return false }
			isLookup := strings.Contains(op.PhysicalOp, "Key Lookup") || strings.Contains(op.PhysicalOp, "RID Lookup")
			if !isLookup { return false }
			return op.EstimatedTotalSubtreeCost/plan.CostSummary.TotalEstimatedCost >= 0.30
		},
		ExtractFinding: func(plan *models.PlanAnalysis, op *models.Operator) models.Finding {
			pct := op.EstimatedTotalSubtreeCost / plan.CostSummary.TotalEstimatedCost * 100
			tbl := getTableName(op)
			return makeBaseFinding(op,
				fmt.Sprintf("Key Lookup hotspot on %s (%.0f%% of plan cost)", tbl, pct),
				fmt.Sprintf("A %s consumes %.1f%% of total plan cost. Each qualifying row from the index seek triggers a separate clustered-index row fetch.", op.PhysicalOp, pct),
				fmt.Sprintf("Add a covering index or INCLUDE columns so the nonclustered index satisfies all output columns, eliminating the lookup. CREATE NONCLUSTERED INDEX on %s ... INCLUDE (...)", tbl),
				"High I/O per qualifying row; scales linearly with row count",
				models.SeverityCritical,
			)
		},
		Priority:    1,
		BookChapter: "Key Lookup / Bookmark Lookup",
		Tags:        []string{"key-lookup", "hotspot", "covering-index"},
	})

	// Excessive Rewinds — nested-loop inner side re-executed too many times
	e.rules = append(e.rules, RuleDefinition{
		Name:        "ExcessiveRewinds",
		Description: "Operator is rewound (re-executed) more than 100 times",
		Category:    "Join",
		Severity:    models.SeverityHigh,
		RuleType:    RuleTypeJoin,
		Condition: func(plan *models.PlanAnalysis, op *models.Operator) bool {
			return op.EstimateRewinds > 100 && op.EstimatedTotalSubtreeCost > 0.5
		},
		ExtractFinding: func(plan *models.PlanAnalysis, op *models.Operator) models.Finding {
			return makeBaseFinding(op,
				fmt.Sprintf("Excessive rewinds on %s (%d rewinds)", op.PhysicalOp, op.EstimateRewinds),
				fmt.Sprintf("Operator %s is rewound %d times. Each rewind re-executes the entire subtree, multiplying its cost by the outer-loop iteration count.", op.PhysicalOp, op.EstimateRewinds),
				"Consider a Lazy Spool to cache the inner result set, or rewrite the query to eliminate correlated sub-expressions driving the nested loop.",
				fmt.Sprintf("Effective subtree cost ≈ %.2f × %d = %.0f (amplified by rewinds)", op.EstimatedTotalSubtreeCost, op.EstimateRewinds, op.EstimatedTotalSubtreeCost*float64(op.EstimateRewinds)),
				models.SeverityHigh,
			)
		},
		Priority:    2,
		BookChapter: "Nested Loops / Rewinds",
		Tags:        []string{"rewinds", "nested-loops", "correlated"},
	})

	// Parameter Sniffing — compiled and runtime parameter values differ
	e.rules = append(e.rules, RuleDefinition{
		Name:        "ParameterSniffingMismatch",
		Description: "Compiled parameter values differ from runtime values",
		Category:    "Estimation",
		Severity:    models.SeverityCritical,
		RuleType:    RuleTypeEstimation,
		Condition: func(plan *models.PlanAnalysis, op *models.Operator) bool {
			if len(plan.CompiledParameters) == 0 || len(plan.RuntimeParameters) == 0 { return false }
			// Only fire on root operator (op.ID == 1) to emit once per plan
			if op.ID != 1 { return false }
			compiled := make(map[string]string, len(plan.CompiledParameters))
			for _, p := range plan.CompiledParameters { compiled[p.Parameter] = p.Value }
			for _, p := range plan.RuntimeParameters {
				if cv, ok := compiled[p.Parameter]; ok && cv != p.Value { return true }
			}
			return false
		},
		ExtractFinding: func(plan *models.PlanAnalysis, op *models.Operator) models.Finding {
			mismatches := []string{}
			compiled := make(map[string]string, len(plan.CompiledParameters))
			for _, p := range plan.CompiledParameters { compiled[p.Parameter] = p.Value }
			for _, p := range plan.RuntimeParameters {
				if cv, ok := compiled[p.Parameter]; ok && cv != p.Value {
					mismatches = append(mismatches, fmt.Sprintf("%s: compiled=%s runtime=%s", p.Parameter, cv, p.Value))
				}
			}
			return makeBaseFinding(op,
				fmt.Sprintf("Parameter sniffing: %d parameter(s) compiled with different values than runtime", len(mismatches)),
				fmt.Sprintf("The plan was compiled for parameter values that differ from the current execution: %s. The optimizer chose a plan optimal for the compiled values, which may be suboptimal now.", strings.Join(mismatches, "; ")),
				"Add OPTION (OPTIMIZE FOR UNKNOWN) or OPTION (RECOMPILE) to force per-execution optimization. Consider plan guides for stable parameters.",
				"Query may run slower than expected; cardinality estimates are based on wrong parameter values",
				models.SeverityCritical,
			)
		},
		Priority:    1,
		BookChapter: "Parameter Sniffing",
		Tags:        []string{"parameter-sniffing", "recompile", "plan-cache"},
	})

	// Forced serial plan with large estimated row count
	e.rules = append(e.rules, RuleDefinition{
		Name:        "ForcedSerialLargeRowset",
		Description: "Plan is forced serial (non-parallel) but has large estimated rows",
		Category:    "Parallelism",
		Severity:    models.SeverityMedium,
		RuleType:    RuleTypeParallelism,
		Condition: func(plan *models.PlanAnalysis, op *models.Operator) bool {
			if plan.QueryPlan == nil { return false }
			reason := plan.QueryPlan.NonParallelPlanReason
			if reason == "" || reason == "MaxDOPSetToOne" { return false }
			// Fire on root operator only
			if op.ID != 1 { return false }
			return op.EstimateRows > 5_000_000
		},
		ExtractFinding: func(plan *models.PlanAnalysis, op *models.Operator) models.Finding {
			reason := ""
			if plan.QueryPlan != nil { reason = plan.QueryPlan.NonParallelPlanReason }
			return makeBaseFinding(op,
				fmt.Sprintf("Serial plan forced (%s) with %.0fM estimated rows", reason, float64(op.EstimateRows)/1_000_000),
				fmt.Sprintf("The optimizer chose a serial plan (reason: %s) despite %.0fM estimated rows. Parallelism could significantly reduce elapsed time on a multi-core server.", reason, float64(op.EstimateRows)/1_000_000),
				"Investigate the reason for non-parallel execution. Common causes: MAXDOP hint, non-parallelizable operator (scalar UDF, ROWLOCK hint), or small-table heuristic. Remove the constraint if query latency is critical.",
				"Query runs single-threaded; elapsed time not reduced by additional CPU cores",
				models.SeverityMedium,
			)
		},
		Priority:    3,
		BookChapter: "Parallelism / NonParallelPlanReason",
		Tags:        []string{"parallelism", "serial", "maxdop"},
	})
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
		AffectedRows:        int64(op.EstimateRows),
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
