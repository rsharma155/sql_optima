// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Dashboard filter normalization and SQL clause helpers for Storage/Index Health.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package hot

import (
	"fmt"
	"strings"
)

func normalizeCSV(xs []string) []string {
	out := make([]string, 0, len(xs))
	seen := make(map[string]struct{}, len(xs))
	for _, x := range xs {
		x = strings.TrimSpace(x)
		if x == "" {
			continue
		}
		if _, ok := seen[x]; ok {
			continue
		}
		seen[x] = struct{}{}
		out = append(out, x)
	}
	return out
}

// sihAppendFilters appends db/schema/table ILIKE clauses for dashboard queries. argStart is the next $N index.
func sihAppendFilters(where string, args []interface{}, f SIHFilters, argStart int) (string, []interface{}, int) {
	n := argStart
	if len(f.DBNames) > 0 {
		where += fmt.Sprintf(" AND db_name = ANY($%d)", n)
		args = append(args, f.DBNames)
		n++
	}
	if len(f.SchemaNames) > 0 {
		where += fmt.Sprintf(" AND schema_name = ANY($%d)", n)
		args = append(args, f.SchemaNames)
		n++
	}
	if f.TableLike != "" {
		where += fmt.Sprintf(" AND table_name ILIKE $%d", n)
		args = append(args, "%"+f.TableLike+"%")
		n++
	}
	return where, args, n
}

