// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Common type definitions for missing index module.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package types

// RecommendationStatus represents the outcome of index analysis
type RecommendationStatus string

const (
	RecommendationStatusRecommended    RecommendationStatus = "recommended"
	RecommendationStatusNotRecommended RecommendationStatus = "not_recommended"
	RecommendationStatusError          RecommendationStatus = "error"
)

// IndexMethod represents the type of index to create
type IndexMethod string

const (
	IndexMethodBTree IndexMethod = "btree"
)

// PredicateType categorizes the type of filter predicate
type PredicateType string

const (
	PredicateTypeEquality PredicateType = "equality"
	PredicateTypeRange    PredicateType = "range"
	PredicateTypeJoin     PredicateType = "join"
	PredicateTypeIn       PredicateType = "in"
	PredicateTypeIsNull   PredicateType = "is_null"
	PredicateTypeBoolean  PredicateType = "boolean"
)

// IndexCandidate represents a potential index to recommend
type IndexCandidate struct {
	Table       TableRef
	IndexMethod IndexMethod
	KeyColumns  []IndexColumn
	IncludeCols []IndexColumn
	Reasoning   []string
	Score       float64
}

// IndexColumn represents a column in an index definition
type IndexColumn struct {
	Name       string
	Descending bool
	NullsFirst bool
}

// TableRef uniquely identifies a table in the database
type TableRef struct {
	Schema string
	Name   string
}

// QueryAnalysis contains the parsed SQL query information
type QueryAnalysis struct {
	StatementType string
	Tables        []TableRef
	TableAliases  map[string]TableRef
	Predicates    []Predicate
	JoinInfo      []JoinInfo
	OrderBy       []OrderByColumn
	ProjectedCols []string
	Limit         *int
}

// Predicate represents a WHERE clause condition
type Predicate struct {
	Table        TableRef
	Column       string
	Type         PredicateType
	Operator     string
	Value        any
	IsNullableOk bool
}

// JoinInfo represents a JOIN clause condition
type JoinInfo struct {
	LeftTable  TableRef
	RightTable TableRef
	Columns    []JoinColumn
	Type       string
}

// JoinColumn represents columns in a join condition
type JoinColumn struct {
	LeftCol  string
	RightCol string
}

// OrderByColumn represents an ORDER BY clause element
type OrderByColumn struct {
	Table      TableRef
	Column     string
	Descending bool
	NullsFirst bool
}

// PlanAnalysis contains the parsed execution plan information
type PlanAnalysis struct {
	RootNode      *PlanNode
	TotalCost     float64
	StartupCost   float64
	PlanRows      int64
	TargetTables  []TableRef
	Opportunities []TableOpportunity
}

// PlanNode represents a single node in the execution plan
type PlanNode struct {
	NodeType     string
	RelationName *string
	Alias        *string
	Schema       *string
	IndexName    *string
	IndexCond    []string
	Filter       *string
	HashCond     []string
	MergeCond    []string
	SortKey      []string
	StartupCost  float64
	TotalCost    float64
	PlanRows     int64
	PlanWidth    int64
	Children     []*PlanNode
}

// TableOpportunity represents an indexing opportunity for a specific table
type TableOpportunity struct {
	Table           TableRef
	ScanType        string
	FilterColumns   []string
	JoinColumns     []string
	OrderByColumns  []string
	CurrentCost     float64
	EstimatedRows   int64
	HasSortPressure bool
}

// TableStats contains statistical information about a table
type TableStats interface{}

// ColumnStats contains statistical information about a column
type ColumnStats struct {
	TableName       string
	ColumnName      string
	NDistinct       *float64
	MostCommonVals  []any
	MostCommonFreqs []float64
	HistogramBounds []any
	Correlation     *float64
	NullFrac        float64
}

// ExistingIndex represents an index that already exists on a table
type ExistingIndex struct {
	Table            TableRef
	IndexName        string
	IndexMethod      IndexMethod
	KeyColumns       []string
	IncludeColumns   []string
	IsUnique         bool
	IsPartial        bool
	PartialPredicate *string
}

// VerificationResult contains the result of HypoPG verification
type VerificationResult struct {
	Candidate        IndexCandidate
	OriginalCost     float64
	HypotheticalCost float64
	ImprovementPct   float64
	IndexUsedInPlan  bool
	PlanChanged      bool
	NewPlanJSON      string
	Success          bool
	Error            *string
	// HeuristicOnly is true when HypoPG was not used; scoring uses relaxed gates.
	HeuristicOnly bool
}

// ConfidenceResult contains the final scoring result
type ConfidenceResult struct {
	Confidence       float64
	IsRecommended    bool
	Reasoning        []string
	RejectionReasons []string
}

// DiagnosticInfo contains diagnostic information about the analysis
type DiagnosticInfo struct {
	ExistingIndexesChecked bool
	HypoPGAvailable        bool
	QuerySupported         bool
	ParsedPlan             bool
	ParsedSQL              bool
}

// AnalysisRequest represents the internal analysis request
type AnalysisRequest struct {
	DatabaseDSN       string
	QueryText         string
	ExecutionPlanJSON map[string]any
	QueryParams       []any
	Options           *RequestOptions
}

// RequestOptions contains optional parameters for the analysis
type RequestOptions struct {
	MaxCandidates      int
	MinImprovementPct  float64
	StatementTimeoutMs int
	IncludeColumns     bool
}

// AnalysisResult contains the result of index analysis
type AnalysisResult struct {
	Status        RecommendationStatus
	TopCandidate  *ScoredCandidate
	Alternatives  []ScoredCandidate
	Rejections    []Rejection
	Diagnostics   DiagnosticInfo
	DebugInfo     *DebugInfo
	QueryRewrites []QueryRewriteResult
}

// QueryRewriteResult contains the result of query rewrite
type QueryRewriteResult struct {
	OriginalQuery  string
	RewrittenQuery string
	AppliedRules   []string
}

// DebugInfo contains debug information
type DebugInfo struct {
	PlanTargetTables int
	QueryTables      int
	Opportunities    int
	Candidates       int
	Verified         int
	Scored           int
}

// ScoredCandidate represents a candidate with scoring info
type ScoredCandidate struct {
	Table            TableRef
	IndexMethod      IndexMethod
	IndexStatement   string
	Confidence       float64
	Reasoning        []string
	OriginalCost     float64
	HypotheticalCost float64
	ImprovementPct   float64
	PlanChanged      bool
	IndexUsed        bool
}

// Rejection represents a rejected candidate with reason
type Rejection struct {
	Candidate IndexCandidate
	Reason    string
}

type OptimizationResult struct {
	OptimizedSQL string
	Indexes      []IndexCandidate
	Cost         float64
	AppliedRules []string
}

type CombinedCandidate struct {
	Query         string
	Indexes       []IndexCandidate
	Rewrites      []string
	EstimatedCost float64
}

type JoinOrderResult struct {
	JoinOrder           []string
	JoinMethods         []string
	EstimatedCost       float64
	RowExplosionPenalty float64
	Confidence          float64
	PolicyUsed          string
}

type JoinGraph struct {
	Tables  []TableRef
	Edges   []JoinEdge
	Filters []Predicate
}

type JoinEdge struct {
	LeftTable   TableRef
	RightTable  TableRef
	Columns     []JoinColumn
	Selectivity float64
}

type QueryEmbedding struct {
	ID         string
	QueryHash  string
	Vector     []float64
	JoinOrder  []string
	Indexes    []IndexCandidate
	AvgLatency float64
	ExecCount  int64
	LastSeen   int64
}

type SimilarityResult struct {
	QueryID          string
	Similarity       float64
	ReusedJoinOrder  []string
	ReusedIndexes    []IndexCandidate
	JoinOrderChanged bool
	IndexChangeCount int
}

type FeedbackRecord struct {
	QueryID       string
	QueryText     string
	ActualLatency float64
	RowsProcessed int64
	IndexesUsed   []string
	MemoryUsageMB float64
	PlanUsed      string
	JoinOrder     []string
	Timestamp     int64
}

type OptimizationResultV2 struct {
	OptimizedSQL      string
	JoinOrder         []string
	Indexes           []IndexCandidate
	ReusedPlan        bool
	SimilarQueryID    string
	EstimatedCost     float64
	AppliedStrategies []string
	Confidence        float64
}
