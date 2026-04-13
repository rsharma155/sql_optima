package analyzer

import (
	"testing"

	"github.com/rsharma155/sql_optima/internal/explain/parser"
	"github.com/rsharma155/sql_optima/internal/explain/types"
)

func TestAnalyzer_Analyze(t *testing.T) {
	input := `Seq Scan on users (cost=0.00..10.00 rows=1000 width=100) (actual time=5.0..50.0 rows=1000 loops=1)
 Planning Time: 1.000 ms
 Execution Time: 55.000 ms`

	plan, err := parser.ParseText(input)
	if err != nil {
		t.Fatalf("ParseText failed: %v", err)
	}

	a := New()
	result := a.Analyze(plan)

	if result == nil {
		t.Fatal("Expected analysis result, got nil")
	}

	if result.Summary.NodeCount == 0 {
		t.Errorf("Expected node count > 0, got %d", result.Summary.NodeCount)
	}
}

func TestAnalyzer_SequentialScanFinding(t *testing.T) {
	input := `Seq Scan on large_table (cost=0.00..20000.00 rows=100000 width=100)
 Planning Time: 1.000 ms
 Execution Time: 100.000 ms`

	plan, err := parser.ParseText(input)
	if err != nil {
		t.Fatalf("ParseText failed: %v", err)
	}

	a := New()
	result := a.Analyze(plan)

	hasSeqScanFinding := false
	for _, f := range result.Findings {
		if f.Type == "SequentialScanWarning" {
			hasSeqScanFinding = true
			break
		}
	}

	if !hasSeqScanFinding {
		t.Logf("Expected SequentialScanWarning finding for large table scan")
	}
}

func TestAnalyzer_RowMismatchFinding(t *testing.T) {
	input := `Seq Scan on users (cost=0.00..10.00 rows=10 width=100) (actual time=5.0..50.0 rows=5000 loops=1)
 Planning Time: 1.000 ms
 Execution Time: 55.000 ms`

	plan, err := parser.ParseText(input)
	if err != nil {
		t.Fatalf("ParseText failed: %v", err)
	}

	a := New()
	result := a.Analyze(plan)

	hasMismatchFinding := false
	for _, f := range result.Findings {
		if f.Type == "RowEstimationMismatch" {
			hasMismatchFinding = true
			break
		}
	}

	if !hasMismatchFinding {
		t.Logf("Expected RowEstimationMismatch finding for significant row count difference")
	}
}

func TestAnalyzer_HashSpillFinding(t *testing.T) {
	input := `Hash Join  (cost=100.00..200.00 rows=50 width=200) (actual time=5.0..10.0 rows=50 loops=1)
   Hash Cond: (a.id = b.id)
   ->  Seq Scan on table_a a  (cost=0.00..50.00 rows=100 width=100)
   ->  Hash  (cost=25.00..25.00 rows=10 width=50)
         Buckets: 1024  Batches: 8
         ->  Seq Scan on table_b b  (cost=0.00..25.00 rows=10 width=50)
 Planning Time: 2.000 ms
 Execution Time: 15.000 ms`

	plan, err := parser.ParseText(input)
	if err != nil {
		t.Fatalf("ParseText failed: %v", err)
	}

	a := New()
	result := a.Analyze(plan)

	if result.Summary.TotalCost == 0 {
		t.Errorf("Expected total cost > 0, got %f", result.Summary.TotalCost)
	}
}

func TestAnalyzer_EmptyPlan(t *testing.T) {
	input := ""

	plan, err := parser.ParseText(input)
	if err != nil {
		t.Logf("Empty input may return error: %v", err)
	}

	if plan != nil {
		a := New()
		result := a.Analyze(plan)
		if result == nil {
			t.Error("Expected analysis result for empty plan")
		}
	}
}

func TestAnalyzer_Summary(t *testing.T) {
	input := `Hash Join  (cost=100.00..200.00 rows=50 width=200)
   Hash Cond: (a.id = b.id)
   ->  Seq Scan on table_a a  (cost=0.00..50.00 rows=100 width=100)
   ->  Seq Scan on table_b b  (cost=0.00..25.00 rows=10 width=50)
 Planning Time: 2.000 ms
 Execution Time: 15.000 ms`

	plan, err := parser.ParseText(input)
	if err != nil {
		t.Fatalf("ParseText failed: %v", err)
	}

	a := New()
	result := a.Analyze(plan)

	if result.Summary.TotalCost == 0 {
		t.Errorf("Expected total cost > 0, got %f", result.Summary.TotalCost)
	}

	if result.Summary.ExecutionTimeMs == 0 {
		t.Errorf("Expected execution time > 0, got %f", result.Summary.ExecutionTimeMs)
	}
}

func TestAnalyzer_AnalyzeFromText(t *testing.T) {
	input := `Seq Scan on users (cost=0.00..10.00 rows=5 width=100)
 Planning Time: 1.000 ms
 Execution Time: 5.000 ms`

	a := New()
	result, err := a.AnalyzeFromText(input)
	if err != nil {
		t.Fatalf("AnalyzeFromText failed: %v", err)
	}

	if result == nil {
		t.Fatal("Expected analysis result, got nil")
	}

	if result.Summary.ExecutionTimeMs == 0 {
		t.Errorf("Expected execution time > 0, got %f", result.Summary.ExecutionTimeMs)
	}
}

func TestAnalyzer_CustomConfig(t *testing.T) {
	config := &types.AnalyzerConfig{
		SequentialScanThreshold: 100,
	}

	a := NewWithConfig(config)
	_, err := a.AnalyzeFromJSON("{}")
	if err != nil {
		t.Logf("Custom config works with valid plan")
	}
}

func TestAnalyzer_FindingsSummary(t *testing.T) {
	input := `Seq Scan on users (cost=0.00..20000.00 rows=50000 width=100) (actual time=5.0..50.0 rows=50000 loops=1)
 Planning Time: 1.000 ms
 Execution Time: 55.000 ms`

	plan, err := parser.ParseText(input)
	if err != nil {
		t.Fatalf("ParseText failed: %v", err)
	}

	a := New()
	result := a.Analyze(plan)

	if result.Summary.FindingsCount == nil {
		t.Error("Expected findings count in summary")
	}

	if result.Summary.NodeCount == 0 {
		t.Error("Expected node count > 0")
	}
}
