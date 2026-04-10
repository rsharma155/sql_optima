package analyzer

import (
	"time"

	"github.com/yourorg/pg_explain_analyze/analyzer/rules"
	"github.com/yourorg/pg_explain_analyze/parser"
	"github.com/yourorg/pg_explain_analyze/types"
)

type Analyzer struct {
	config *types.AnalyzerConfig
	rules  []rules.AnalysisRule
}

func New() *Analyzer {
	return &Analyzer{
		config: &types.DefaultConfig,
		rules:  rules.GetAllRules(),
	}
}

func NewWithConfig(config *types.AnalyzerConfig) *Analyzer {
	return &Analyzer{
		config: config,
		rules:  rules.GetAllRules(),
	}
}

func (a *Analyzer) Analyze(plan *types.Plan) *types.AnalysisResult {
	result := types.NewAnalysisResult(plan.Query, *plan)
	result.ID = generateID()
	result.CreatedAt = time.Now().UTC().Format(time.RFC3339)

	flatNodes := result.FlattenNodes()
	result.Summary.NodeCount = len(flatNodes)

	var scanCount, joinCount int
	for _, node := range flatNodes {
		if node.IsScanNode() {
			scanCount++
		}
		if node.IsJoinNode() {
			joinCount++
		}
	}
	result.Summary.ScanCount = scanCount
	result.Summary.JoinCount = joinCount

	for _, node := range flatNodes {
		for _, rule := range a.rules {
			if finding := rule.Check(&node, plan, a.config); finding != nil {
				finding.NodeID = node.ID
				result.Findings = append(result.Findings, *finding)
			}
		}
	}

	summary := result.Findings.Summary()
	result.Summary.FindingsCount = summary

	return result
}

func (a *Analyzer) AnalyzeFromText(text string) (*types.AnalysisResult, error) {
	plan, err := parser.ParseText(text)
	if err != nil {
		return nil, err
	}
	return a.Analyze(plan), nil
}

func (a *Analyzer) AnalyzeFromJSON(jsonStr string) (*types.AnalysisResult, error) {
	plan, err := parser.ParseJSON(jsonStr)
	if err != nil {
		return nil, err
	}
	return a.Analyze(plan), nil
}

func (a *Analyzer) Compare(plan1, plan2 *types.Plan) *types.PlanDiff {
	diff := &types.PlanDiff{
		ChangedNodes: []types.NodeChange{},
	}

	result1 := a.Analyze(plan1)
	result2 := a.Analyze(plan2)

	diff.Summary.CostChange = result2.Summary.TotalCost - result1.Summary.TotalCost
	diff.Summary.TimeChange = result2.Summary.ExecutionTimeMs - result1.Summary.ExecutionTimeMs

	nodes1 := result1.FlattenNodes()
	nodes2 := result2.FlattenNodes()

	for i := 0; i < len(nodes1) && i < len(nodes2); i++ {
		n1 := nodes1[i]
		n2 := nodes2[i]

		if n1.TotalCost != n2.TotalCost {
			diff.ChangedNodes = append(diff.ChangedNodes, types.NodeChange{
				NodeID:   n1.ID,
				NodeType: n1.NodeType,
				Field:    "TotalCost",
				OldValue: n1.TotalCost,
				NewValue: n2.TotalCost,
			})
		}

		if n1.ActualRows != n2.ActualRows {
			diff.ChangedNodes = append(diff.ChangedNodes, types.NodeChange{
				NodeID:   n1.ID,
				NodeType: n1.NodeType,
				Field:    "ActualRows",
				OldValue: n1.ActualRows,
				NewValue: n2.ActualRows,
			})
		}

		if n1.ActualTotalTime != n2.ActualTotalTime {
			diff.ChangedNodes = append(diff.ChangedNodes, types.NodeChange{
				NodeID:   n1.ID,
				NodeType: n1.NodeType,
				Field:    "ActualTotalTime",
				OldValue: n1.ActualTotalTime,
				NewValue: n2.ActualTotalTime,
			})
		}
	}

	diff.Summary.TotalChanges = len(diff.ChangedNodes)
	diff.Summary.TotalChanges += len(diff.AddedNodes)
	diff.Summary.TotalChanges += len(diff.RemovedNodes)

	return diff
}

func generateID() string {
	return time.Now().UTC().Format("20060102150405")
}
