// Package: github.com/rsharma155/sqlplan-analyzer/internal/models
// Purpose: Core domain models for SQL Server execution plan analysis
package models

import (
	"time"
)

type EvidenceSource string

const (
	EvidenceRuntimeCounters EvidenceSource = "RunTimeCountersPerThread"
	EvidenceMemoryGrant     EvidenceSource = "MemoryGrantInfo"
	EvidenceQueryPlan       EvidenceSource = "QueryPlan"
	EvidenceWarnings        EvidenceSource = "Warnings"
	EvidenceMissingIndex    EvidenceSource = "MissingIndexes"
	EvidenceOperator        EvidenceSource = "Operator"
	EvidencePredicate       EvidenceSource = "Predicate"
	EvidenceParallelism     EvidenceSource = "Parallelism"
	EvidenceStatistics      EvidenceSource = "Statistics"
)

type ConfidenceLevel float64

const (
	ConfidenceHigh   ConfidenceLevel = 0.90
	ConfidenceMedium ConfidenceLevel = 0.60
	ConfidenceLow    ConfidenceLevel = 0.30
	ConfidenceNone   ConfidenceLevel = 0.0
)

type Severity string

const (
	SeverityCritical Severity = "Critical"
	SeverityHigh     Severity = "High"
	SeverityMedium   Severity = "Medium"
	SeverityLow      Severity = "Low"
	SeverityInfo     Severity = "Info"
)

type HealthScore struct {
	OverallScore       int            `json:"overall_score"`
	AccessMethodsScore int            `json:"access_methods_score"`
	MemoryUsageScore   int            `json:"memory_usage_score"`
	JoinStrategyScore  int            `json:"join_strategy_score"`
	ParallelismScore   int            `json:"parallelism_score"`
	CardinalityScore   int            `json:"cardinality_score"`
	Breakdown          map[string]int `json:"breakdown"`
}

type QueryMetadata struct {
	QueryText                   string        `json:"query_text"`
	StatementText               string        `json:"statement_text"`
	StatementEstLines           int           `json:"statement_est_lines"`
	StatementSubDbID            string        `json:"statement_sub_db_id"`
	StatementCompO              int           `json:"statement_comp_o"`
	StatementOpti               string        `json:"statement_opti"`
	StatementOptiOn             string        `json:"statement_opti_on"`
	StatementSubProc            int           `json:"statement_sub_proc"`
	StatementType               string        `json:"statement_type"`
	ServerName                  string        `json:"server_name"`
	DatabaseName                string        `json:"database_name"`
	UserName                    string        `json:"user_name"`
	LaunchDop                   int           `json:"launch_dop"`
	ParseTime                   time.Duration `json:"parse_time"`
	CompileTime                 time.Duration `json:"compile_time"`
	OptimizedPlan               bool          `json:"optimized_plan"`
	StatementId                 string        `json:"statement_id"`
	QueryHash                   string        `json:"query_hash"`
	QueryPlanHash               string        `json:"query_plan_hash"`
	CEVersion                   string        `json:"ce_version"`
	RetrievedFromCache          bool          `json:"retrieved_from_cache"`
	DatabaseContextSettingsId   string        `json:"database_context_settings_id"`
	ParentObjectId              int           `json:"parent_object_id"`
	StatementParameterizationType string      `json:"statement_parameterization_type"`
}

type Operator struct {
	ID                       int                 `json:"id"`
	PhysicalOp               string              `json:"physical_op"`
	LogicalOp                string              `json:"logical_op"`
	NodeID                   int                 `json:"node_id"`
	ParentNodeID             int                 `json:"parent_node_id"`
	EstimateCost             float64             `json:"estimate_cost"`
	EstimatedTotalSubtreeCost float64            `json:"estimated_total_subtree_cost"`
	EstimateRows             float64             `json:"estimate_rows"`
	EstimatedRowsRead        float64             `json:"estimated_rows_read"`
	EstimateCPUms            float64             `json:"estimate_cpu_ms"`
	EstimatedIOs             float64             `json:"estimated_ios"`
	AvgRowSize               float64             `json:"avg_row_size"`
	EstimateRebinds          int64               `json:"estimate_rebinds"`
	EstimateRewinds          int64               `json:"estimate_rewinds"`
	Parallel                 bool                `json:"parallel"`
	ParallelThreadCount      int                 `json:"parallel_thread_count"`
	Storage                  string              `json:"storage"`
	Action                   string              `json:"action"`
	ForceColumnOrder         bool                `json:"force_column_order"`
	ConstantScan             []ConstantScan      `json:"constant_scan"`
	RelOp                    *Operator           `json:"rel_op,omitempty"`
	Children                 []*Operator         `json:"children,omitempty"`
	Warnings                 []Warning           `json:"warnings,omitempty"`
	Predicate                *Predicate          `json:"predicate,omitempty"`
	OutputList               []OutputColumn      `json:"output_list,omitempty"`
	IndexScan                *IndexScan           `json:"index_scan,omitempty"`
	TableScan                *TableScan          `json:"table_scan,omitempty"`
	SeekPredicates           []SeekPredicate     `json:"seek_predicates,omitempty"`
	RelOpType                string              `json:"rel_op_type"`
	Depth                    int                 `json:"depth"`
	ActualRows               int64               `json:"actual_rows"`
	ActualExecutions         int                 `json:"actual_executions"`
	ActualCPUms              float64             `json:"actual_cpu_ms"`
	ActualElapsedms          float64             `json:"actual_elapsed_ms"`
	ActualIOs                float64             `json:"actual_ios"`
	ActualLogicalReads       int64               `json:"actual_logical_reads"`
	ActualPhysicalReads      int64               `json:"actual_physical_reads"`
	ActualRebinds            int64               `json:"actual_rebinds"`
	ActualRewinds            int64               `json:"actual_rewinds"`
	ActualSpills             int64               `json:"actual_spills"`
	TableCardinality          int64               `json:"table_cardinality"`
	EstimatedExecutionMode    string              `json:"estimated_execution_mode"`
	RuntimeCounters          []RuntimeCounter    `json:"runtime_counters,omitempty"`
	AdaptiveThresholdRows    float64             `json:"adaptive_threshold_rows,omitempty"`
	OpWaitStats              []WaitStat          `json:"op_wait_stats,omitempty"`
	OpStatisticsInfo         []OperatorStatsInfo `json:"op_statistics_info,omitempty"`
	Hash                     *HashMatch          `json:"hash,omitempty"`
	NestedLoops              *NestedLoops       `json:"nested_loops,omitempty"`
	Merge                    *MergeJoin          `json:"merge,omitempty"`
	MemoryFractions          *MemoryFractions    `json:"memory_fractions,omitempty"`
	TopRowCount              string              `json:"top_row_count,omitempty"`
	TopIsPercent             bool                `json:"top_is_percent,omitempty"`
	TopWithTies              bool                `json:"top_with_ties,omitempty"`
	Sort                     *SortInfo           `json:"sort,omitempty"`
	HashAggregate            *HashAggregateInfo  `json:"hash_aggregate,omitempty"`
	StreamAggregate          *StreamAggregateInfo `json:"stream_aggregate,omitempty"`
	Spool                    *SpoolInfo          `json:"spool,omitempty"`
	WindowAggregate          *WindowAggregateInfo `json:"window_aggregate,omitempty"`
	Parameters               []ParameterInfo     `json:"parameters,omitempty"`
	PartitioningType         string              `json:"partitioning_type,omitempty"`
	PartitionCount           int                 `json:"partition_count,omitempty"`
	IsDistinct               bool                `json:"is_distinct,omitempty"`
	IsAssert                 bool                `json:"is_assert,omitempty"`
	IsConcatenation          bool                `json:"is_concatenation,omitempty"`
	IsConstantScan           bool                `json:"is_constant_scan,omitempty"`
	IsTableValuedFunction    bool                `json:"is_table_valued_function,omitempty"`
	IsSequenceProject        bool                `json:"is_sequence_project,omitempty"`
	IsRemoteQuery            bool                `json:"is_remote_query,omitempty"`
	IsUpdate                 bool                `json:"is_update,omitempty"`
	IsInsert                 bool                `json:"is_insert,omitempty"`
	IsDelete                 bool                `json:"is_delete,omitempty"`
	IsComputeScalar          bool                `json:"is_compute_scalar,omitempty"`
	IsStartupExpression      bool                `json:"is_startup_expression,omitempty"`
	IsBitmap                 bool                `json:"is_bitmap,omitempty"`
	IsBitmapCreator           bool                `json:"is_bitmap_creator,omitempty"`
	IsSegment                bool                `json:"is_segment,omitempty"`
	DefinedValues            string              `json:"defined_values,omitempty"`
	AssertExpression         string              `json:"assert_expression,omitempty"`
	ConstantScanValues       string              `json:"constant_scan_values,omitempty"`
	UserDefinedFunction      string              `json:"user_defined_function,omitempty"`
	ObjectReference          string              `json:"object_reference,omitempty"`
	RemoteQueryText          string              `json:"remote_query_text,omitempty"`
}

type ConstantScan struct {
	Values []string `json:"values"`
	Rows   int     `json:"rows"`
}

type Predicate struct {
	ColumnReference []ColumnReference `json:"column_reference"`
	ScalarOperator  string            `json:"scalar_operator"`
	ScalarString    string            `json:"scalar_string"`
}

type ColumnReference struct {
	Database    string `json:"database"`
	Table      string `json:"table"`
	Schema     string `json:"schema"`
	Alias      string `json:"alias"`
	Column     string `json:"column"`
	ColumnID   int    `json:"column_id"`
	UsableInIndex bool `json:"usable_in_index"`
}

type OutputColumn struct {
	Column          string `json:"column"`
	Schema          string `json:"schema"`
	Type           string `json:"type"`
	MaxLength      int    `json:"max_length"`
	Precision     int    `json:"precision"`
	Scale         int    `json:"scale"`
	ColumnOrder   int    `json:"column_order"`
	IsComputed    bool   `json:"is_computed"`
	Collation     string `json:"collation"`
}

type IndexScan struct {
	Object          IndexObject `json:"object"`
	IndexKind     string     `json:"index_kind"`
	ScanType      string     `json:"scan_type"`
	IndexCols     []IndexExpr `json:"index_columns"`
	IncludedCols  []IndexExpr `json:"included_columns"`
	Storage       string     `json:"storage"`
	Ordered       bool       `json:"ordered,omitempty"`
	ForcedIndex   bool       `json:"forced_index,omitempty"`
}

type TableScan struct {
	Object TableObject `json:"object"`
	Tables string      `json:"tables"`
}

type IndexObject struct {
	Database string `json:"database"`
	Table    string `json:"table"`
	Schema   string `json:"schema"`
	Alias    string `json:"alias"`
	Index    string `json:"index"`
	IndexID  int    `json:"index_id"`
}

type TableObject struct {
	Database  string `json:"database"`
	Table     string `json:"table"`
	Schema    string `json:"schema"`
	Alias     string `json:"alias"`
	DbID      int    `json:"db_id"`
	ObjectID  int    `json:"object_id"`
}

type IndexExpr struct {
	Column     string `json:"column"`
	Expression string `json:"expression"`
	Ascending  bool   `json:"ascending"`
}

type SeekPredicate struct {
	SeekType          string            `json:"seek_type"`
	PrefixPredicate  []PrefixPredicate `json:"prefix_predicate"`
	RangePredicate   []RangePredicate  `json:"range_predicate"`
	ColumnReference []ColumnReference `json:"column_reference"`
}

type PrefixPredicate struct {
	ColumnReference ColumnReference `json:"column_reference"`
	ScalarOperator string         `json:"scalar_operator"`
	ScalarString   string         `json:"scalar_string"`
}

type RangePredicate struct {
	ColumnReference ColumnReference `json:"column_reference"`
	ScalarOperator  string          `json:"scalar_operator"`
	FromValue       string          `json:"from_value"`
	ToValue         string          `json:"to_value"`
	FromBound      string          `json:"from_bound"`
	ToBound        string          `json:"to_bound"`
}

type Warning struct {
	Type             WarningType `json:"type"`
	Highlight       int         `json:"highlight"`
	Severity         Severity    `json:"severity"`
	Message         string      `json:"message"`
	CpuMsUsed        int         `json:"cpu_ms_used"`
	ElapsedMs        int         `json:"elapsed_ms"`
	ThreadAvailable int         `json:"thread_available"`
	ThreadUsed      int         `json:"thread_used"`
	ActiveDop       int         `json:"active_dop"`
	BlockedThreadCount int       `json:"blocked_thread_count"`
}

type WarningType string

const (
	WarningTypeSpillToTempDB       WarningType = "SpillToTempDb"
	WarningTypeNoStatistics      WarningType = "NoStatistics"
	WarningTypeSpatialExact     WarningType = "SpatialExact"
	WarningTypeCardinalityEst WarningType = "CardinalityEstimateIssue"
	WarningTypeTypeConversion  WarningType = "TypeConversion"
	WarningTypeNoJoinPredicate WarningType = "NoJoinPredicate"
	WarningTypeTrivialPlan     WarningType = "TrivialPlanAutoUpdate"
	WarningTypeMissingIndex    WarningType = "MissingIndex"
	WarningTypeUnmatchedIndex  WarningType = "UnmatchedIndexes"
	WarningTypeFunctionOnIndex WarningType = "FunctionOnIndex"
	WarningTypeColumnsNotCovered WarningType = "ColumnsNotCovered"
	WarningTypeWaitFor          WarningType = "WaitFor"
	WarningTypeOptimization   WarningType = "Optimization"
	WarningTypeParallelism     WarningType = "Parallelism"
)

type MissingIndex struct {
	ID                    int             `json:"id"`
	Database             string          `json:"database"`
	Schema               string          `json:"schema"`
	Table                string          `json:"table"`
	Columns              []MissingIndexColumn `json:"columns"`
	IncludedColumns       []string        `json:"included_columns"`
	Statement            string          `json:"statement"`
	Score                int             `json:"score"`
	Usefulness           int             `json:"usefulness"`
	CreateIndexStatement string          `json:"create_index_statement"`
}

type MissingIndexColumn struct {
	Column            string `json:"column"`
	Catalog           string `json:"catalog"`
	Type             int    `json:"type"`
	ColumnGuid       string `json:"column_guid"`
	IsInferred       bool   `json:"is_inferred"`
	Count            int    `json:"count"`
	Equality          bool   `json:"equality"`
	Inequality       bool   `json:"inequality"`
	Inclusion        bool   `json:"inclusion"`
}

type RuntimeCounter struct {
	Thread                int     `json:"thread"`
	RowEstimate           int64   `json:"row_estimate"`
	ActualRows            int64   `json:"actual_rows"`
	ActualRowsRead        int64   `json:"actual_rows_read"`
	ActualExecutions      int     `json:"actual_executions"`
	ActualElapsedms       float64 `json:"actual_elapsed_ms"`
	ActualCPUms           float64 `json:"actual_cpu_ms"`
	ActualLogicalReads    int64   `json:"actual_logical_reads"`
	ActualPhysicalReads   int64   `json:"actual_physical_reads"`
	ActiveParallelThread  int     `json:"active_parallel_thread"`
	Batches               int     `json:"batches"`
	ActualEndOfScans      int     `json:"actual_end_of_scans"`
	ActualScans           int     `json:"actual_scans"`
	ActualRebinds         int64   `json:"actual_rebinds"`
	ActualRewinds         int64   `json:"actual_rewinds"`
	MBecameTooExpensive   bool    `json:"became_too_expensive"`
}

type HashMatch struct {
	HashKeysBuild   []ColumnReference `json:"hash_keys_build"`
	HashKeysProbe   []ColumnReference `json:"hash_keys_probe"`
	ProbeResidual   string            `json:"probe_residual"`
	BuildResidual   string            `json:"build_residual"`
	BitmapCreator   bool              `json:"bitmap_creator"`
}

type NestedLoops struct {
	OuterReferences   []ColumnReference `json:"outer_references"`
	Predicate         string            `json:"predicate"`
	OuterIsAdaptive   bool              `json:"outer_is_adaptive"`
}

type MergeJoin struct {
	InnerSideJoinColumns  []ColumnReference `json:"inner_side_join_columns"`
	OuterSideJoinColumns []ColumnReference `json:"outer_side_join_columns"`
	PassThrough          string           `json:"pass_through"`
}

type MemoryFractions struct {
	Input  float64 `json:"input"`
	Output float64 `json:"output"`
}

type MemoryGrantInfo struct {
	GrantedMemory          int     `json:"granted_memory"`
	MaxUsedMemory          int     `json:"max_used_memory"`
	IdealMemory            int     `json:"ideal_memory"`
	SerialRequiredMemory   int     `json:"serial_required_memory"`
	SerialDesiredMemory    int64   `json:"serial_desired_memory"`
	RequestedMemory        int     `json:"requested_memory"`
	TransferMode          string  `json:"transfer_mode"`
	DirectWaits           string  `json:"direct_waits"`
	MemoryUtilization     float64 `json:"memory_utilization,omitempty"`
}

func (m *MemoryGrantInfo) CalculateUtilization() float64 {
	if m.GrantedMemory <= 0 {
		return 0
	}
	return float64(m.MaxUsedMemory) / float64(m.GrantedMemory)
}

type QueryTimeStats struct {
	ElapsedMs   int `json:"elapsed_ms"`
	CpuMs      int `json:"cpu_ms"`
	RowCount   int `json:"row_count"`
	EndRowSet int `json:"end_row_set"`
}

type ThreadStat struct {
	Activity          string `json:"activity"`
	ThreadCount      int    `json:"thread_count"`
	BusyWaitMs      int    `json:"busy_wait_ms"`
	IdleWaitMs      int    `json:"idle_wait_ms"`
	YieldWaitMs     int    `json:"yield_wait_ms"`
	YieldWaitMsPct float64 `json:"yield_wait_ms_pct"`
}

type OptimizerStatsUsage struct {
	Predicate       string `json:"predicate"`
	HistogramValues string `json:"histogram_values"`
	LastUpdate      string `json:"last_update"`
	Modification   int    `json:"modification"`
	SamplingPercent float64 `json:"sampling_percent"`
	TableCard       int64  `json:"table_card"`
	ModificationMs  int64  `json:"modification_ms"`
}

type WaitStat struct {
	WaitType    string `json:"wait_type"`
	WaitTimeMs  int    `json:"wait_time_ms"`
	WaitCount   int    `json:"wait_count"`
	Percentage  float64 `json:"percentage"`
}

type ParameterInfo struct {
	Parameter string `json:"parameter"`
	Column    string `json:"column"`
	DataType  string `json:"data_type"`
	Value     string `json:"value"`
}

type SortInfo struct {
	Ascending        bool   `json:"ascending"`
	ZeroColumn       bool   `json:"zero_column"`
	PartitionColumns string `json:"partition_columns"`
	EstimateRows     int64  `json:"estimate_rows"`
}

type HashAggregateInfo struct {
	DefinedValues string `json:"defined_values"`
	GroupBy        string `json:"group_by"`
	EstimateRows   int64  `json:"estimate_rows"`
}

type StreamAggregateInfo struct {
	DefinedValues string `json:"defined_values"`
	GroupBy       string `json:"group_by"`
}

type SpoolInfo struct {
	IsAsync      bool  `json:"is_async"`
	EstimateRows int64 `json:"estimate_rows"`
}

type WindowAggregateInfo struct {
	WindowFunctions string `json:"window_functions"`
	DefinedValues    string `json:"defined_values"`
}

type QueryPlan struct {
	RelOp                  *Operator     `json:"rel_op"`
	HasMemoryGrant          bool         `json:"has_memory_grant"`
	MemoryGrantInfo        MemoryGrantInfo `json:"memory_grant_info,omitempty"`
	QueryTimeStats         QueryTimeStats `json:"query_time_stats,omitempty"`
	ThreadStats           []ThreadStat  `json:"thread_stats,omitempty"`
	OptimizerStatsUsage   []OptimizerStatsUsage `json:"optimizer_stats_usage,omitempty"`
	WaitStats             []WaitStat   `json:"wait_stats,omitempty"`
	MissingIndexes        []MissingIndex `json:"missing_indexes,omitempty"`
	DegreeOfParallelism     int          `json:"degree_of_parallelism"`
	MaxDegreeOfParallelism int         `json:"max_degree_of_parallelism"`
	MemoryGrant            int         `json:"memory_grant"`
	EstimatedAvailableDOP int         `json:"estimated_available_dop"`
	EstimatedAvailableMemoryGrant int `json:"estimated_available_memory_grant"`
	MaxCompileMemory       int         `json:"max_compile_memory"`
	UseParallelBatches    bool         `json:"use_parallel_batches"`
	CachedPlanCached      bool         `json:"cached_plan_cached"`
	CachedPlanSize       int          `json:"cached_plan_size"`
	CompileTimeMs         int          `json:"compile_time_ms"`
	CompileCPU          int          `json:"compile_cpu"`
	CompileMemory       int          `json:"compile_memory"`
	OptimizationLevel    string       `json:"optimization_level"`
	WaitStatsBatch           bool   `json:"wait_stats_batch"`
	NonParallelPlanReason    string `json:"non_parallel_plan_reason,omitempty"`
}

type EvidenceDetail struct {
	Source      EvidenceSource `json:"source"`
	Value       string         `json:"value"`
	Description string         `json:"description"`
	OperatorIDs []int          `json:"operator_ids,omitempty"`
}

type Finding struct {
	FindingType     string     `json:"finding_type"`
	Severity        Severity   `json:"severity"`
	OperatorID      int        `json:"operator_id"`
	OperatorName   string     `json:"operator_name"`
	OperatorIDs    []int      `json:"operator_ids,omitempty"`
	Title          string     `json:"title"`
	TechnicalExplanation string `json:"technical_explanation"`
	BusinessExplanation string `json:"business_explanation"`
	Explanation    string     `json:"explanation"`
	Recommendation   string     `json:"recommendation"`
	Impact         string     `json:"impact"`
	Confidence     float64    `json:"confidence"`
	NumericImpact  float64    `json:"numeric_impact"`
	EvidenceTrace  []EvidenceDetail `json:"evidence_trace,omitempty"`
	RootCauses     []Finding  `json:"root_cause,omitempty"`
	Children       []Finding `json:"children,omitempty"`
	AffectedRows   int64      `json:"affected_rows"`
	EstimatedCost float64    `json:"estimated_cost"`
	QueryPlanNode  *Operator `json:"query_plan_node,omitempty"`
	RuleName       string    `json:"rule_name"`
	RuleEnabled    bool      `json:"rule_enabled"`
	Category      string    `json:"category"`
	SubCategory   string    `json:"sub_category"`
	OperatorRefs  []OperatorRef `json:"operator_refs,omitempty"`
}

type OperatorRef struct {
	OperatorID  int    `json:"operator_id"`
	PhysicalOp  string `json:"physical_op"`
	ObjectName  string `json:"object_name"`
}

type Recommendation struct {
	ID           string `json:"id"`
	Type        string `json:"type"`
	Severity     Severity `json:"severity"`
	Title       string `json:"title"`
	Description string `json:"description"`
	SQL          string `json:"sql,omitempty"`
	Impact      string `json:"impact"`
	Effort      string `json:"effort"`
	Priority    int    `json:"priority"`
}

// OperatorStatsInfo holds statistics object info referenced by a plan operator.
type OperatorStatsInfo struct {
	Object            string  `json:"object"`
	LastUpdate        string  `json:"last_update"`
	ModificationCount int64   `json:"modification_count"`
	SamplingPercent   float64 `json:"sampling_percent"`
	TableCardinality  int64   `json:"table_cardinality"`
}

// Statement represents a single statement within a multi-statement batch plan.
type Statement struct {
	StatementID   string     `json:"statement_id"`
	StatementText string     `json:"statement_text"`
	SubTreeCost   float64    `json:"sub_tree_cost"`
	CostPercent   float64    `json:"cost_percent"`
	RootOperator  *Operator  `json:"root_operator,omitempty"`
	Operators     []Operator `json:"operators"`
}

type PlanAnalysis struct {
	Version             string           `json:"version"`
	Build               string           `json:"build"`
	Timestamp           time.Time        `json:"timestamp"`
	Metadata            QueryMetadata    `json:"metadata"`
	QueryPlan           *QueryPlan       `json:"query_plan"`
	Operators           []Operator       `json:"operators"`
	Warnings            []Warning        `json:"warnings"`
	MissingIndexes      []MissingIndex   `json:"missing_indexes"`
	Findings            []Finding        `json:"findings"`
	Recommendations     []Recommendation `json:"recommendations"`
	HealthScore         HealthScore      `json:"health_score"`
	TechnicalSummary    TechnicalSummary `json:"technical_summary"`
	ExecutiveSummary    ExecutiveSummary `json:"executive_summary"`
	CostSummary         CostSummary      `json:"cost_summary"`
	QueryNarrative      []string         `json:"query_narrative"`
	// Multi-statement batch support
	IsBatch             bool             `json:"is_batch"`
	Statements          []Statement      `json:"statements,omitempty"`
	// Plan-level parameter lists (for sniffing detection)
	CompiledParameters  []ParameterInfo  `json:"compiled_parameters,omitempty"`
	RuntimeParameters   []ParameterInfo  `json:"runtime_parameters,omitempty"`
	// Plan-level statistics objects
	StatisticsInfo      []OperatorStatsInfo `json:"statistics_info,omitempty"`
}

type BusinessExplanation struct {
	Status       string `json:"status"`
	Summary      string `json:"summary"`
	Problems     []string `json:"problems"`
	Impact       string `json:"impact"`
	Analogy      string `json:"analogy"`
	ActionItems  []string `json:"action_items"`
}

type TechnicalSummary struct {
	TopBottlenecks []Operator        `json:"top_bottlenecks"`
	Warnings      []Warning         `json:"warnings"`
	MissingIdx    []MissingIndex    `json:"missing_indexes"`
	Parallelism   ParallelismStats `json:"parallelism_stats"`
	Memory        MemoryStats     `json:"memory_stats"`
	Statistics    []string        `json:"statistics"`
}

type ParallelismStats struct {
	DegreeOfParallelism int       `json:"degree_of_parallelism"`
	ThreadDistribution []TCount  `json:"thread_distribution"`
	CXPACKETWait      int       `json:"cxpacket_wait"`
	ThreadSkew        float64   `json:"thread_skew"`
}

type TCount struct {
	ThreadID   int `json:"thread_id"`
	RowCount   int64 `json:"row_count"`
	CPUms      float64 `json:"cpu_ms"`
}

type MemoryStats struct {
	GrantRequested int `json:"grant_requested"`
	GrantUsed    int `json:"grant_used"`
	SpilledRows  int64 `json:"spilled_rows"`
}

type CostSummary struct {
	TotalEstimatedCost float64   `json:"total_estimated_cost"`
	CPUCost           float64   `json:"cpu_cost"`
	IOCost            float64   `json:"io_cost"`
	OperatorCount    int       `json:"operator_count"`
	TopOperators     []OperatorCost `json:"top_operators"`
}

type OperatorCost struct {
	ID          int     `json:"id"`
	Name       string  `json:"name"`
	CPUCost    float64 `json:"cpu_cost"`
	IOCost    float64 `json:"io_cost"`
	TotalCost float64 `json:"total_cost"`
	RowEstimate int64 `json:"row_estimate"`
	ActualRows int64  `json:"actual_rows"`
}

type ExecutiveSummary struct {
	HealthScore        int                  `json:"health_score"`
	Status            string               `json:"status"`
	Summary           string               `json:"summary"`
	PrimaryProblems   []string             `json:"primary_problems"`
	BusinessImpact    string              `json:"business_impact"`
	PlainEnglish      BusinessExplanation `json:"plain_english"`
	TrafficLight     string               `json:"traffic_light"`
}
