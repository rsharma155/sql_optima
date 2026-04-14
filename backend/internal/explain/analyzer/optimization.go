// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Query optimization suggestions from plan analysis.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package analyzer

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/rsharma155/sql_optima/internal/explain/types"
)

type OptimizationAnalyzer struct {
	config *types.AnalyzerConfig
}

func NewOptimizationAnalyzer() *OptimizationAnalyzer {
	return &OptimizationAnalyzer{
		config: &types.DefaultConfig,
	}
}

func (a *OptimizationAnalyzer) GenerateOptimizationReport(plan *types.Plan, findings []types.Finding) *types.OptimizationReport {
	flatNodes := a.flattenNodes(&plan.Plan)

	report := &types.OptimizationReport{
		ExecutiveSummary:    a.generateExecutiveSummary(plan, findings, flatNodes),
		PerformanceScore:    a.calculatePerformanceScore(plan, findings, flatNodes),
		TopIssues:           a.identifyTopIssues(findings, flatNodes),
		Recommendations:     a.generateRecommendations(plan, findings, flatNodes),
		PlanAnalysis:        a.analyzePlan(plan, flatNodes),
		AdvancedDiagnostics: a.analyzeAdvancedDiagnostics(plan, flatNodes),
		IndexAnalysis:       a.analyzeIndexes(plan, flatNodes),
		QueryImprovements:   a.analyzeQueryImprovements(plan.Query),
		ConfidenceScore:     a.calculateConfidenceScore(plan, flatNodes),
	}

	return report
}

func (a *OptimizationAnalyzer) flattenNodes(node *types.PlanNode) []types.PlanNode {
	var nodes []types.PlanNode
	nodes = append(nodes, *node)

	for i := range node.Plans {
		nodes = append(nodes, a.flattenNodes(&node.Plans[i])...)
	}

	return nodes
}

func (a *OptimizationAnalyzer) generateExecutiveSummary(plan *types.Plan, findings []types.Finding, nodes []types.PlanNode) types.ExecutiveSummary {
	execTime := plan.ExecutionTime
	var primaryBottleneck string
	var secondaryIssues string
	var overallDiagnosis string

	slowestNode := a.findSlowestNode(nodes)
	if slowestNode != nil {
		primaryBottleneck = fmt.Sprintf("%s on %s (%.2fms)", slowestNode.NodeType, slowestNode.RelationName, slowestNode.ActualTotalTime)
	}

	criticalCount := 0
	highCount := 0
	for _, f := range findings {
		if f.Severity == types.SeverityCritical {
			criticalCount++
		} else if f.Severity == types.SeverityHigh {
			highCount++
		}
	}

	if criticalCount > 0 {
		secondaryIssues = fmt.Sprintf("%d critical, %d high severity issues", criticalCount, highCount)
	} else if highCount > 0 {
		secondaryIssues = fmt.Sprintf("%d high severity issues", highCount)
	} else {
		secondaryIssues = "No critical issues detected"
	}

	if execTime < 10 {
		overallDiagnosis = "Query executes quickly with minimal optimization needed"
	} else if execTime < 100 {
		overallDiagnosis = "Moderate execution time with room for improvement"
	} else {
		overallDiagnosis = "Slow execution requiring immediate optimization attention"
	}

	return types.ExecutiveSummary{
		TotalExecutionTime: fmt.Sprintf("%.2fms", execTime),
		PrimaryBottleneck:  primaryBottleneck,
		SecondaryIssues:    secondaryIssues,
		OverallDiagnosis:   overallDiagnosis,
	}
}

func (a *OptimizationAnalyzer) calculatePerformanceScore(plan *types.Plan, findings []types.Finding, nodes []types.PlanNode) types.PerformanceScore {
	score := 100

	executionPenalty := math.Min(40, plan.ExecutionTime/10)
	score -= int(executionPenalty)

	costPenalty := math.Min(20, plan.Plan.TotalCost/1000)
	score -= int(costPenalty)

	criticalPenalty := 0
	highPenalty := 0
	mediumPenalty := 0

	for _, f := range findings {
		switch f.Severity {
		case types.SeverityCritical:
			criticalPenalty += 15
		case types.SeverityHigh:
			highPenalty += 8
		case types.SeverityMedium:
			mediumPenalty += 3
		}
	}

	score -= criticalPenalty
	score -= highPenalty
	score -= mediumPenalty

	rowMismatchCount := a.countRowMismatches(nodes)
	score -= rowMismatchCount * 5

	if score < 0 {
		score = 0
	}

	var label string
	switch {
	case score >= 80:
		label = "good"
	case score >= 50:
		label = "moderate"
	default:
		label = "poor"
	}

	return types.PerformanceScore{
		Score: score,
		Label: label,
	}
}

func (a *OptimizationAnalyzer) identifyTopIssues(findings []types.Finding, nodes []types.PlanNode) []types.TopIssue {
	var issues []types.TopIssue

	severityOrder := map[types.Severity]int{
		types.SeverityCritical: 4,
		types.SeverityHigh:     3,
		types.SeverityMedium:   2,
		types.SeverityLow:      1,
		types.SeverityInfo:     0,
	}

	sort.Slice(findings, func(i, j int) bool {
		return severityOrder[findings[i].Severity] > severityOrder[findings[j].Severity]
	})

	addedIssues := make(map[string]bool)
	for _, f := range findings {
		if len(issues) >= 5 {
			break
		}

		title := f.Message
		evidence := fmt.Sprintf("Node: %s", f.NodeType)

		if f.RelationName != "" {
			evidence += fmt.Sprintf(" | Table: %s", f.RelationName)
		}
		if f.NodeType != "" {
			evidence += fmt.Sprintf(" | Operation: %s", f.NodeType)
		}
		if f.ActualValue != nil {
			evidence += fmt.Sprintf(" | Value: %v", f.ActualValue)
		}

		impact := f.Suggestion
		switch f.Type {
		case "SequentialScanWarning":
			rowsVal := f.ActualValue
			var rows int
			switch v := rowsVal.(type) {
			case float64:
				rows = int(v)
			case int:
				rows = v
			default:
				rows = 0
			}
			title = fmt.Sprintf("Sequential scan on table '%s' - %d rows", f.RelationName, rows)
			impact = fmt.Sprintf("Add index on '%s' to use index scan instead. This will significantly reduce I/O and improve query speed.", f.RelationName)
		case "RowEstimationMismatch":
			title = fmt.Sprintf("Row count misestimated for '%s'", f.RelationName)
			impact = fmt.Sprintf("Run ANALYZE on '%s' to update statistics. Planner estimated %v rows but actual was %v.", f.RelationName, f.ExpectedValue, f.ActualValue)
		case "HashSpill":
			title = "Hash operation spilled to disk"
			impact = "Increase work_mem parameter. Disk spills cause 10-100x slower performance compared to in-memory hash operations."
		case "SortSpill":
			title = "Sort operation spilled to disk"
			impact = "Increase work_mem to keep sort in memory. On-disk sorting is extremely slow for large datasets."
		case "MissingIndex":
			title = fmt.Sprintf("Missing index on '%s' for filter", f.RelationName)
			impact = fmt.Sprintf("Create index on filtered column(s) in '%s'. Currently using slow sequential scan.", f.RelationName)
		case "NestedLoopIssue":
			rowsVal := f.ActualValue
			var rows int
			switch v := rowsVal.(type) {
			case float64:
				rows = int(v)
			case int:
				rows = v
			default:
				rows = 0
			}
			title = fmt.Sprintf("Nested loop with high row count: %d rows", rows)
			impact = "Consider using hash join or increase work_mem. Nested loops are inefficient for large datasets."
		case "SlowScan":
			var msPerRow float64
			switch v := f.ActualValue.(type) {
			case float64:
				msPerRow = v
			case int:
				msPerRow = float64(v)
			default:
				msPerRow = 0
			}
			title = fmt.Sprintf("Slow scan on '%s': %.2fms per row", f.RelationName, msPerRow)
			impact = fmt.Sprintf("Add index on '%s' to speed up this scan. Current scan takes %.2fms per row which is very slow.", f.RelationName, msPerRow)
		case "HighCost":
			var cost float64
			switch v := f.ActualValue.(type) {
			case float64:
				cost = v
			case int:
				cost = float64(v)
			default:
				cost = 0
			}
			title = fmt.Sprintf("High cost node: %.2f on '%s'", cost, f.RelationName)
			impact = fmt.Sprintf("Review and optimize this %s operation on '%s'. High cost indicates significant resource usage.", f.NodeType, f.RelationName)
		case "RowsRemovedByFilter":
			rowsVal := f.ActualValue
			var rows int
			switch v := rowsVal.(type) {
			case float64:
				rows = int(v)
			case int:
				rows = v
			default:
				rows = 0
			}
			title = fmt.Sprintf("Removed %d rows by filter on '%s'", rows, f.RelationName)
			impact = "Create an index on filtered column to filter rows earlier in the plan and reduce processed rows before scanning."
		}

		issue := types.TopIssue{
			Title:       title,
			Severity:    string(f.Severity),
			Description: f.Message,
			Evidence:    evidence,
			Impact:      impact,
		}

		key := f.Type + f.Message
		if !addedIssues[key] {
			issues = append(issues, issue)
			addedIssues[key] = true
		}
	}

	return issues
}

func (a *OptimizationAnalyzer) generateRecommendations(plan *types.Plan, findings []types.Finding, nodes []types.PlanNode) []types.Recommendation {
	var recommendations []types.Recommendation
	added := make(map[string]bool)

	for _, f := range findings {
		key := f.Type + f.RelationName
		if added[key] {
			continue
		}
		added[key] = true

		rec := types.Recommendation{
			IssueTitle:  f.Type,
			Action:      f.Suggestion,
			Explanation: f.Message,
		}

		switch f.Type {
		case "SequentialScanWarning":
			rec.Action = fmt.Sprintf("Create an index on table '%s' to replace sequential scan", f.RelationName)
			rec.SQLExample = fmt.Sprintf("CREATE INDEX idx_%s_filter ON %s (column);", f.RelationName, f.RelationName)
			rec.ExpectedImpact = "Will use index scan instead of sequential scan, reducing I/O and improving query speed by 50-90%"
		case "RowEstimationMismatch":
			rec.Action = fmt.Sprintf("Run ANALYZE on table '%s' to update statistics", f.RelationName)
			rec.SQLExample = fmt.Sprintf("ANALYZE %s;", f.RelationName)
			rec.ExpectedImpact = "Improves query planner accuracy, leading to better execution plans and faster queries"
		case "HashSpill":
			rec.Action = "Increase work_mem to keep hash operations in memory"
			rec.SQLExample = "SET work_mem = '256MB'; -- or higher for large hash joins"
			rec.ExpectedImpact = "Eliminates disk spills, significantly faster hash joins (2-10x improvement)"
		case "SortSpill":
			rec.Action = "Increase work_mem or optimize sort operations"
			rec.SQLExample = "SET work_mem = '256MB'; -- increase for large sorts"
			rec.ExpectedImpact = "Keeps sort in memory, dramatically improves ORDER BY and GROUP BY performance"
		case "MissingIndex":
			filterCols := a.extractColumns(f.Suggestion)
			if len(filterCols) > 0 {
				rec.Action = fmt.Sprintf("Create index on table '%s' columns: %s", f.RelationName, strings.Join(filterCols, ", "))
				rec.SQLExample = fmt.Sprintf("CREATE INDEX idx_%s_%s ON %s (%s);", f.RelationName, strings.Join(filterCols, "_"), f.RelationName, strings.Join(filterCols, ", "))
			} else {
				rec.Action = f.Suggestion
				rec.SQLExample = fmt.Sprintf("CREATE INDEX ON %s (your_filter_column);", f.RelationName)
			}
			rec.ExpectedImpact = "Allows index scan instead of sequential scan, typically 10-100x faster for filtered queries"
		case "NestedLoopIssue":
			rec.Action = "Consider using hash join or increase work_mem for nested loop optimization"
			rec.SQLExample = "SET work_mem = '256MB'; -- or rewrite query to use hash join"
			rec.ExpectedImpact = "Reduces iteration overhead, improves join performance by 5-50x for large datasets"
		case "SlowScan":
			rec.Action = fmt.Sprintf("Add index on table '%s' to speed up scan", f.RelationName)
			rec.SQLExample = fmt.Sprintf("CREATE INDEX ON %s (your_filter_column);", f.RelationName)
			rec.ExpectedImpact = "Reduces per-row scan time from milliseconds to microseconds"
		case "HighCost":
			rec.Action = fmt.Sprintf("Optimize high-cost node: %s on table '%s'", f.NodeType, f.RelationName)
			rec.SQLExample = "-- Review and optimize the query or add appropriate indexes"
			rec.ExpectedImpact = "Lower overall query cost leads to faster execution and reduced resource usage"
		case "RowsRemovedByFilter":
			rec.Action = fmt.Sprintf("Create index on '%s' to filter rows earlier in the plan", f.RelationName)
			rec.SQLExample = fmt.Sprintf("CREATE INDEX ON %s (filtered_column);", f.RelationName)
			rec.ExpectedImpact = "Filters rows before scan, reducing I/O and memory usage significantly"
		case "BitmapTooManyRows":
			rec.Action = "Improve index selectivity or increase work_mem for bitmap operations"
			rec.SQLExample = "SET work_mem = '128MB'; -- or create better targeted indexes"
			rec.ExpectedImpact = "Reduces lossy bitmap pages, improves bitmap heap scan efficiency"
		case "ParallelDisabled":
			rec.Action = fmt.Sprintf("Enable parallel query for large sequential scan on '%s'", f.RelationName)
			rec.SQLExample = "SET max_parallel_workers_per_gather = 4; -- enable parallel queries"
			rec.ExpectedImpact = "Uses multiple CPU cores, can improve large scan performance by 2-4x"
		case "ManyLoops":
			if loops, ok := f.ActualValue.(float64); ok {
				rec.Action = fmt.Sprintf("Optimize node '%s' that loops %d times - consider better indexing or join order", f.NodeType, int(loops))
			} else {
				rec.Action = fmt.Sprintf("Optimize node '%s' that loops many times - consider better indexing or join order", f.NodeType)
			}
			rec.SQLExample = "-- Rewrite query or add indexes to reduce loop iterations"
			rec.ExpectedImpact = "Fewer loop iterations mean faster execution and less CPU usage"
		default:
			rec.Action = f.Suggestion
		}

		recommendations = append(recommendations, rec)
	}

	return recommendations
}

func (a *OptimizationAnalyzer) analyzePlan(plan *types.Plan, nodes []types.PlanNode) types.PlanAnalysis {
	planAnalysis := types.PlanAnalysis{
		KeyNodes:         []types.KeyNode{},
		TimePerNode:      make(map[string]float64),
		InefficientSteps: []string{},
	}

	totalTime := plan.ExecutionTime
	if totalTime == 0 {
		totalTime = 1
	}

	for _, node := range nodes {
		if node.ActualTotalTime > 0 {
			key := node.NodeType
			if node.RelationName != "" {
				key = node.NodeType + ":" + node.RelationName
			}

			planAnalysis.TimePerNode[key] = node.ActualTotalTime

			timePercent := (node.ActualTotalTime / totalTime) * 100
			if timePercent > 5 {
				planAnalysis.KeyNodes = append(planAnalysis.KeyNodes, types.KeyNode{
					NodeType:      node.NodeType,
					RelationName:  node.RelationName,
					TimePercent:   timePercent,
					RowsProcessed: node.ActualRows,
					RowsEstimated: node.PlanRows,
				})
			}

			if node.ActualTotalTime > 50 && node.NodeType == "Seq Scan" {
				planAnalysis.InefficientSteps = append(planAnalysis.InefficientSteps,
					fmt.Sprintf("Slow sequential scan on %s (%.2fms)", node.RelationName, node.ActualTotalTime))
			}
		}
	}

	sort.Slice(planAnalysis.KeyNodes, func(i, j int) bool {
		return planAnalysis.KeyNodes[i].TimePercent > planAnalysis.KeyNodes[j].TimePercent
	})

	return planAnalysis
}

func (a *OptimizationAnalyzer) analyzeAdvancedDiagnostics(plan *types.Plan, nodes []types.PlanNode) types.AdvancedDiagnostics {
	return types.AdvancedDiagnostics{
		RowEstimationAccuracy: a.analyzeRowEstimation(nodes),
		JoinStrategyAnalysis:  a.analyzeJoinStrategy(nodes),
		MemoryDiskUsage:       a.analyzeMemoryDisk(nodes),
		ParallelismAnalysis:   a.analyzeParallelism(plan, nodes),
		IOvsCPUBound:          a.analyzeIOvsCPU(nodes),
	}
}

func (a *OptimizationAnalyzer) analyzeRowEstimation(nodes []types.PlanNode) types.RowEstimationAccuracy {
	accuracy := types.RowEstimationAccuracy{
		Mismatches: []types.RowMismatch{},
	}

	for _, node := range nodes {
		if node.ActualRows > 0 && node.PlanRows > 0 {
			ratio := float64(node.ActualRows) / float64(node.PlanRows)
			if ratio > 10 || ratio < 0.1 {
				accuracy.Mismatches = append(accuracy.Mismatches, types.RowMismatch{
					NodeType:     node.NodeType,
					RelationName: node.RelationName,
					Estimated:    node.PlanRows,
					Actual:       node.ActualRows,
					Ratio:        ratio,
				})
			}
		}
	}

	return accuracy
}

func (a *OptimizationAnalyzer) analyzeJoinStrategy(nodes []types.PlanNode) types.JoinStrategyAnalysis {
	analysis := types.JoinStrategyAnalysis{
		Nodes: []types.JoinAnalysisNode{},
	}

	for _, node := range nodes {
		if node.IsJoinNode() {
			isAppropriate := true
			suggestion := ""

			if strings.Contains(node.NodeType, "Nested Loop") && node.ActualRows > 10000 {
				isAppropriate = false
				suggestion = "Consider hash join or merge join for large datasets"
			}

			analysis.Nodes = append(analysis.Nodes, types.JoinAnalysisNode{
				NodeType:      node.NodeType,
				JoinType:      node.JoinType,
				IsAppropriate: isAppropriate,
				Suggestion:    suggestion,
			})
		}
	}

	return analysis
}

func (a *OptimizationAnalyzer) analyzeMemoryDisk(nodes []types.PlanNode) types.MemoryDiskUsage {
	usage := types.MemoryDiskUsage{
		HashSpills: []types.DiskSpill{},
		SortSpills: []types.DiskSpill{},
	}

	for _, node := range nodes {
		if node.Buffers != nil && (node.Buffers.TempRead > 0 || node.Buffers.TempWritten > 0) {
			if strings.Contains(node.NodeType, "Hash") {
				usage.HashSpills = append(usage.HashSpills, types.DiskSpill{
					NodeType:   node.NodeType,
					MemoryUsed: node.MemoryUsage,
					DiskUsed:   node.Buffers.TempRead + node.Buffers.TempWritten,
					Suggestion: "Increase work_mem",
				})
			}

			if strings.Contains(node.NodeType, "Sort") {
				usage.SortSpills = append(usage.SortSpills, types.DiskSpill{
					NodeType:   node.NodeType,
					MemoryUsed: node.MemoryUsage,
					DiskUsed:   node.Buffers.TempRead + node.Buffers.TempWritten,
					Suggestion: "Increase work_mem or optimize sort key",
				})
			}
		}
	}

	return usage
}

func (a *OptimizationAnalyzer) analyzeParallelism(plan *types.Plan, nodes []types.PlanNode) types.ParallelismAnalysis {
	analysis := types.ParallelismAnalysis{
		Used: false,
	}

	for _, node := range nodes {
		if node.WorkersLaunched > 0 {
			analysis.Used = true
			analysis.WorkersLaunched = node.WorkersLaunched
			break
		}
	}

	hasLargeSeqScan := false
	for _, node := range nodes {
		if node.NodeType == "Seq Scan" && node.PlanRows > 10000 {
			hasLargeSeqScan = true
			break
		}
	}

	if hasLargeSeqScan && !analysis.Used {
		analysis.ShouldUse = true
		analysis.Suggestion = "Enable parallel query for large sequential scans"
	}

	return analysis
}

func (a *OptimizationAnalyzer) analyzeIOvsCPU(nodes []types.PlanNode) types.IOvsCPUBound {
	analysis := types.IOvsCPUBound{
		IsIOBound: false,
	}

	totalBufferHit := 0
	totalBufferRead := 0

	for _, node := range nodes {
		if node.Buffers != nil {
			totalBufferHit += node.Buffers.SharedHit
			totalBufferRead += node.Buffers.SharedRead
		}
	}

	totalIO := totalBufferHit + totalBufferRead
	if totalIO > 0 {
		readRatio := float64(totalBufferRead) / float64(totalIO)
		if readRatio > 0.5 {
			analysis.IsIOBound = true
			analysis.Reason = "High buffer read ratio (>50%)"
			analysis.Evidence = fmt.Sprintf("Shared hit: %d, Shared read: %d", totalBufferHit, totalBufferRead)
		} else {
			analysis.Reason = "Buffers mostly cached"
			analysis.Evidence = fmt.Sprintf("Shared hit: %d, Shared read: %d", totalBufferHit, totalBufferRead)
		}
	} else {
		analysis.Reason = "No buffer metrics available"
		analysis.Evidence = "Analysis based on node types"
	}

	return analysis
}

func (a *OptimizationAnalyzer) analyzeIndexes(plan *types.Plan, nodes []types.PlanNode) types.IndexAnalysis {
	analysis := types.IndexAnalysis{
		MissingIndexes:     []types.IndexSuggestion{},
		InefficientIndexes: []types.IndexSuggestion{},
	}

	for _, node := range nodes {
		if node.NodeType == "Seq Scan" && node.PlanRows > 1000 {
			if node.Filter != "" {
				analysis.MissingIndexes = append(analysis.MissingIndexes, types.IndexSuggestion{
					Table:   node.RelationName,
					Columns: a.extractColumns(node.Filter),
					Reason:  "Sequential scan with filter",
					SQL:     fmt.Sprintf("CREATE INDEX ON %s (%s);", node.RelationName, strings.Join(a.extractColumns(node.Filter), ", ")),
				})
			}
		}

		if node.NodeType == "Index Scan" && node.ActualTotalTime > 10 {
			analysis.InefficientIndexes = append(analysis.InefficientIndexes, types.IndexSuggestion{
				Table:   node.RelationName,
				Columns: []string{node.IndexName},
				Reason:  "Slow index scan",
				SQL:     "Consider covering index or composite index",
			})
		}
	}

	return analysis
}

func (a *OptimizationAnalyzer) analyzeQueryImprovements(query string) []types.QueryImprovement {
	var improvements []types.QueryImprovement

	if query == "" {
		return improvements
	}

	upperQuery := strings.ToUpper(query)

	if strings.Contains(upperQuery, "SELECT *") {
		improvements = append(improvements, types.QueryImprovement{
			Type:        "SelectStar",
			Description: "Using SELECT * fetches all columns",
			Suggestion:  "Specify only needed columns",
			SQLExample:  "SELECT column1, column2 FROM table",
		})
	}

	if strings.Contains(upperQuery, "LIMIT") && strings.Contains(upperQuery, "OFFSET") {
		improvements = append(improvements, types.QueryImprovement{
			Type:        "OffsetPagination",
			Description: "OFFSET pagination can be slow for large offsets",
			Suggestion:  "Use keyset pagination (WHERE id > last_id)",
			SQLExample:  "SELECT * FROM table WHERE id > :last_id ORDER BY id LIMIT 10",
		})
	}

	joinCount := strings.Count(upperQuery, " JOIN ")
	if joinCount > 3 {
		improvements = append(improvements, types.QueryImprovement{
			Type:        "TooManyJoins",
			Description: fmt.Sprintf("Query has %d joins", joinCount),
			Suggestion:  "Consider denormalization or materialized views",
		})
	}

	if strings.Contains(upperQuery, "ORDER BY RAND()") {
		improvements = append(improvements, types.QueryImprovement{
			Type:        "RandomOrder",
			Description: "ORDER BY RAND() is inefficient",
			Suggestion:  "Use application-level randomization or alternative approach",
		})
	}

	subqueryCount := strings.Count(upperQuery, "(SELECT")
	if subqueryCount > 2 {
		improvements = append(improvements, types.QueryImprovement{
			Type:        "TooManySubqueries",
			Description: fmt.Sprintf("Query has %d subqueries", subqueryCount),
			Suggestion:  "Consider using CTEs or restructuring query",
		})
	}

	return improvements
}

func (a *OptimizationAnalyzer) calculateConfidenceScore(plan *types.Plan, nodes []types.PlanNode) types.ConfidenceScore {
	score := types.ConfidenceScore{
		Level:   "high",
		Factors: []string{},
	}

	hasActualMetrics := false
	for _, node := range nodes {
		if node.ActualTotalTime > 0 || node.ActualRows > 0 {
			hasActualMetrics = true
			break
		}
	}

	if !hasActualMetrics {
		score.Level = "low"
		score.Factors = append(score.Factors, "No actual execution metrics available")
		score.Factors = append(score.Factors, "Run EXPLAIN ANALYZE for accurate results")
		return score
	}

	rowMismatchCount := a.countRowMismatches(nodes)
	if rowMismatchCount > 3 {
		score.Level = "medium"
		score.Factors = append(score.Factors, fmt.Sprintf("%d row estimation mismatches detected", rowMismatchCount))
	}

	score.Factors = append(score.Factors, "Actual execution metrics available")

	if plan.ExecutionTime > 0 {
		score.Factors = append(score.Factors, "Execution time measured")
	}

	if len(nodes) > 20 {
		if score.Level == "high" {
			score.Level = "medium"
		}
		score.Factors = append(score.Factors, "Complex query plan may have edge cases")
	}

	return score
}

func (a *OptimizationAnalyzer) findSlowestNode(nodes []types.PlanNode) *types.PlanNode {
	var slowest *types.PlanNode
	maxTime := 0.0

	for i := range nodes {
		if nodes[i].ActualTotalTime > maxTime {
			maxTime = nodes[i].ActualTotalTime
			slowest = &nodes[i]
		}
	}

	return slowest
}

func (a *OptimizationAnalyzer) countRowMismatches(nodes []types.PlanNode) int {
	count := 0
	for _, node := range nodes {
		if node.ActualRows > 0 && node.PlanRows > 0 {
			ratio := float64(node.ActualRows) / float64(node.PlanRows)
			if ratio > 10 || ratio < 0.1 {
				count++
			}
		}
	}
	return count
}

func (a *OptimizationAnalyzer) extractColumns(filter string) []string {
	var columns []string
	parts := strings.Split(filter, " AND ")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		ops := []string{"=", "<", ">", "<=", ">=", "<>", "!="}
		for _, op := range ops {
			if idx := strings.Index(part, op); idx > 0 {
				col := strings.TrimSpace(part[:idx])
				col = strings.Trim(col, "\"")
				if col != "" {
					columns = append(columns, col)
				}
				break
			}
		}
	}
	return columns
}
