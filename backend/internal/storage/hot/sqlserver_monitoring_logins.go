// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: SQL fragments excluding monitoring collector logins at read time.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package hot

import (
	"strings"
)

var sqlServerDefaultExcludedLogins = []string{
	"dbmonitor_user", "dbmonitor", "sql-optima", "sql_optima", "sqloptima",
}

func mergeSqlServerExcludedLogins(monitoringLogins []string) []string {
	seen := make(map[string]struct{})
	add := func(name string) {
		name = strings.TrimSpace(strings.ToLower(name))
		if name != "" {
			seen[name] = struct{}{}
		}
	}
	for _, u := range sqlServerDefaultExcludedLogins {
		add(u)
	}
	for _, u := range monitoringLogins {
		add(u)
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	return out
}

// sqlServerMonitoringLoginExcludeSQL is retained for legacy callers; workload dashboards
// use is_user_workload instead of blanket login exclusion (see sqlServerUserWorkloadNoiseSQL).
func sqlServerMonitoringLoginExcludeSQL(col string, monitoringLogins []string) string {
	logins := mergeSqlServerExcludedLogins(monitoringLogins)
	if len(logins) == 0 {
		return ""
	}
	quoted := make([]string, len(logins))
	for i, l := range logins {
		quoted[i] = `'` + strings.ReplaceAll(l, `'`, `''`) + `'`
	}
	return `
		  AND LOWER(TRIM(COALESCE(` + col + `login_name, ''))) NOT IN (` + strings.Join(quoted, ", ") + `)`
}

// sqlServerMonitoringAppExcludeSQL excludes known monitoring application names.
func sqlServerMonitoringAppExcludeSQL(col string) string {
	return `
		  AND LOWER(TRIM(COALESCE(` + col + `application_name, ''))) NOT IN ('sql-optima', 'go-mssqldb', 'sqlserverms')`
}
