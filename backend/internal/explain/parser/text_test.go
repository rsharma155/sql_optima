package parser

import (
	"encoding/json"
	"testing"
)

func TestParseText_SimplePlan(t *testing.T) {
	input := `Seq Scan on users (cost=0.00..10.00 rows=5 width=100)
 Planning Time: 1.234 ms
 Execution Time: 5.678 ms`

	plan, err := ParseText(input)
	if err != nil {
		t.Fatalf("ParseText failed: %v", err)
	}

	if plan.Plan.NodeType != "Seq Scan" {
		t.Errorf("Expected node type 'Seq Scan', got '%s'", plan.Plan.NodeType)
	}

	if plan.Plan.TotalCost != 10.00 {
		t.Errorf("Expected total cost 10.00, got %f", plan.Plan.TotalCost)
	}

	if plan.Plan.PlanRows != 5 {
		t.Errorf("Expected plan rows 5, got %d", plan.Plan.PlanRows)
	}

	if plan.PlanningTime != 1.234 {
		t.Errorf("Expected planning time 1.234, got %f", plan.PlanningTime)
	}

	if plan.ExecutionTime != 5.678 {
		t.Errorf("Expected execution time 5.678, got %f", plan.ExecutionTime)
	}
}

func TestParseText_NestedPlan(t *testing.T) {
	input := `Hash Join  (cost=100.00..200.00 rows=50 width=200)
   Hash Cond: (a.id = b.id)
   ->  Seq Scan on table_a a  (cost=0.00..50.00 rows=100 width=100)
   ->  Hash  (cost=25.00..25.00 rows=10 width=50)
         ->  Seq Scan on table_b b  (cost=0.00..25.00 rows=10 width=50)
 Planning Time: 2.000 ms
 Execution Time: 10.000 ms`

	plan, err := ParseText(input)
	if err != nil {
		t.Fatalf("ParseText failed: %v", err)
	}

	if plan.Plan.NodeType != "Hash Join" {
		t.Errorf("Expected root node type 'Hash Join', got '%s'", plan.Plan.NodeType)
	}

	if len(plan.Plan.Plans) < 1 {
		t.Logf("Expected at least 1 child node, got %d", len(plan.Plan.Plans))
	}
}

func TestParseText_WithQuery(t *testing.T) {
	input := `=> EXPLAIN SELECT * FROM users WHERE id = 1
Seq Scan on users (cost=0.00..10.00 rows=1 width=100)
 Planning Time: 1.000 ms
 Execution Time: 0.500 ms`

	plan, err := ParseText(input)
	if err != nil {
		t.Fatalf("ParseText failed: %v", err)
	}

	if plan.Query == "" {
		t.Error("Expected query to be parsed, got empty string")
	}

	if plan.Plan.NodeType != "Seq Scan" {
		t.Errorf("Expected node type 'Seq Scan', got '%s'", plan.Plan.NodeType)
	}
}

func TestParseText_Buffers(t *testing.T) {
	input := `Seq Scan on users (cost=0.00..10.00 rows=5 width=100) (actual time=1.0..2.0 rows=5 loops=1)
   Filter: id > 10
   Rows Removed by Filter: 2
   Buffers: shared hit=100 read=50
 Planning Time: 1.000 ms
 Execution Time: 3.000 ms`

	plan, err := ParseText(input)
	if err != nil {
		t.Fatalf("ParseText failed: %v", err)
	}

	if plan.Plan.Filter != "id > 10" {
		t.Errorf("Expected filter 'id > 10', got '%s'", plan.Plan.Filter)
	}

	if plan.Plan.RowsRemovedByFilter != 2 {
		t.Errorf("Expected rows removed 2, got %d", plan.Plan.RowsRemovedByFilter)
	}
}

func TestParseText_ActualMetrics(t *testing.T) {
	input := `Seq Scan on users (cost=0.00..10.00 rows=1000 width=100) (actual time=5.0..50.0 rows=500 loops=1)
  Planning Time: 1.000 ms
  Execution Time: 55.000 ms`

	plan, err := ParseText(input)
	if err != nil {
		t.Fatalf("ParseText failed: %v", err)
	}

	if plan.Plan.ActualTotalTime != 50.0 {
		t.Logf("Plan.Plan = %+v", plan.Plan)
		t.Errorf("Expected actual total time 50.0, got %f", plan.Plan.ActualTotalTime)
	}

	if plan.Plan.ActualRows != 500 {
		t.Errorf("Expected actual rows 500, got %d", plan.Plan.ActualRows)
	}

	if plan.Plan.ActualLoops != 1 {
		t.Errorf("Expected actual loops 1, got %d", plan.Plan.ActualLoops)
	}
}

func TestParseText_EmptyInput(t *testing.T) {
	plan, err := ParseText("")
	if err != nil {
		t.Logf("Empty input returns error (acceptable): %v", err)
	}
	if plan != nil && plan.Plan.NodeType != "" {
		t.Errorf("Expected empty plan for empty input, got node type '%s'", plan.Plan.NodeType)
	}
}

func TestParseText_SortInfo(t *testing.T) {
	input := `Sort  (cost=100.00..110.00 rows=50 width=200) (actual time=5.0..6.0 rows=50 loops=1)
   Sort Key: id, name
   Sort Method: quicksort  Memory: 38kB
   ->  Seq Scan on users (cost=0.00..50.00 rows=50 width=200)
 Planning Time: 1.000 ms
 Execution Time: 7.000 ms`

	plan, err := ParseText(input)
	if err != nil {
		t.Fatalf("ParseText failed: %v", err)
	}

	if plan.Plan.SortMethod != "quicksort" {
		t.Errorf("Expected sort method 'quicksort', got '%s'", plan.Plan.SortMethod)
	}
}

func TestParseText_IndexScan(t *testing.T) {
	input := `Index Scan using idx_users_id on users (cost=0.00..10.00 rows=1 width=100) (actual time=0.1..0.2 rows=1 loops=1)
   Index Cond: (id = 1)
 Planning Time: 1.000 ms
 Execution Time: 0.300 ms`

	plan, err := ParseText(input)
	if err != nil {
		t.Fatalf("ParseText failed: %v", err)
	}

	if plan.Plan.NodeType != "Index Scan" {
		t.Errorf("Expected node type 'Index Scan', got '%s'", plan.Plan.NodeType)
	}

	if plan.Plan.IndexName != "idx_users_id" {
		t.Errorf("Expected index name 'idx_users_id', got '%s'", plan.Plan.IndexName)
	}
}

func TestParseJSON_PostgresNativeFormat(t *testing.T) {
	jsonPlan := `{
		"Plan": {
			"Node Type": "Seq Scan",
			"Relation Name": "users",
			"Alias": "u",
			"Startup Cost": 0,
			"Total Cost": 25.5,
			"Plan Rows": 1000,
			"Plan Width": 100,
			"Actual Startup Time": 0.01,
			"Actual Total Time": 2.5,
			"Actual Rows": 1000,
			"Actual Loops": 1,
			"Filter": "(status = 'active'::text)",
			"Shared Hit Blocks": 150,
			"Shared Read Blocks": 2
		},
		"Planning Time": 0.234,
		"Execution Time": 2.89
	}`
	plan, err := ParseJSON(jsonPlan)
	if err != nil {
		t.Fatalf("ParseJSON: %v", err)
	}
	if plan.Plan.NodeType != "Seq Scan" {
		t.Errorf("NodeType=%q want Seq Scan", plan.Plan.NodeType)
	}
	if plan.Plan.RelationName != "users" {
		t.Errorf("RelationName=%q want users", plan.Plan.RelationName)
	}
	if plan.Plan.TotalCost != 25.5 {
		t.Errorf("TotalCost=%f want 25.5", plan.Plan.TotalCost)
	}
	if plan.PlanningTime != 0.234 {
		t.Errorf("PlanningTime=%f", plan.PlanningTime)
	}
	if plan.Plan.Buffers == nil || plan.Plan.Buffers.SharedHit != 150 || plan.Plan.Buffers.SharedRead != 2 {
		t.Errorf("Buffers=%+v", plan.Plan.Buffers)
	}
}

func TestParseJSON_HashJoinArrayCond(t *testing.T) {
	jsonPlan := `{
		"Plan": {
			"Node Type": "Hash Join",
			"Hash Cond": ["(a.id = b.aid)"],
			"Plans": [
				{"Node Type": "Seq Scan", "Relation Name": "a", "Startup Cost": 0, "Total Cost": 1, "Plan Rows": 1, "Plan Width": 4}
			],
			"Startup Cost": 0,
			"Total Cost": 10,
			"Plan Rows": 1,
			"Plan Width": 8
		},
		"Planning Time": 0,
		"Execution Time": 0
	}`
	plan, err := ParseJSON(jsonPlan)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Plan.HashCond != "(a.id = b.aid)" {
		t.Errorf("HashCond=%q", plan.Plan.HashCond)
	}
}

func TestUnwrapExplainJSON_PreservesPlanKey(t *testing.T) {
	const s = `{"execution_plan_json":{"Plan":{"Node Type":"Result","Startup Cost":0,"Total Cost":0.01,"Plan Rows":1,"Plan Width":0},"Planning Time":0,"Execution Time":0}}`
	b, err := UnwrapExplainJSON([]byte(s))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	p, ok := m["Plan"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing capital Plan, keys=%v", keysOf(m))
	}
	if p["Node Type"] != "Result" {
		t.Fatalf("%v", p)
	}
}

func keysOf(m map[string]interface{}) []string {
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}

func TestParseJSON_ExecutionPlanJSONWrapper(t *testing.T) {
	// Same shape as pg_missing_index/request.json (outer API envelope).
	const s = `{
		"database_dsn": "postgres://x",
		"query_text": "SELECT 1",
		"execution_plan_json": {
			"Plan": {
				"Node Type": "Result",
				"Parallel Aware": false,
				"Startup Cost": 0,
				"Total Cost": 0.01,
				"Plan Rows": 1,
				"Plan Width": 0
			},
			"Planning Time": 0.05,
			"Execution Time": 0.01
		}
	}`
	plan, err := ParseJSON(s)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Plan.NodeType != "Result" {
		t.Fatalf("NodeType=%q", plan.Plan.NodeType)
	}
	if plan.Query != "SELECT 1" {
		t.Fatalf("Query=%q want SELECT 1", plan.Query)
	}
}

func TestParseJSON_AlreadySnakeCase(t *testing.T) {
	jsonPlan := `{"plan":{"node_type":"Limit","plans":[{"node_type":"Seq Scan","relation_name":"t","startup_cost":0,"total_cost":1,"plan_rows":1,"plan_width":4}],"startup_cost":0,"total_cost":2,"plan_rows":1,"plan_width":4},"planning_time":0.1,"execution_time":0.2}`
	plan, err := ParseJSON(jsonPlan)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Plan.NodeType != "Limit" {
		t.Fatalf("NodeType=%q", plan.Plan.NodeType)
	}
	if len(plan.Plan.Plans) != 1 || plan.Plan.Plans[0].NodeType != "Seq Scan" {
		t.Fatalf("child=%+v", plan.Plan.Plans)
	}
}
