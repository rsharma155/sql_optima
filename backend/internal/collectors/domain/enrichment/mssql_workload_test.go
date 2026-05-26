package enrichment

import (
	"testing"

	"github.com/rsharma155/sql_optima/internal/collectors/domain"
)

func TestClassifyMSSQLWorkload_collectorDMV(t *testing.T) {
	s := domain.MSSQLQuerySnapshot{
		QueryTextRaw:  "SELECT * FROM sys.dm_exec_query_stats",
		StatementText: "SELECT TOP 500 FROM sys.dm_exec_query_stats qs",
	}
	if classifyMSSQLWorkload(s, nil) != 0 {
		t.Fatal("DMV-shaped SQL should not be user workload")
	}
}

func TestEnrichMSSQL_defaultsNonUserWithoutEnrichment(t *testing.T) {
	snaps := []domain.MSSQLQuerySnapshot{{
		QueryTextRaw:  "SELECT id FROM users",
		StatementText: "SELECT id FROM users",
		PlanHandle:    []byte{1},
	}}
	out := EnrichMSSQL(snaps, nil)
	if len(out) != 1 || out[0].IsUserWorkload != 0 {
		t.Fatalf("expected non-user default, got %d", out[0].IsUserWorkload)
	}
}

func TestClassifyMSSQLWorkload_internalCatalog(t *testing.T) {
	s := domain.MSSQLQuerySnapshot{
		StatementText: "SELECT db_id() AS database_id, c.system_type_id FROM sys.columns c",
	}
	if classifyMSSQLWorkload(s, nil) != 0 {
		t.Fatal("internal catalog SQL should not be user workload")
	}
}

func TestEnrichMSSQL_monitorAppFromEnrichment(t *testing.T) {
	snaps := []domain.MSSQLQuerySnapshot{{
		QueryTextRaw:  "SELECT id FROM users",
		StatementText: "SELECT id FROM users",
		PlanHandle:    []byte{1},
	}}
	enrich := []domain.MSSQLSessionEnrichment{{
		PlanHandle:      []byte{1},
		ApplicationName: "go-mssqldb",
		IsUserWorkload:  1,
	}}
	out := EnrichMSSQL(snaps, enrich)
	if out[0].IsUserWorkload != 0 {
		t.Fatal("go-mssqldb enrichment should be classified as system")
	}
}
