package parser

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rsharma155/sql_optima/internal/explain/types"
)

func TestParseJSON_PGMissingIndexRequestFile(t *testing.T) {
	path := filepath.Join("..", "..", "missing_index", "request.json")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Skip("sample not found (expected next to module): ", path, ": ", err)
	}
	plan, err := ParseJSON(string(b))
	if err != nil {
		t.Fatal(err)
	}
	if plan.Plan.NodeType != "Gather" {
		t.Fatalf("root node type: got %q want Gather", plan.Plan.NodeType)
	}
	if len(plan.Plan.Plans) != 1 {
		t.Fatalf("Gather children: %d", len(plan.Plan.Plans))
	}
	if plan.Plan.Plans[0].NodeType != "Seq Scan" || plan.Plan.Plans[0].RelationName != "users" {
		t.Fatalf("child: %+v", plan.Plan.Plans[0])
	}
	if plan.Query == "" {
		t.Fatal("expected query_text from wrapper copied into plan.Query")
	}
	if plan.ExecutionTime == 0 || plan.PlanningTime == 0 {
		t.Fatalf("times: planning=%v exec=%v", plan.PlanningTime, plan.ExecutionTime)
	}
}

func TestParseJSON_ComplexPlanArrayFile(t *testing.T) {
	path := filepath.Join("..", "..", "missing_index", "complext_plan.json")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Skip("sample not found: ", path, ": ", err)
	}
	plan, err := ParseJSON(string(b))
	if err != nil {
		t.Fatal(err)
	}
	if plan.Plan.NodeType != "Sort" {
		t.Fatalf("root: got %q want Sort", plan.Plan.NodeType)
	}
	if len(plan.Plan.SortKey) == 0 {
		t.Fatal("expected Sort Key array on root")
	}
	// Hash node: Peak Memory Usage -> memory_usage, Hash Buckets -> buckets
	var findHash func(n *types.PlanNode) (buckets, mem int, ok bool)
	findHash = func(n *types.PlanNode) (int, int, bool) {
		if n.NodeType == "Hash" && n.Buckets > 0 && n.MemoryUsage > 0 {
			return n.Buckets, n.MemoryUsage, true
		}
		for i := range n.Plans {
			if b, m, ok := findHash(&n.Plans[i]); ok {
				return b, m, ok
			}
		}
		return 0, 0, false
	}
	buckets, mem, ok := findHash(&plan.Plan)
	if !ok {
		t.Fatal("expected a Hash node with buckets and memory_usage from JSON aliases")
	}
	if buckets != 16384 && buckets != 131072 {
		t.Fatalf("unexpected buckets=%d", buckets)
	}
	if mem < 1 {
		t.Fatalf("memory_usage=%d", mem)
	}
}
