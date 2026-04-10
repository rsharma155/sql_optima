package explain

import (
	"testing"

	"github.com/yourorg/pg_explain_analyze/types"
)

func TestExtractIndexColumns(t *testing.T) {
	cols := extractIndexColumns(`(status = 'active'::text) AND (id > 10)`)
	if len(cols) < 1 || cols[0] != "status" {
		t.Fatalf("got %v", cols)
	}
}

func TestSuggestIndexDDL(t *testing.T) {
	sql := suggestIndexDDL(&types.PlanNode{
		NodeType:     "Seq Scan",
		RelationName: "users",
		Filter:       "(status = 'active'::text)",
	})
	if sql == "" || len(sql) < 30 {
		t.Fatalf("expected DDL, got %q", sql)
	}
}
