// SQL Optima — MSSQL workload classification at enrichment time.
package enrichment

import (
	"strings"

	"github.com/rsharma155/sql_optima/internal/collectors/domain"
)

// classifyMSSQLWorkload mirrors dashboard read-time noise rules so metrics are stored correctly
// even when plan_handle session enrichment is missing or stale.
func isSQLServerInternalCatalogSQL(text string) bool {
	lower := strings.ToLower(text)
	if !strings.Contains(lower, "db_id()") {
		return false
	}
	return strings.Contains(lower, "system_type_id") ||
		strings.Contains(lower, "is_inlineable") ||
		strings.Contains(lower, "vector_column_count") ||
		strings.Contains(lower, "is_sparse") ||
		strings.Contains(lower, "is_column_set")
}

func classifyMSSQLWorkload(s domain.MSSQLQuerySnapshot, e *domain.MSSQLSessionEnrichment) int {
	raw := strings.ToLower(s.QueryTextRaw)
	stmt := strings.ToLower(s.StatementText)
	if strings.Contains(raw, "/* sql_optima") || strings.Contains(stmt, "/* sql_optima") {
		return 0
	}
	if strings.Contains(raw, "sys.dm_") || strings.Contains(stmt, "sys.dm_") {
		return 0
	}
	if isSQLServerInternalCatalogSQL(raw) || isSQLServerInternalCatalogSQL(stmt) {
		return 0
	}
	if e != nil {
		app := strings.ToLower(strings.TrimSpace(e.ApplicationName))
		if app == "sql-optima" || app == "go-mssqldb" || app == "sqlserverms" {
			return 0
		}
		return e.IsUserWorkload
	}
	return 0
}
