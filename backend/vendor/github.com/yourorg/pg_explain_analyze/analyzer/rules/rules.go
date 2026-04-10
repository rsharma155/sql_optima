package rules

import (
	"fmt"
	"strings"

	"github.com/yourorg/pg_explain_analyze/types"
)

type AnalysisRule interface {
	Name() string
	Severity() types.Severity
	Check(node *types.PlanNode, plan *types.Plan, config *types.AnalyzerConfig) *types.Finding
}

type SequentialScanRule struct{}

func (r *SequentialScanRule) Name() string {
	return "SequentialScanWarning"
}

func (r *SequentialScanRule) Severity() types.Severity {
	return types.SeverityHigh
}

func (r *SequentialScanRule) Check(node *types.PlanNode, plan *types.Plan, config *types.AnalyzerConfig) *types.Finding {
	if node.NodeType != "Seq Scan" {
		return nil
	}

	threshold := config.SequentialScanThreshold
	if threshold == 0 {
		threshold = 10000
	}

	if node.PlanRows > threshold {
		return &types.Finding{
			Type:         r.Name(),
			Severity:     r.Severity(),
			Message:      fmt.Sprintf("Sequential scan on table '%s' with %d rows", node.RelationName, node.PlanRows),
			NodeType:     node.NodeType,
			RelationName: node.RelationName,
			Suggestion:   fmt.Sprintf("Consider adding an index on '%s' for better performance", node.RelationName),
			ActualValue:  node.PlanRows,
		}
	}

	if node.Filter != "" {
		return &types.Finding{
			Type:         r.Name(),
			Severity:     types.SeverityMedium,
			Message:      fmt.Sprintf("Sequential scan with filter on table '%s'", node.RelationName),
			NodeType:     node.NodeType,
			RelationName: node.RelationName,
			Suggestion:   "Create an index on the filtered column(s) to avoid sequential scan",
		}
	}

	return nil
}

type RowEstimationMismatchRule struct{}

func (r *RowEstimationMismatchRule) Name() string {
	return "RowEstimationMismatch"
}

func (r *RowEstimationMismatchRule) Severity() types.Severity {
	return types.SeverityHigh
}

func (r *RowEstimationMismatchRule) Check(node *types.PlanNode, plan *types.Plan, config *types.AnalyzerConfig) *types.Finding {
	if node.ActualRows == 0 || node.PlanRows == 0 {
		return nil
	}

	threshold := config.RowMismatchThreshold
	if threshold == 0 {
		threshold = 10.0
	}

	ratio := float64(node.ActualRows) / float64(node.PlanRows)

	if ratio > threshold {
		return &types.Finding{
			Type:          r.Name(),
			Severity:      r.Severity(),
			Message:       fmt.Sprintf("Row count misestimated: planned %d, actual %d (%.1fx more)", node.PlanRows, node.ActualRows, ratio),
			NodeType:      node.NodeType,
			RelationName:  node.RelationName,
			Suggestion:    "Run ANALYZE on the table to update statistics",
			ActualValue:   node.ActualRows,
			ExpectedValue: node.PlanRows,
		}
	}

	if ratio < 1/threshold && node.PlanRows > 100 {
		return &types.Finding{
			Type:          r.Name(),
			Severity:      types.SeverityMedium,
			Message:       fmt.Sprintf("Row count overestimated: planned %d, actual %d (%.1fx less)", node.PlanRows, node.ActualRows, 1/ratio),
			NodeType:      node.NodeType,
			RelationName:  node.RelationName,
			Suggestion:    "Run ANALYZE to improve planning accuracy",
			ActualValue:   node.ActualRows,
			ExpectedValue: node.PlanRows,
		}
	}

	return nil
}

type NestedLoopIssueRule struct{}

func (r *NestedLoopIssueRule) Name() string {
	return "NestedLoopIssue"
}

func (r *NestedLoopIssueRule) Severity() types.Severity {
	return types.SeverityHigh
}

func (r *NestedLoopIssueRule) Check(node *types.PlanNode, plan *types.Plan, config *types.AnalyzerConfig) *types.Finding {
	if !strings.Contains(node.NodeType, "Nested Loop") {
		return nil
	}

	threshold := config.NestedLoopRowThreshold
	if threshold == 0 {
		threshold = 1000
	}

	if node.ActualRows > threshold {
		return &types.Finding{
			Type:        r.Name(),
			Severity:    r.Severity(),
			Message:     fmt.Sprintf("Nested loop with high row count: %d rows", node.ActualRows),
			NodeType:    node.NodeType,
			Suggestion:  "Consider using hash join or increasing work_mem",
			ActualValue: node.ActualRows,
		}
	}

	return nil
}

type HashSpillRule struct{}

func (r *HashSpillRule) Name() string {
	return "HashSpill"
}

func (r *HashSpillRule) Severity() types.Severity {
	return types.SeverityMedium
}

func (r *HashSpillRule) Check(node *types.PlanNode, plan *types.Plan, config *types.AnalyzerConfig) *types.Finding {
	if !strings.Contains(node.NodeType, "Hash") && node.NodeType != "Hash" {
		return nil
	}

	if node.Buffers != nil && (node.Buffers.TempWritten > 0 || node.Buffers.TempRead > 0) {
		return &types.Finding{
			Type:     r.Name(),
			Severity: r.Severity(),
			Message: fmt.Sprintf("Hash operation spilled to disk: temp read=%d, temp written=%d",
				node.Buffers.TempRead, node.Buffers.TempWritten),
			NodeType:   node.NodeType,
			Suggestion: "Increase work_mem parameter to avoid disk spills",
		}
	}

	if node.Batches > 1 {
		return &types.Finding{
			Type:       r.Name(),
			Severity:   r.Severity(),
			Message:    fmt.Sprintf("Hash operation used %d batches", node.Batches),
			NodeType:   node.NodeType,
			Suggestion: "Increase work_mem to reduce batch count",
		}
	}

	return nil
}

type SortSpillRule struct{}

func (r *SortSpillRule) Name() string {
	return "SortSpill"
}

func (r *SortSpillRule) Severity() types.Severity {
	return types.SeverityMedium
}

func (r *SortSpillRule) Check(node *types.PlanNode, plan *types.Plan, config *types.AnalyzerConfig) *types.Finding {
	if !strings.Contains(node.NodeType, "Sort") && node.NodeType != "Sort" {
		return nil
	}

	if node.Buffers != nil && (node.Buffers.TempWritten > 0 || node.Buffers.TempRead > 0) {
		return &types.Finding{
			Type:     r.Name(),
			Severity: r.Severity(),
			Message: fmt.Sprintf("Sort operation spilled to disk: temp read=%d, temp written=%d",
				node.Buffers.TempRead, node.Buffers.TempWritten),
			NodeType:   node.NodeType,
			Suggestion: "Increase work_mem to keep sort in memory",
		}
	}

	if node.SortSpaceUsed > 0 && node.SortSpaceType == "Disk" {
		return &types.Finding{
			Type:       r.Name(),
			Severity:   r.Severity(),
			Message:    fmt.Sprintf("Sort used %d KB on disk", node.SortSpaceUsed),
			NodeType:   node.NodeType,
			Suggestion: "Increase work_mem to sort in memory",
		}
	}

	return nil
}

type MissingIndexRule struct{}

func (r *MissingIndexRule) Name() string {
	return "MissingIndex"
}

func (r *MissingIndexRule) Severity() types.Severity {
	return types.SeverityHigh
}

func (r *MissingIndexRule) Check(node *types.PlanNode, plan *types.Plan, config *types.AnalyzerConfig) *types.Finding {
	if node.NodeType != "Seq Scan" {
		return nil
	}

	if node.Filter != "" && node.PlanRows > 1000 {
		columns := extractColumnsFromFilter(node.Filter)
		if len(columns) > 0 {
			return &types.Finding{
				Type:         r.Name(),
				Severity:     r.Severity(),
				Message:      fmt.Sprintf("Potential missing index on '%s' for filter: %s", node.RelationName, node.Filter),
				NodeType:     node.NodeType,
				RelationName: node.RelationName,
				Suggestion:   fmt.Sprintf("CREATE INDEX ON %s (%s);", node.RelationName, strings.Join(columns, ", ")),
			}
		}
	}

	return nil
}

type LargeOffsetRule struct{}

func (r *LargeOffsetRule) Name() string {
	return "LargeOffset"
}

func (r *LargeOffsetRule) Severity() types.Severity {
	return types.SeverityLow
}

func (r *LargeOffsetRule) Check(node *types.PlanNode, plan *types.Plan, config *types.AnalyzerConfig) *types.Finding {
	if node.NodeType == "Limit" && node.PlanRows < 100 {
		return &types.Finding{
			Type:       r.Name(),
			Severity:   r.Severity(),
			Message:    "Small LIMIT may cause performance issues with large OFFSET",
			NodeType:   node.NodeType,
			Suggestion: "Use keyset pagination (WHERE id > last_id) instead of OFFSET",
		}
	}

	return nil
}

type BitmapTooManyRowsRule struct{}

func (r *BitmapTooManyRowsRule) Name() string {
	return "BitmapTooManyRows"
}

func (r *BitmapTooManyRowsRule) Severity() types.Severity {
	return types.SeverityMedium
}

func (r *BitmapTooManyRowsRule) Check(node *types.PlanNode, plan *types.Plan, config *types.AnalyzerConfig) *types.Finding {
	if node.NodeType != "Bitmap Heap Scan" {
		return nil
	}

	if node.Buffers != nil && node.Buffers.SharedDirtied > 0 {
		return &types.Finding{
			Type:       r.Name(),
			Severity:   r.Severity(),
			Message:    "Bitmap heap scan with lossy bitmaps (dirtied pages)",
			NodeType:   node.NodeType,
			Suggestion: "Increase work_mem or create better index",
		}
	}

	return nil
}

type ParallelDisabledRule struct{}

func (r *ParallelDisabledRule) Name() string {
	return "ParallelDisabled"
}

func (r *ParallelDisabledRule) Severity() types.Severity {
	return types.SeverityMedium
}

func (r *ParallelDisabledRule) Check(node *types.PlanNode, plan *types.Plan, config *types.AnalyzerConfig) *types.Finding {
	if !config.EnableParallelQuery {
		return nil
	}

	if node.NodeType == "Seq Scan" && node.PlanRows > 10000 && node.ActualLoops == 1 && node.WorkersLaunched == 0 {
		return &types.Finding{
			Type:         r.Name(),
			Severity:     r.Severity(),
			Message:      fmt.Sprintf("Sequential scan on large table '%s' without parallel workers", node.RelationName),
			NodeType:     node.NodeType,
			RelationName: node.RelationName,
			Suggestion:   "Enable parallel query or optimize with indexes",
		}
	}

	return nil
}

type SlowScanRule struct{}

func (r *SlowScanRule) Name() string {
	return "SlowScan"
}

func (r *SlowScanRule) Severity() types.Severity {
	return types.SeverityMedium
}

func (r *SlowScanRule) Check(node *types.PlanNode, plan *types.Plan, config *types.AnalyzerConfig) *types.Finding {
	if !node.IsScanNode() {
		return nil
	}

	if node.ActualTotalTime > 100 && node.ActualRows > 0 {
		timePerRow := node.ActualTotalTime / float64(node.ActualRows)
		if timePerRow > 1 {
			return &types.Finding{
				Type:         r.Name(),
				Severity:     r.Severity(),
				Message:      fmt.Sprintf("Slow scan on '%s': %.2fms per row", node.RelationName, timePerRow),
				NodeType:     node.NodeType,
				RelationName: node.RelationName,
				Suggestion:   "Consider adding an index to speed up this scan",
			}
		}
	}

	return nil
}

type HighCostRule struct{}

func (r *HighCostRule) Name() string {
	return "HighCost"
}

func (r *HighCostRule) Severity() types.Severity {
	return types.SeverityMedium
}

func (r *HighCostRule) Check(node *types.PlanNode, plan *types.Plan, config *types.AnalyzerConfig) *types.Finding {
	if node.TotalCost > 10000 {
		return &types.Finding{
			Type:         r.Name(),
			Severity:     r.Severity(),
			Message:      fmt.Sprintf("High cost node: %.2f on %s (%s)", node.TotalCost, node.RelationName, node.NodeType),
			NodeType:     node.NodeType,
			RelationName: node.RelationName,
			Suggestion:   fmt.Sprintf("Optimize this %s operation. Consider adding indexes or rewriting the query to reduce cost.", node.NodeType),
			ActualValue:  node.TotalCost,
		}
	}

	return nil
}

type ManyLoopsRule struct{}

func (r *ManyLoopsRule) Name() string {
	return "ManyLoops"
}

func (r *ManyLoopsRule) Severity() types.Severity {
	return types.SeverityMedium
}

func (r *ManyLoopsRule) Check(node *types.PlanNode, plan *types.Plan, config *types.AnalyzerConfig) *types.Finding {
	if node.ActualLoops > 1000 {
		return &types.Finding{
			Type:       r.Name(),
			Severity:   r.Severity(),
			Message:    fmt.Sprintf("Node executed %d times (loops)", node.ActualLoops),
			NodeType:   node.NodeType,
			Suggestion: "Consider reducing loop count with better indexing or join order",
		}
	}

	return nil
}

type RowsRemovedByFilterRule struct{}

func (r *RowsRemovedByFilterRule) Name() string {
	return "RowsRemovedByFilter"
}

func (r *RowsRemovedByFilterRule) Severity() types.Severity {
	return types.SeverityMedium
}

func (r *RowsRemovedByFilterRule) Check(node *types.PlanNode, plan *types.Plan, config *types.AnalyzerConfig) *types.Finding {
	if node.RowsRemovedByFilter > 1000 {
		return &types.Finding{
			Type:         r.Name(),
			Severity:     r.Severity(),
			Message:      fmt.Sprintf("Removed %d rows by filter on '%s'", node.RowsRemovedByFilter, node.RelationName),
			NodeType:     node.NodeType,
			RelationName: node.RelationName,
			Suggestion:   "Create an index on filtered column to reduce rows before scanning",
		}
	}

	return nil
}

func GetAllRules() []AnalysisRule {
	return []AnalysisRule{
		&SequentialScanRule{},
		&RowEstimationMismatchRule{},
		&NestedLoopIssueRule{},
		&HashSpillRule{},
		&SortSpillRule{},
		&MissingIndexRule{},
		&LargeOffsetRule{},
		&BitmapTooManyRowsRule{},
		&ParallelDisabledRule{},
		&SlowScanRule{},
		&HighCostRule{},
		&ManyLoopsRule{},
		&RowsRemovedByFilterRule{},
	}
}

func extractColumnsFromFilter(filter string) []string {
	var columns []string
	parts := strings.Split(filter, " AND ")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		part = strings.TrimPrefix(part, "(")
		part = strings.TrimSuffix(part, ")")

		ops := []string{"=", "<", ">", "<=", ">=", "<>", "!=", "LIKE", "IN", "BETWEEN"}
		for _, op := range ops {
			if idx := strings.Index(part, op); idx > 0 {
				col := strings.TrimSpace(part[:idx])
				col = strings.Trim(col, "\"")
				if col != "" && !contains(columns, col) {
					columns = append(columns, col)
				}
				break
			}
		}
	}
	return columns
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
