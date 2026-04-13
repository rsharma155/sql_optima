package analyzer

import (
	"testing"

	"github.com/rsharma155/sql_optima/internal/explain/parser"
	"github.com/rsharma155/sql_optima/internal/explain/types"
)

func TestOptimizationAnalyzer_GenerateReport(t *testing.T) {
	input := `Seq Scan on users (cost=0.00..10.00 rows=1000 width=100) (actual time=5.0..50.0 rows=1000 loops=1)
  Planning Time: 1.000 ms
  Execution Time: 55.000 ms`

	plan, err := parser.ParseText(input)
	if err != nil {
		t.Fatalf("ParseText failed: %v", err)
	}

	a := New()
	result := a.Analyze(plan)

	optAnalyzer := NewOptimizationAnalyzer()
	report := optAnalyzer.GenerateOptimizationReport(plan, result.Findings)

	if report == nil {
		t.Fatal("Expected optimization report, got nil")
	}

	if report.PerformanceScore.Score < 0 || report.PerformanceScore.Score > 100 {
		t.Errorf("Performance score should be between 0 and 100, got %d", report.PerformanceScore.Score)
	}

	if report.PerformanceScore.Label == "" {
		t.Error("Performance score label should not be empty")
	}
}

func TestOptimizationAnalyzer_ExecutiveSummary(t *testing.T) {
	input := `Hash Join  (cost=100.00..200.00 rows=50 width=200) (actual time=5.0..100.0 rows=50 loops=1)
   Hash Cond: (a.id = b.id)
   ->  Seq Scan on table_a a  (cost=0.00..50.00 rows=100 width=100)
   ->  Hash  (cost=25.00..25.00 rows=10 width=50)
         Buckets: 1024  Batches: 8
         ->  Seq Scan on table_b b  (cost=0.00..25.00 rows=10 width=50)
  Planning Time: 2.000 ms
  Execution Time: 120.000 ms`

	plan, err := parser.ParseText(input)
	if err != nil {
		t.Fatalf("ParseText failed: %v", err)
	}

	a := New()
	result := a.Analyze(plan)

	optAnalyzer := NewOptimizationAnalyzer()
	report := optAnalyzer.GenerateOptimizationReport(plan, result.Findings)

	if report.ExecutiveSummary.TotalExecutionTime == "" {
		t.Error("Executive summary should have execution time")
	}

	if report.ExecutiveSummary.OverallDiagnosis == "" {
		t.Error("Executive summary should have overall diagnosis")
	}
}

func TestOptimizationAnalyzer_PerformanceScore_Labels(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Good performance",
			input:    "Seq Scan on small_table (cost=0.00..10.00 rows=10 width=100) (actual time=0.1..1.0 rows=10 loops=1)\n  Planning Time: 0.100 ms\n  Execution Time: 1.000 ms",
			expected: "good",
		},
		{
			name:     "Poor performance",
			input:    "Seq Scan on large_table (cost=0.00..100000.00 rows=1000000 width=100) (actual time=100.0..10000.0 rows=1000000 loops=1)\n  Planning Time: 10.000 ms\n  Execution Time: 10000.000 ms",
			expected: "poor",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := parser.ParseText(tt.input)
			if err != nil {
				t.Fatalf("ParseText failed: %v", err)
			}

			a := New()
			result := a.Analyze(plan)

			optAnalyzer := NewOptimizationAnalyzer()
			report := optAnalyzer.GenerateOptimizationReport(plan, result.Findings)

			if report.PerformanceScore.Label != tt.expected {
				t.Errorf("Expected label %s, got %s", tt.expected, report.PerformanceScore.Label)
			}
		})
	}
}

func TestOptimizationAnalyzer_TopIssues(t *testing.T) {
	input := `Seq Scan on users (cost=0.00..20000.00 rows=100000 width=100) (actual time=5.0..5000.0 rows=100000 loops=1)
  Planning Time: 1.000 ms
  Execution Time: 5000.000 ms`

	plan, err := parser.ParseText(input)
	if err != nil {
		t.Fatalf("ParseText failed: %v", err)
	}

	a := New()
	_ = a.Analyze(plan)

	optAnalyzer := NewOptimizationAnalyzer()
	flatNodes := optAnalyzer.flattenNodes(&plan.Plan)
	issues := optAnalyzer.identifyTopIssues(a.Analyze(plan).Findings, flatNodes)

	if len(issues) > 0 {
		if issues[0].Title == "" {
			t.Error("Issue title should not be empty")
		}

		if issues[0].Severity == "" {
			t.Error("Issue severity should not be empty")
		}
	}
}

func TestOptimizationAnalyzer_Recommendations(t *testing.T) {
	input := `Seq Scan on users (cost=0.00..20000.00 rows=100000 width=100) (actual time=5.0..5000.0 rows=100000 loops=1)
  Planning Time: 1.000 ms
  Execution Time: 5000.000 ms`

	plan, err := parser.ParseText(input)
	if err != nil {
		t.Fatalf("ParseText failed: %v", err)
	}

	a := New()
	_ = a.Analyze(plan)

	optAnalyzer := NewOptimizationAnalyzer()
	flatNodes := optAnalyzer.flattenNodes(&plan.Plan)
	recommendations := optAnalyzer.generateRecommendations(plan, a.Analyze(plan).Findings, flatNodes)

	for _, rec := range recommendations {
		if rec.Action == "" {
			t.Error("Recommendation action should not be empty")
		}
	}
}

func TestOptimizationAnalyzer_PlanAnalysis(t *testing.T) {
	input := `Hash Join  (cost=100.00..200.00 rows=50 width=200) (actual time=5.0..100.0 rows=50 loops=1)
   Hash Cond: (a.id = b.id)
   ->  Seq Scan on table_a a  (cost=0.00..50.00 rows=100 width=100) (actual time=1.0..10.0 rows=100 loops=1)
   ->  Hash  (cost=25.00..25.00 rows=10 width=50)
         ->  Seq Scan on table_b b  (cost=0.00..25.00 rows=10 width=50) (actual time=0.5..5.0 rows=10 loops=1)
  Planning Time: 2.000 ms
  Execution Time: 120.000 ms`

	plan, err := parser.ParseText(input)
	if err != nil {
		t.Fatalf("ParseText failed: %v", err)
	}

	a := New()
	_ = a.Analyze(plan)

	optAnalyzer := NewOptimizationAnalyzer()
	flatNodes := optAnalyzer.flattenNodes(&plan.Plan)
	planAnalysis := optAnalyzer.analyzePlan(plan, flatNodes)

	if len(planAnalysis.TimePerNode) == 0 {
		t.Error("Plan analysis should have time per node")
	}
}

func TestOptimizationAnalyzer_AdvancedDiagnostics(t *testing.T) {
	input := `Hash Join  (cost=100.00..200.00 rows=50 width=200) (actual time=5.0..100.0 rows=5000 loops=1)
   Hash Cond: (a.id = b.id)
   ->  Seq Scan on table_a a  (cost=0.00..50.00 rows=10 width=100) (actual time=1.0..10.0 rows=1000 loops=1)
   ->  Hash  (cost=25.00..25.00 rows=10 width=50)
         ->  Seq Scan on table_b b  (cost=0.00..25.00 rows=10 width=50) (actual time=0.5..5.0 rows=10 loops=1)
  Planning Time: 2.000 ms
  Execution Time: 120.000 ms`

	plan, err := parser.ParseText(input)
	if err != nil {
		t.Fatalf("ParseText failed: %v", err)
	}

	a := New()
	_ = a.Analyze(plan)

	optAnalyzer := NewOptimizationAnalyzer()
	flatNodes := optAnalyzer.flattenNodes(&plan.Plan)
	diagnostics := optAnalyzer.analyzeAdvancedDiagnostics(plan, flatNodes)

	if diagnostics.RowEstimationAccuracy.Mismatches == nil {
		t.Error("Row estimation accuracy should not be nil")
	}

	if diagnostics.JoinStrategyAnalysis.Nodes == nil {
		t.Error("Join strategy analysis should not be nil")
	}
}

func TestOptimizationAnalyzer_IndexAnalysis(t *testing.T) {
	input := `Seq Scan on users (cost=0.00..20000.00 rows=100000 width=100) (actual time=5.0..5000.0 rows=100000 loops=1)
  Filter: status = 'active'
  Planning Time: 1.000 ms
  Execution Time: 5000.000 ms`

	plan, err := parser.ParseText(input)
	if err != nil {
		t.Fatalf("ParseText failed: %v", err)
	}

	optAnalyzer := NewOptimizationAnalyzer()
	flatNodes := optAnalyzer.flattenNodes(&plan.Plan)
	indexAnalysis := optAnalyzer.analyzeIndexes(plan, flatNodes)

	if indexAnalysis.MissingIndexes == nil {
		t.Error("Index analysis should not be nil")
	}
}

func TestOptimizationAnalyzer_QueryImprovements(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		expected int
	}{
		{
			name:     "SELECT star",
			query:    "SELECT * FROM users WHERE id = 1",
			expected: 1,
		},
		{
			name:     "Offset pagination - no select star",
			query:    "SELECT id, name FROM users LIMIT 10 OFFSET 1000",
			expected: 1,
		},
		{
			name:     "Random order",
			query:    "SELECT * FROM users ORDER BY RANDOM() LIMIT 1",
			expected: 1,
		},
		{
			name:     "Clean query",
			query:    "SELECT id, name FROM users WHERE id = 1",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			optAnalyzer := NewOptimizationAnalyzer()
			improvements := optAnalyzer.analyzeQueryImprovements(tt.query)

			if len(improvements) != tt.expected {
				t.Errorf("Expected %d improvements, got %d", tt.expected, len(improvements))
			}
		})
	}
}

func TestOptimizationAnalyzer_ConfidenceScore(t *testing.T) {
	input := `Seq Scan on users (cost=0.00..10.00 rows=1000 width=100) (actual time=5.0..50.0 rows=1000 loops=1)
  Planning Time: 1.000 ms
  Execution Time: 55.000 ms`

	plan, err := parser.ParseText(input)
	if err != nil {
		t.Fatalf("ParseText failed: %v", err)
	}

	optAnalyzer := NewOptimizationAnalyzer()
	flatNodes := optAnalyzer.flattenNodes(&plan.Plan)
	confidence := optAnalyzer.calculateConfidenceScore(plan, flatNodes)

	if confidence.Level == "" {
		t.Error("Confidence level should not be empty")
	}

	if len(confidence.Factors) == 0 {
		t.Error("Confidence factors should not be empty")
	}
}

func TestOptimizationAnalyzer_ConfidenceScore_ExplicitHigh(t *testing.T) {
	input := `Seq Scan on users (cost=0.00..10.00 rows=1000 width=100) (actual time=5.0..50.0 rows=1000 loops=1)
  Planning Time: 1.000 ms
  Execution Time: 55.000 ms`

	plan, err := parser.ParseText(input)
	if err != nil {
		t.Fatalf("ParseText failed: %v", err)
	}

	optAnalyzer := NewOptimizationAnalyzer()
	flatNodes := optAnalyzer.flattenNodes(&plan.Plan)
	confidence := optAnalyzer.calculateConfidenceScore(plan, flatNodes)

	if confidence.Level != "high" {
		t.Errorf("Expected high confidence for plans with actual metrics, got %s", confidence.Level)
	}
}

func TestOptimizationAnalyzer_ParallelismAnalysis(t *testing.T) {
	input := `Parallel Seq Scan on users (cost=0.00..20000.00 rows=100000 width=100) (actual time=5.0..500.0 rows=100000 loops=4)
  Workers Launched: 4
  Planning Time: 1.000 ms
  Execution Time: 600.000 ms`

	plan, err := parser.ParseText(input)
	if err != nil {
		t.Fatalf("ParseText failed: %v", err)
	}

	optAnalyzer := NewOptimizationAnalyzer()
	flatNodes := optAnalyzer.flattenNodes(&plan.Plan)
	parallelism := optAnalyzer.analyzeParallelism(plan, flatNodes)

	if !parallelism.Used {
		t.Error("Expected parallelism to be detected as used")
	}

	if parallelism.WorkersLaunched == 0 {
		t.Error("Expected workers launched to be greater than 0")
	}
}

func TestOptimizationAnalyzer_IOvsCPUBound(t *testing.T) {
	input := `Seq Scan on users (cost=0.00..10.00 rows=1000 width=100) (actual time=5.0..50.0 rows=1000 loops=1)
  Buffers: shared hit=100 read=500
  Planning Time: 1.000 ms
  Execution Time: 55.000 ms`

	plan, err := parser.ParseText(input)
	if err != nil {
		t.Fatalf("ParseText failed: %v", err)
	}

	optAnalyzer := NewOptimizationAnalyzer()
	flatNodes := optAnalyzer.flattenNodes(&plan.Plan)
	ioVsCPU := optAnalyzer.analyzeIOvsCPU(flatNodes)

	if ioVsCPU.Reason == "" {
		t.Error("IO vs CPU analysis should have a reason")
	}
}

func TestOptimizationAnalyzer_FlattenNodes(t *testing.T) {
	input := `Hash Join  (cost=100.00..200.00 rows=50 width=200) (actual time=5.0..100.0 rows=50 loops=1)
   Hash Cond: (a.id = b.id)
   ->  Seq Scan on table_a a  (cost=0.00..50.00 rows=100 width=100)
   ->  Hash  (cost=25.00..25.00 rows=10 width=50)
         ->  Seq Scan on table_b b  (cost=0.00..25.00 rows=10 width=50)
  Planning Time: 2.000 ms
  Execution Time: 120.000 ms`

	plan, err := parser.ParseText(input)
	if err != nil {
		t.Fatalf("ParseText failed: %v", err)
	}

	optAnalyzer := NewOptimizationAnalyzer()
	nodes := optAnalyzer.flattenNodes(&plan.Plan)

	if len(nodes) == 0 {
		t.Error("Expected at least one node after flatten")
	}
}

func TestOptimizationAnalyzer_EmptyPlan(t *testing.T) {
	plan := &types.Plan{
		Plan:          types.PlanNode{},
		PlanningTime:  0,
		ExecutionTime: 0,
	}

	optAnalyzer := NewOptimizationAnalyzer()
	report := optAnalyzer.GenerateOptimizationReport(plan, nil)

	if report == nil {
		t.Fatal("Expected optimization report, got nil")
	}

	if report.PerformanceScore.Score > 100 || report.PerformanceScore.Score < 0 {
		t.Errorf("Performance score should be between 0 and 100, got %d", report.PerformanceScore.Score)
	}
}
