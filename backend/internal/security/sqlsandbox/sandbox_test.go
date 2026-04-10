package sqlsandbox

import (
	"strings"
	"testing"
)

func TestValidateReadOnly_Postgres(t *testing.T) {
	err := ValidateReadOnly(Options{Dialect: "postgres"}, "SELECT 1")
	if err != nil {
		t.Fatal(err)
	}
	if ValidateReadOnly(Options{Dialect: "postgres"}, "DELETE FROM t") == nil {
		t.Fatal("expected error")
	}
	if ValidateReadOnly(Options{Dialect: "postgres"}, "WITH x AS (SELECT 1) SELECT * FROM x") != nil {
		t.Fatal("expected CTE ok")
	}
}

func TestWrapWithRowLimit(t *testing.T) {
	out, err := WrapWithRowLimit("postgres", "SELECT n FROM generate_series(1,3) n", 10)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "LIMIT 10") {
		t.Fatalf("got %q", out)
	}
}
