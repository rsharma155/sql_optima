package types

import (
	"encoding/json"
	"fmt"
	"strings"
)

type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityHigh     Severity = "high"
	SeverityMedium   Severity = "medium"
	SeverityLow      Severity = "low"
	SeverityInfo     Severity = "info"
)

type Finding struct {
	Type          string      `json:"type"`
	Severity      Severity    `json:"severity"`
	Message       string      `json:"message"`
	NodeID        int         `json:"node_id,omitempty"`
	NodeType      string      `json:"node_type,omitempty"`
	RelationName  string      `json:"relation_name,omitempty"`
	Suggestion    string      `json:"suggestion"`
	ActualValue   interface{} `json:"actual_value,omitempty"`
	ExpectedValue interface{} `json:"expected_value,omitempty"`
}

type Findings []Finding

func (f Findings) FilterByMinSeverity(minSeverity Severity) Findings {
	severityOrder := map[Severity]int{
		SeverityInfo:     0,
		SeverityLow:      1,
		SeverityMedium:   2,
		SeverityHigh:     3,
		SeverityCritical: 4,
	}

	minLevel := severityOrder[minSeverity]
	var result Findings
	for _, finding := range f {
		if severityOrder[finding.Severity] >= minLevel {
			result = append(result, finding)
		}
	}
	return result
}

func (f Findings) Summary() map[Severity]int {
	summary := map[Severity]int{
		SeverityCritical: 0,
		SeverityHigh:     0,
		SeverityMedium:   0,
		SeverityLow:      0,
		SeverityInfo:     0,
	}
	for _, finding := range f {
		summary[finding.Severity]++
	}
	return summary
}

type Buffers struct {
	SharedHit     int `json:"shared_hit,omitempty"`
	SharedRead    int `json:"shared_read,omitempty"`
	SharedWritten int `json:"shared_written,omitempty"`
	SharedDirtied int `json:"shared_dirtied,omitempty"`
	LocalHit      int `json:"local_hit,omitempty"`
	LocalRead     int `json:"local_read,omitempty"`
	LocalWritten  int `json:"local_written,omitempty"`
	TempRead      int `json:"temp_read,omitempty"`
	TempWritten   int `json:"temp_written,omitempty"`
}

func (b Buffers) String() string {
	var parts []string
	if b.SharedHit > 0 {
		parts = append(parts, fmt.Sprintf("shared hit=%d", b.SharedHit))
	}
	if b.SharedRead > 0 {
		parts = append(parts, fmt.Sprintf("shared read=%d", b.SharedRead))
	}
	if b.SharedWritten > 0 {
		parts = append(parts, fmt.Sprintf("shared written=%d", b.SharedWritten))
	}
	if b.TempRead > 0 {
		parts = append(parts, fmt.Sprintf("temp read=%d", b.TempRead))
	}
	if b.TempWritten > 0 {
		parts = append(parts, fmt.Sprintf("temp written=%d", b.TempWritten))
	}
	return strings.Join(parts, ", ")
}

type PlanNode struct {
	ID            int      `json:"id"`
	NodeType      string   `json:"node_type"`
	RelationName  string   `json:"relation_name,omitempty"`
	Alias         string   `json:"alias,omitempty"`
	IndexName     string   `json:"index_name,omitempty"`
	IndexCond     string   `json:"index_cond,omitempty"`
	Filter        string   `json:"filter,omitempty"`
	JoinFilter    string   `json:"join_filter,omitempty"`
	JoinType      string   `json:"join_type,omitempty"`
	HashCond      string   `json:"hash_cond,omitempty"`
	MergeCond     string   `json:"merge_cond,omitempty"`
	SortKey       []string `json:"sort_key,omitempty"`
	GroupKey      []string `json:"group_key,omitempty"`
	SortMethod    string   `json:"sort_method,omitempty"`
	SortSpaceType string   `json:"sort_space_type,omitempty"`
	SortSpaceUsed int      `json:"sort_space_used,omitempty"`

	StartupCost float64 `json:"startup_cost"`
	TotalCost   float64 `json:"total_cost"`
	PlanRows    int     `json:"plan_rows"`
	PlanWidth   int     `json:"plan_width"`

	ActualStartupTime float64 `json:"actual_startup_time,omitempty"`
	ActualTotalTime   float64 `json:"actual_total_time,omitempty"`
	ActualRows        int     `json:"actual_rows,omitempty"`
	ActualLoops       int     `json:"actual_loops,omitempty"`

	SharedHitBuffers  int      `json:"shared_hit_buffers,omitempty"`
	SharedReadBuffers int      `json:"shared_read_buffers,omitempty"`
	Buffers           *Buffers `json:"buffers,omitempty"`

	WorkersLaunched int `json:"workers_launched,omitempty"`
	WorkersPlanned  int `json:"workers_planned,omitempty"`

	Buckets     int `json:"buckets,omitempty"`
	Batches     int `json:"batches,omitempty"`
	MemoryUsage int `json:"memory_usage,omitempty"`

	RowsRemovedByFilter int `json:"rows_removed_by_filter,omitempty"`

	Plans []PlanNode `json:"plans,omitempty"`

	ExclusiveTime float64 `json:"exclusive_time,omitempty"`
	InclusiveTime float64 `json:"inclusive_time,omitempty"`

	Parent *PlanNode `json:"-"`
	Depth  int       `json:"depth,omitempty"`
}

func (p PlanNode) GetCost() (startup, total float64) {
	return p.StartupCost, p.TotalCost
}

func (p PlanNode) HasActualMetrics() bool {
	return p.ActualTotalTime > 0 || p.ActualRows > 0
}

func (p PlanNode) IsScanNode() bool {
	scanTypes := []string{
		"Seq Scan", "Index Scan", "Index Only Scan",
		"Bitmap Heap Scan", "Tid Scan", "Tid Range Scan",
		"Function Scan", "Table Function Scan", "Values Scan",
		"Cte Scan", "Named Tuplestore Scan", "WorkTable Scan",
		"Foreign Scan", "Custom Scan",
	}
	for _, t := range scanTypes {
		if strings.Contains(p.NodeType, t) {
			return true
		}
	}
	return false
}

func (p PlanNode) IsJoinNode() bool {
	joinTypes := []string{
		"Nested Loop", "Hash Join", "Merge Join",
		"Materialize", "Memoize",
	}
	for _, t := range joinTypes {
		if strings.Contains(p.NodeType, t) {
			return true
		}
	}
	return false
}

func (p PlanNode) IsAggregationNode() bool {
	aggTypes := []string{
		"Aggregate", "GroupAggregate", "HashAggregate",
		"Mixed Aggregate", "WindowAgg", "Partial HashAggregate",
		"Finalize GroupAggregate",
	}
	for _, t := range aggTypes {
		if strings.Contains(p.NodeType, t) {
			return true
		}
	}
	return false
}

type Plan struct {
	Query         string            `json:"query,omitempty"`
	Plan          PlanNode          `json:"plan"`
	PlanningTime  float64           `json:"planning_time"`
	ExecutionTime float64           `json:"execution_time"`
	Settings      map[string]string `json:"settings,omitempty"`
	Jit           *JitInfo          `json:"jit,omitempty"`
	Triggers      []TriggerInfo     `json:"triggers,omitempty"`
}

type JitInfo struct {
	Functions  int `json:"functions,omitempty"`
	Operations int `json:"operations,omitempty"`
	Timeline   int `json:"timeline,omitempty"`
}

type TriggerInfo struct {
	Name string  `json:"name"`
	Time float64 `json:"time"`
}

type AnalysisResult struct {
	ID        string   `json:"id"`
	Query     string   `json:"query,omitempty"`
	Plan      Plan     `json:"plan"`
	PlanTree  PlanNode `json:"plan_tree"`
	Findings  Findings `json:"findings"`
	Summary   Summary  `json:"summary"`
	CreatedAt string   `json:"created_at"`
	RawPlan   string   `json:"raw_plan,omitempty"`
}

type Summary struct {
	TotalCost       float64          `json:"total_cost"`
	ExecutionTimeMs float64          `json:"execution_time_ms"`
	PlanningTimeMs  float64          `json:"planning_time_ms"`
	FindingsCount   map[Severity]int `json:"findings_count"`
	NodeCount       int              `json:"node_count"`
	ScanCount       int              `json:"scan_count"`
	JoinCount       int              `json:"join_count"`
}

func NewAnalysisResult(query string, plan Plan) *AnalysisResult {
	result := &AnalysisResult{
		Query:    query,
		Plan:     plan,
		PlanTree: plan.Plan,
	}
	result.Summary.TotalCost = plan.Plan.TotalCost
	result.Summary.ExecutionTimeMs = plan.ExecutionTime
	result.Summary.PlanningTimeMs = plan.PlanningTime
	result.Summary.FindingsCount = make(map[Severity]int)

	return result
}

func (a *AnalysisResult) ToJSON() (string, error) {
	a.PlanTree = a.Plan.Plan
	a.Summary.TotalCost = a.PlanTree.TotalCost
	a.Summary.ExecutionTimeMs = a.Plan.ExecutionTime
	a.Summary.PlanningTimeMs = a.Plan.PlanningTime
	data, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (a *AnalysisResult) FlattenNodes() []PlanNode {
	var nodes []PlanNode

	var flatten func(node *PlanNode, depth int)
	flatten = func(node *PlanNode, depth int) {
		node.Depth = depth
		nodes = append(nodes, *node)
		for i := range node.Plans {
			node.Plans[i].Parent = node
			flatten(&node.Plans[i], depth+1)
		}
	}
	flatten(&a.PlanTree, 0)

	for i := range nodes {
		nodes[i].ID = i + 1
	}

	a.calculateTimes()

	return nodes
}

func (a *AnalysisResult) calculateTimes() {
	var calc func(node *PlanNode) float64
	calc = func(node *PlanNode) float64 {
		if node.ActualTotalTime == 0 {
			node.ExclusiveTime = 0
			node.InclusiveTime = 0
			for i := range node.Plans {
				calc(&node.Plans[i])
			}
			return 0
		}

		inclusive := node.ActualTotalTime
		var childTime float64
		for i := range node.Plans {
			childTime += calc(&node.Plans[i])
		}
		exclusive := inclusive - childTime
		if exclusive < 0 {
			exclusive = 0
		}

		node.ExclusiveTime = exclusive
		node.InclusiveTime = inclusive

		return inclusive
	}

	calc(&a.PlanTree)
}

type PlanDiff struct {
	AddedNodes   []PlanNode   `json:"added_nodes"`
	RemovedNodes []PlanNode   `json:"removed_nodes"`
	ChangedNodes []NodeChange `json:"changed_nodes"`
	Summary      DiffSummary  `json:"summary"`
}

type NodeChange struct {
	NodeID   int         `json:"node_id"`
	NodeType string      `json:"node_type"`
	Field    string      `json:"field"`
	OldValue interface{} `json:"old_value"`
	NewValue interface{} `json:"new_value"`
}

type DiffSummary struct {
	TotalChanges int     `json:"total_changes"`
	CostChange   float64 `json:"cost_change"`
	TimeChange   float64 `json:"time_change"`
}

type AnalyzerConfig struct {
	SequentialScanThreshold int     `json:"sequential_scan_threshold"`
	RowMismatchThreshold    float64 `json:"row_mismatch_threshold"`
	NestedLoopRowThreshold  int     `json:"nested_loop_row_threshold"`
	WorkMemMB               int     `json:"work_mem_mb"`
	EnableParallelQuery     bool    `json:"enable_parallel_query"`
}

var DefaultConfig = AnalyzerConfig{
	SequentialScanThreshold: 10000,
	RowMismatchThreshold:    10.0,
	NestedLoopRowThreshold:  1000,
	WorkMemMB:               4,
	EnableParallelQuery:     true,
}

type OptimizationReport struct {
	ExecutiveSummary    ExecutiveSummary    `json:"executive_summary"`
	PerformanceScore    PerformanceScore    `json:"performance_score"`
	TopIssues           []TopIssue          `json:"top_issues"`
	Recommendations     []Recommendation    `json:"recommendations"`
	PlanAnalysis        PlanAnalysis        `json:"plan_analysis"`
	AdvancedDiagnostics AdvancedDiagnostics `json:"advanced_diagnostics"`
	IndexAnalysis       IndexAnalysis       `json:"index_analysis"`
	QueryImprovements   []QueryImprovement  `json:"query_improvements"`
	ConfidenceScore     ConfidenceScore     `json:"confidence_score"`
}

type ExecutiveSummary struct {
	TotalExecutionTime string `json:"total_execution_time"`
	PrimaryBottleneck  string `json:"primary_bottleneck"`
	SecondaryIssues    string `json:"secondary_issues"`
	OverallDiagnosis   string `json:"overall_diagnosis"`
}

type PerformanceScore struct {
	Score int    `json:"score"`
	Label string `json:"label"`
}

type TopIssue struct {
	Title       string `json:"title"`
	Severity    string `json:"severity"`
	Description string `json:"description"`
	Evidence    string `json:"evidence"`
	Impact      string `json:"impact"`
}

type Recommendation struct {
	IssueTitle     string `json:"issue_title"`
	Action         string `json:"action"`
	Explanation    string `json:"explanation"`
	SQLExample     string `json:"sql_example,omitempty"`
	ExpectedImpact string `json:"expected_impact"`
}

type PlanAnalysis struct {
	KeyNodes         []KeyNode          `json:"key_nodes"`
	TimePerNode      map[string]float64 `json:"time_per_node"`
	InefficientSteps []string           `json:"inefficient_steps"`
}

type KeyNode struct {
	NodeType      string  `json:"node_type"`
	RelationName  string  `json:"relation_name,omitempty"`
	TimePercent   float64 `json:"time_percent"`
	RowsProcessed int     `json:"rows_processed"`
	RowsEstimated int     `json:"rows_estimated"`
}

type AdvancedDiagnostics struct {
	RowEstimationAccuracy RowEstimationAccuracy `json:"row_estimation_accuracy"`
	JoinStrategyAnalysis  JoinStrategyAnalysis  `json:"join_strategy_analysis"`
	MemoryDiskUsage       MemoryDiskUsage       `json:"memory_disk_usage"`
	ParallelismAnalysis   ParallelismAnalysis   `json:"parallelism_analysis"`
	IOvsCPUBound          IOvsCPUBound          `json:"io_vs_cpu_bound"`
}

type RowEstimationAccuracy struct {
	Mismatches []RowMismatch `json:"mismatches"`
}

type RowMismatch struct {
	NodeType     string  `json:"node_type"`
	RelationName string  `json:"relation_name"`
	Estimated    int     `json:"estimated"`
	Actual       int     `json:"actual"`
	Ratio        float64 `json:"ratio"`
}

type JoinStrategyAnalysis struct {
	Nodes []JoinAnalysisNode `json:"nodes"`
}

type JoinAnalysisNode struct {
	NodeType      string `json:"node_type"`
	JoinType      string `json:"join_type"`
	IsAppropriate bool   `json:"is_appropriate"`
	Suggestion    string `json:"suggestion,omitempty"`
}

type MemoryDiskUsage struct {
	HashSpills []DiskSpill `json:"hash_spills"`
	SortSpills []DiskSpill `json:"sort_spills"`
}

type DiskSpill struct {
	NodeType   string `json:"node_type"`
	MemoryUsed int    `json:"memory_used_kb"`
	DiskUsed   int    `json:"disk_used_kb"`
	Suggestion string `json:"suggestion"`
}

type ParallelismAnalysis struct {
	Used            bool   `json:"used"`
	WorkersLaunched int    `json:"workers_launched"`
	ShouldUse       bool   `json:"should_use"`
	Suggestion      string `json:"suggestion,omitempty"`
}

type IOvsCPUBound struct {
	IsIOBound bool   `json:"is_io_bound"`
	Reason    string `json:"reason"`
	Evidence  string `json:"evidence"`
}

type IndexAnalysis struct {
	MissingIndexes     []IndexSuggestion `json:"missing_indexes"`
	InefficientIndexes []IndexSuggestion `json:"inefficient_indexes"`
}

type IndexSuggestion struct {
	Table   string   `json:"table"`
	Columns []string `json:"columns"`
	Reason  string   `json:"reason"`
	SQL     string   `json:"sql"`
}

type QueryImprovement struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	Suggestion  string `json:"suggestion"`
	SQLExample  string `json:"sql_example,omitempty"`
}

type ConfidenceScore struct {
	Level   string   `json:"level"`
	Factors []string `json:"factors"`
}
