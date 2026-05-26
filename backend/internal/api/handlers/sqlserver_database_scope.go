// SQL Optima — database scope for SQL Server workload / query-analysis APIs.
package handlers

import (
	"net/http"
	"strings"
)

// ParseSQLServerDatabaseScope interprets the database query parameter.
//
//   - Param omitted: autoPick=true → handler may pick the busiest database in range.
//   - Param present as "" or "all": autoPick=false, database="" → all databases.
//   - Param present as a name: autoPick=false, database set → that database only.
func ParseSQLServerDatabaseScope(r *http.Request) (database string, autoPick bool) {
	vals, ok := r.URL.Query()["database"]
	if !ok {
		return "", true
	}
	if len(vals) == 0 {
		return "", false
	}
	db := strings.TrimSpace(vals[0])
	if db == "" || strings.EqualFold(db, "all") {
		return "", false
	}
	return db, false
}
