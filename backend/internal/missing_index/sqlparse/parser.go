// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: SQL parser for query analysis.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package sqlparse

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/rsharma155/sql_optima/internal/missing_index/types"
)

func Parse(ctx context.Context, query string) (*types.QueryAnalysis, error) {
	if strings.TrimSpace(query) == "" {
		return nil, nil
	}

	tables, aliases := extractTables(query)

	analysis := &types.QueryAnalysis{
		StatementType: detectStatementType(query),
		Tables:        tables,
		TableAliases:  aliases,
		Predicates:    extractPredicates(query, aliases),
		JoinInfo:      extractJoins(query, aliases),
		OrderBy:       extractOrderBy(query),
		ProjectedCols: extractProjectedColumns(query),
		Limit:         extractLimit(query),
	}

	return analysis, nil
}

func detectStatementType(query string) string {
	upper := strings.ToUpper(strings.TrimSpace(query))
	if strings.HasPrefix(upper, "SELECT") {
		return "SELECT"
	}
	return "unknown"
}

func extractTables(query string) ([]types.TableRef, map[string]types.TableRef) {
	fromPattern := regexp.MustCompile(`(?i)FROM\s+(\w+(?:\.\w+)?)(?:\s+(?:AS\s+)?(\w+))?`)
	matches := fromPattern.FindAllStringSubmatch(query, -1)

	var tables []types.TableRef
	seen := make(map[string]bool)
	aliases := make(map[string]types.TableRef)

	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		parts := strings.Split(match[1], ".")
		schema := "public"
		name := parts[len(parts)-1]

		if len(parts) > 1 {
			schema = parts[0]
		}

		key := schema + "." + name
		if !seen[key] {
			seen[key] = true
			tables = append(tables, types.TableRef{Schema: schema, Name: name})
		}

		if len(match) >= 3 && match[2] != "" {
			aliases[match[2]] = types.TableRef{Schema: schema, Name: name}
		}
	}

	joinTablePattern := regexp.MustCompile(`(?i)\s+JOIN\s+(\w+(?:\.\w+)?)(?:\s+(?:AS\s+)?(\w+))?\s+`)
	joinMatches := joinTablePattern.FindAllStringSubmatch(query, -1)

	for _, match := range joinMatches {
		if len(match) < 2 {
			continue
		}
		parts := strings.Split(match[1], ".")
		schema := "public"
		name := parts[len(parts)-1]

		if len(parts) > 1 {
			schema = parts[0]
		}

		key := schema + "." + name
		if !seen[key] {
			seen[key] = true
			tables = append(tables, types.TableRef{Schema: schema, Name: name})
		}

		if len(match) >= 3 && match[2] != "" {
			aliases[match[2]] = types.TableRef{Schema: schema, Name: name}
		}
	}

	return tables, aliases
}

func extractPredicates(query string, aliases map[string]types.TableRef) []types.Predicate {
	wherePattern := regexp.MustCompile(`(?i)WHERE\s+(.+?)(?:GROUP|ORDER|LIMIT|$)`)
	matches := wherePattern.FindAllStringSubmatch(query, -1)

	var predicates []types.Predicate

	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		whereClause := match[1]

		aliasColPattern := regexp.MustCompile(`(\w+)\.(\w+)\s*(?:=|<>|!=|<>|<)`)
		aliasMatches := aliasColPattern.FindAllStringSubmatch(whereClause, -1)
		for _, m := range aliasMatches {
			if len(m) >= 3 {
				alias := m[1]
				col := m[2]
				if tableRef, ok := aliases[alias]; ok {
					predicates = append(predicates, types.Predicate{
						Table:    tableRef,
						Column:   col,
						Type:     types.PredicateTypeEquality,
						Operator: "=",
					})
				}
			}
		}

		eqPattern := regexp.MustCompile(`(?:^|[^.\w])(\w+)\s*(?:=|<>|!=|<>|<)`)
		eqMatches := eqPattern.FindAllStringSubmatch(whereClause, -1)
		for _, m := range eqMatches {
			if len(m) >= 2 {
				predicates = append(predicates, types.Predicate{
					Column:   m[1],
					Type:     types.PredicateTypeEquality,
					Operator: "=",
				})
			}
		}

		rangePattern := regexp.MustCompile(`(\w+)\s*(?:>|<|>=|<=)`)
		rangeMatches := rangePattern.FindAllStringSubmatch(whereClause, -1)
		for _, m := range rangeMatches {
			if len(m) >= 2 {
				predicates = append(predicates, types.Predicate{
					Column:   m[1],
					Type:     types.PredicateTypeRange,
					Operator: ">",
				})
			}
		}

		isNullPattern := regexp.MustCompile(`(\w+)\s+IS\s+(?:NOT\s+)?NULL`)
		nullMatches := isNullPattern.FindAllStringSubmatch(whereClause, -1)
		for _, m := range nullMatches {
			if len(m) >= 2 {
				predicates = append(predicates, types.Predicate{
					Column: m[1],
					Type:   types.PredicateTypeIsNull,
				})
			}
		}
	}

	return predicates
}

func extractJoins(query string, aliases map[string]types.TableRef) []types.JoinInfo {
	joinPattern := regexp.MustCompile(`(?i)(INNER|LEFT|RIGHT|OUTER)?\s*JOIN\s+(\w+(?:\.\w+)?)\s+(?:AS\s+)?(\w+)?\s+ON\s+(.+?)(?:\s+JOIN|\s+WHERE|$)`)
	matches := joinPattern.FindAllStringSubmatch(query, -1)

	var joins []types.JoinInfo

	for _, match := range matches {
		if len(match) < 5 {
			continue
		}

		joinType := match[1]
		_ = joinType

		onClause := match[4]
		colPattern := regexp.MustCompile(`(\w+)\.(\w+)\s*=\s*(\w+)\.(\w+)`)
		colMatches := colPattern.FindAllStringSubmatch(onClause, -1)

		for _, m := range colMatches {
			if len(m) >= 5 {
				leftAlias := m[1]
				leftCol := m[2]
				rightAlias := m[3]
				rightCol := m[4]

				var leftTable, rightTable types.TableRef

				if t, ok := aliases[leftAlias]; ok {
					leftTable = t
				}
				if t, ok := aliases[rightAlias]; ok {
					rightTable = t
				}

				joins = append(joins, types.JoinInfo{
					LeftTable:  leftTable,
					RightTable: rightTable,
					Columns: []types.JoinColumn{
						{LeftCol: leftCol, RightCol: rightCol},
					},
				})
			}
		}
	}

	return joins
}

func extractOrderBy(query string) []types.OrderByColumn {
	orderPattern := regexp.MustCompile(`(?i)ORDER\s+BY\s+(.+?)(?:LIMIT|$)`)
	matches := orderPattern.FindAllStringSubmatch(query, -1)

	var orderBy []types.OrderByColumn

	for _, match := range matches {
		if len(match) < 2 {
			continue
		}

		colsStr := match[1]
		colPattern := regexp.MustCompile(`(\w+(?:\.\w+)?)(?:\s+(ASC|DESC))?`)
		colMatches := colPattern.FindAllStringSubmatch(colsStr, -1)

		for _, m := range colMatches {
			if len(m) >= 2 {
				colName := m[1]
				parts := strings.Split(colName, ".")
				if len(parts) > 1 {
					colName = parts[1]
				}

				descending := false
				if len(m) >= 3 && strings.ToUpper(m[2]) == "DESC" {
					descending = true
				}

				orderBy = append(orderBy, types.OrderByColumn{
					Column:     colName,
					Descending: descending,
				})
			}
		}
	}

	return orderBy
}

func extractProjectedColumns(query string) []string {
	selectPattern := regexp.MustCompile(`(?i)SELECT\s+(.+?)\s+FROM`)
	matches := selectPattern.FindAllStringSubmatch(query, -1)

	var cols []string

	for _, match := range matches {
		if len(match) < 2 {
			continue
		}

		selectList := match[1]
		if strings.TrimSpace(selectList) == "*" {
			return []string{"*"}
		}

		colPattern := regexp.MustCompile(`(?:\w+\.)?(\w+)(?:\s+AS\s+\w+)?`)
		colMatches := colPattern.FindAllStringSubmatch(selectList, -1)

		for _, m := range colMatches {
			if len(m) >= 2 && m[1] != "" && m[1] != "*" {
				cols = append(cols, m[1])
			}
		}
	}

	return cols
}

func extractLimit(query string) *int {
	limitPattern := regexp.MustCompile(`(?i)LIMIT\s+(\d+)`)
	matches := limitPattern.FindAllStringSubmatch(query, -1)

	if len(matches) > 0 && len(matches[0]) >= 2 {
		limit := strings.TrimSpace(matches[0][1])
		var n int
		if _, err := fmt.Sscanf(limit, "%d", &n); err == nil {
			return &n
		}
	}
	return nil
}
