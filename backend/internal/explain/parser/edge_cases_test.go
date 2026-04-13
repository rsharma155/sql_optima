package parser

import (
	"encoding/json"
	"testing"

	"github.com/rsharma155/sql_optima/internal/explain/types"
)

func TestParseJSON_MultiElementTopArrayPicksFirstExplain(t *testing.T) {
	// Two statement results; first is valid EXPLAIN JSON.
	const s = `[
		{"Plan":{"Node Type":"Seq Scan","Relation Name":"a","Startup Cost":0,"Total Cost":1,"Plan Rows":1,"Plan Width":4},"Planning Time":0.1,"Execution Time":0.2},
		{"Plan":{"Node Type":"Result","Startup Cost":0,"Total Cost":0.01,"Plan Rows":0,"Plan Width":0},"Planning Time":0,"Execution Time":0}
	]`
	plan, err := ParseJSON(s)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Plan.NodeType != "Seq Scan" || plan.Plan.RelationName != "a" {
		t.Fatalf("%+v", plan.Plan)
	}
}

func TestParseJSON_EmptyIndexCondArray(t *testing.T) {
	const s = `{"Plan":{"Node Type":"Index Scan","Index Cond":[],"Startup Cost":0,"Total Cost":1,"Plan Rows":1,"Plan Width":4},"Planning Time":0,"Execution Time":0}`
	plan, err := ParseJSON(s)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Plan.IndexCond != "" {
		t.Fatalf("IndexCond=%q", plan.Plan.IndexCond)
	}
}

func TestParseJSON_SortKeyAsString(t *testing.T) {
	const s = `{"Plan":{"Node Type":"Sort","Sort Key":"x","Startup Cost":0,"Total Cost":1,"Plan Rows":1,"Plan Width":4},"Planning Time":0,"Execution Time":0}`
	plan, err := ParseJSON(s)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Plan.SortKey) != 1 || plan.Plan.SortKey[0] != "x" {
		t.Fatalf("%#v", plan.Plan.SortKey)
	}
}

func TestHoistSinglePlanWrapperChild(t *testing.T) {
	const s = `{
		"Plan": {
			"Node Type": "Limit",
			"Startup Cost": 0, "Total Cost": 10, "Plan Rows": 1, "Plan Width": 4,
			"Plans": [
				{
					"Plan": {
						"Node Type": "Seq Scan",
						"Relation Name": "t",
						"Startup Cost": 0, "Total Cost": 5, "Plan Rows": 1, "Plan Width": 4
					}
				}
			]
		},
		"Planning Time": 0, "Execution Time": 0
	}`
	plan, err := ParseJSON(s)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Plan.Plans) != 1 {
		t.Fatalf("children %d", len(plan.Plan.Plans))
	}
	ch := plan.Plan.Plans[0]
	if ch.NodeType != "Seq Scan" || ch.RelationName != "t" {
		t.Fatalf("%+v", ch)
	}
}

func TestParseJSON_NodetypeTypoKey(t *testing.T) {
	const s = `{"Plan":{"nodetype":"Seq Scan","relationname":"t","startupcost":0,"totalcost":3,"planrows":1,"planwidth":4},"PlanningTime":0,"ExecutionTime":0}`
	plan, err := ParseJSON(s)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Plan.NodeType != "Seq Scan" || plan.Plan.RelationName != "t" {
		t.Fatalf("%+v", plan.Plan)
	}
}

func TestParseJSON_CamelCaseKeys(t *testing.T) {
	// Some clients / serializers emit camelCase instead of PostgreSQL Title Case or snake_case.
	const s = `{"Plan":{"NodeType":"Seq Scan","RelationName":"users","StartupCost":0,"TotalCost":25.5,"PlanRows":1000,"PlanWidth":100},"PlanningTime":0.1,"ExecutionTime":2}`
	plan, err := ParseJSON(s)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Plan.NodeType != "Seq Scan" || plan.Plan.RelationName != "users" {
		t.Fatalf("%+v", plan.Plan)
	}
	if plan.Plan.TotalCost != 25.5 || plan.Plan.PlanRows != 1000 {
		t.Fatalf("%+v", plan.Plan)
	}
	if plan.PlanningTime != 0.1 || plan.ExecutionTime != 2 {
		t.Fatalf("times %v %v", plan.PlanningTime, plan.ExecutionTime)
	}
}

func TestNormalizeCoercesJSONNumber(t *testing.T) {
	root := map[string]interface{}{
		"Plan": map[string]interface{}{
			"Node Type":    "Seq Scan",
			"Startup Cost": json.Number("0"),
			"Total Cost":   json.Number("1"),
			"Plan Rows":    json.Number("10"),
			"Plan Width":   json.Number("4"),
		},
		"Planning Time":  json.Number("0"),
		"Execution Time": json.Number("0"),
	}
	norm := normalizeExplainValue(root)
	b, err := json.Marshal(norm)
	if err != nil {
		t.Fatal(err)
	}
	var plan types.Plan
	if err := json.Unmarshal(b, &plan); err != nil {
		t.Fatal(err)
	}
	if plan.Plan.PlanRows != 10 {
		t.Fatalf("rows=%d", plan.Plan.PlanRows)
	}
}
