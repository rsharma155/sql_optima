// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Duplicate / overlapping index candidate detection for Storage/Index Health.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package hot

import (
	"context"
	"fmt"
	"strings"
)

func (tl *TimescaleLogger) sihDuplicateIndexCandidates(ctx context.Context, engine, serverID, to string, f SIHFilters) ([]map[string]interface{}, error) {
	// Pull latest definitions per index (day bucket) in scope and do pairwise comparisons in Go.
	args := []interface{}{engine, serverID, to}
	where := `engine=$1 AND server_id=$2 AND time <= $3::timestamptz`
	n := 4
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
	}

	q := `
		SELECT db_name, schema_name, table_name, index_name, key_columns, include_columns, filter_definition, COALESCE(is_pk,false), COALESCE(is_unique,false)
		FROM (
			SELECT DISTINCT ON (db_name, schema_name, table_name, index_name)
				db_name, schema_name, table_name, index_name, key_columns, include_columns, filter_definition, is_pk, is_unique, time
			FROM monitor.index_definitions
			WHERE ` + where + `
			ORDER BY db_name, schema_name, table_name, index_name, time DESC
		) t
	`
	rows, err := tl.pool.Query(ctx, q, args...)
	if err != nil {
		if isMissingRelation(err) {
			return nil, schemaMissingErr("monitor.index_definitions")
		}
		return nil, err
	}
	defer rows.Close()

	type def struct {
		db, sch, tbl, idx string
		key, inc, filter  string
		isPK, isUnique    bool
	}
	byTable := make(map[string][]def)
	for rows.Next() {
		var d def
		if err := rows.Scan(&d.db, &d.sch, &d.tbl, &d.idx, &d.key, &d.inc, &d.filter, &d.isPK, &d.isUnique); err == nil {
			k := d.db + "|" + d.sch + "|" + d.tbl
			byTable[k] = append(byTable[k], d)
		}
	}

	dup := make([]map[string]interface{}, 0, 24)

	splitCols := func(s string) []string {
		parts := strings.Split(s, ",")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				out = append(out, strings.ToLower(p))
			}
		}
		return out
	}
	prefixMatchPct := func(a, b []string) float64 {
		if len(a) == 0 || len(b) == 0 {
			return 0
		}
		min := len(a)
		if len(b) < min {
			min = len(b)
		}
		m := 0
		for i := 0; i < min; i++ {
			if a[i] == b[i] {
				m++
			} else {
				break
			}
		}
		den := len(a)
		if len(b) > den {
			den = len(b)
		}
		return (float64(m) / float64(den)) * 100.0
	}

	for k, defs := range byTable {
		for i := 0; i < len(defs); i++ {
			for j := i + 1; j < len(defs); j++ {
				a := defs[i]
				b := defs[j]
				if a.idx == b.idx {
					continue
				}
				// skip PK indexes
				if a.isPK || b.isPK {
					continue
				}

				// identical keys+includes+filter
				if strings.EqualFold(strings.TrimSpace(a.key), strings.TrimSpace(b.key)) &&
					strings.EqualFold(strings.TrimSpace(a.inc), strings.TrimSpace(b.inc)) &&
					strings.EqualFold(strings.TrimSpace(a.filter), strings.TrimSpace(b.filter)) {
					parts := strings.Split(k, "|")
					dup = append(dup, map[string]interface{}{
						"db_name": parts[0], "schema_name": parts[1], "table_name": parts[2],
						"index1": a.idx, "index2": b.idx,
						"key_match_pct": 100.0,
						"suggestion":    "Duplicate index definitions; consider dropping one after validating usage.",
					})
					continue
				}

				// overlapping (leading columns)
				ap := splitCols(a.key)
				bp := splitCols(b.key)
				pct := prefixMatchPct(ap, bp)
				if pct >= 50.0 && pct > 0 {
					parts := strings.Split(k, "|")
					dup = append(dup, map[string]interface{}{
						"db_name": parts[0], "schema_name": parts[1], "table_name": parts[2],
						"index1": a.idx, "index2": b.idx,
						"key_match_pct": pct,
						"suggestion":    "Overlapping leading key columns; review consolidation opportunities.",
					})
				}
			}
		}
	}

	// cap
	if len(dup) > 100 {
		dup = dup[:100]
	}
	return dup, nil
}
