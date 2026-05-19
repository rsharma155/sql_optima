// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Health Intelligence Engine
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package schema_parser


import (
	"regexp"
	"strings"

	"github.com/rsharma155/sql_optima/internal/intel/utils"
)

var timestampKeywords = []string{
	"timestamp", "datetime", "date", "collection_time", "capture_time",
	"sample_time", "snapshot_time", "created_at", "updated_at", "time",
}

var metricValueTypes = []string{
	"float", "double", "decimal", "numeric", "real", "int", "bigint",
	"smallint", "tinyint", "number",
}

var dimensionKeywords = []string{
	"server_id", "database_id", "instance_name", "host_name", "server_name",
	"db_name", "database_name", "machine_name", "instance_id", "host_id",
}

func ParseCreateTable(sql string) *TableInfo {
	normalized := strings.TrimSpace(strings.ToUpper(sql))
	if !strings.HasPrefix(normalized, "CREATE TABLE") {
		return nil
	}

	tableName := extractTableName(sql)
	if tableName == "" {
		return nil
	}

	info := &TableInfo{Name: tableName}
	columns := extractColumns(sql)
	pkCols := extractTableLevelPK(sql)

	for _, col := range columns {
		colInfo := ColumnInfo{
			Name:          col["name"],
			DataType:      col["dtype"],
			IsNullable:    col["nullable"] == "true",
			IsPrimaryKey:  col["primary_key"] == "true" || contains(pkCols, col["name"]),
			IsTimestamp:   isTimestampColumn(col["name"], col["dtype"]),
			IsMetricValue: isMetricValueColumn(col["name"], col["dtype"]),
			IsDimension:   isDimensionColumn(col["name"]),
		}
		if v, ok := col["max_length"]; ok && v != "" {
			if l, err := parseInt(v); err == nil {
				colInfo.MaxLength = &l
			}
		}

		if colInfo.IsPrimaryKey {
			info.PrimaryKeyColumns = append(info.PrimaryKeyColumns, colInfo.Name)
		}
		if colInfo.IsTimestamp {
			info.TimestampColumns = append(info.TimestampColumns, colInfo.Name)
		}
		if colInfo.IsMetricValue {
			info.ValueColumns = append(info.ValueColumns, colInfo.Name)
		}
		if colInfo.IsDimension {
			info.DimensionColumns = append(info.DimensionColumns, colInfo.Name)
		}
		info.Columns = append(info.Columns, colInfo)
	}

	info.InferredMetricType = inferMetricType(info)
	info.InferredOntologyClass = inferOntologyClass(info, tableName)
	return info
}

func ParseMultipleTables(sql string) []*TableInfo {
	statements := splitSQL(sql)
	var results []*TableInfo
	for _, stmt := range statements {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if parsed := ParseCreateTable(stmt); parsed != nil {
			results = append(results, parsed)
		}
	}
	return results
}

func extractTableName(stmt string) string {
	re := regexp.MustCompile(`(?i)TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?(?:\[([^\]]+)\]|` + "`" + `([^` + "`" + `]+)` + `|"([^"]+)"|(\w+))`)
	matches := re.FindStringSubmatch(stmt)
	if matches == nil {
		return ""
	}
	for _, g := range matches[1:] {
		if g != "" {
			return g
		}
	}
	return ""
}

func extractColumns(stmt string) []map[string]string {
	var columns []map[string]string
	lines := strings.Split(stmt, "\n")
	depth := 0

	constraintRe := regexp.MustCompile(`(?i)^\s*(?:CONSTRAINT|INDEX|PRIMARY\s+KEY|FOREIGN\s+KEY|UNIQUE|CHECK)\s`)
	columnRe := regexp.MustCompile(`(?i)^\s*\[?(\w+)\]?\s+(\w+(?:\s*\(\s*\d+(?:\s*,\s*\d+)?\s*\))?)`)

	for _, line := range lines {
		stripped := strings.TrimSpace(line)
		if stripped == "" {
			continue
		}
		upper := strings.ToUpper(stripped)
		if strings.HasPrefix(upper, "CREATE") || strings.HasPrefix(upper, ")") {
			continue
		}
		if stripped == "(" || stripped == ");" {
			continue
		}

		depth += strings.Count(stripped, "(") - strings.Count(stripped, ")")
		if depth > 0 {
			continue
		}
		if depth < 0 {
			depth = 0
			continue
		}

		if constraintRe.MatchString(stripped) {
			continue
		}

		m := columnRe.FindStringSubmatch(stripped)
		if m != nil {
			colName := m[1]
			colDtype := strings.ToUpper(m[2])
			rest := strings.ToUpper(stripped[len(m[0]):])

			col := map[string]string{
				"name":        colName,
				"dtype":       colDtype,
				"nullable":    "true",
				"primary_key": "false",
				"max_length":  "",
			}
			if strings.Contains(rest, "NOT NULL") {
				col["nullable"] = "false"
			}
			if strings.Contains(rest, "PRIMARY KEY") || strings.Contains(rest, "IDENTITY") {
				col["primary_key"] = "true"
			}
			lenRe := regexp.MustCompile(`\((\d+)\)`)
			if lm := lenRe.FindStringSubmatch(colDtype); lm != nil {
				col["max_length"] = lm[1]
			}
			columns = append(columns, col)
		}
	}
	return columns
}

func extractTableLevelPK(stmt string) []string {
	re := regexp.MustCompile(`(?i)PRIMARY\s+KEY\s*\(([^)]+)\)`)
	m := re.FindStringSubmatch(stmt)
	if m == nil {
		return nil
	}
	colsStr := m[1]
	parts := strings.Split(colsStr, ",")
	var result []string
	for _, p := range parts {
		c := strings.TrimSpace(p)
		c = strings.Trim(c, "[]`\"")
		result = append(result, c)
	}
	return result
}

func isTimestampColumn(name, dtype string) bool {
	dtypeUpper := strings.ToUpper(dtype)
	for _, t := range []string{"TIMESTAMP", "DATE", "DATETIME"} {
		if strings.Contains(dtypeUpper, t) {
			return true
		}
	}
	nameLower := strings.ToLower(name)
	for _, kw := range timestampKeywords {
		if strings.Contains(nameLower, kw) {
			return true
		}
	}
	return false
}

func isMetricValueColumn(name, dtype string) bool {
	dtypeUpper := strings.ToUpper(dtype)
	re := regexp.MustCompile(`\(.*\)`)
	baseType := strings.TrimSpace(re.ReplaceAllString(dtypeUpper, ""))
	found := false
	for _, t := range metricValueTypes {
		if strings.ToUpper(t) == baseType {
			found = true
			break
		}
	}
	if !found {
		return false
	}
	nameLower := strings.ToLower(name)
	skipKeywords := []string{"id", "name", "key", "hash"}
	for _, s := range skipKeywords {
		if strings.Contains(nameLower, s) {
			return false
		}
	}
	return true
}

func isDimensionColumn(name string) bool {
	nameLower := strings.ToLower(name)
	for _, d := range dimensionKeywords {
		if strings.Contains(nameLower, d) {
			return true
		}
	}
	return false
}

func inferMetricType(info *TableInfo) string {
	hasDiff := false
	hasCount := false
	for _, c := range info.Columns {
		cl := strings.ToLower(c.Name)
		if strings.Contains(cl, "diff") || strings.Contains(cl, "delta") {
			hasDiff = true
		}
		if strings.Contains(cl, "count") || strings.Contains(cl, "total") {
			hasCount = true
		}
	}
	if hasDiff {
		return "counter"
	}
	if hasCount {
		return "counter"
	}
	if len(info.TimestampColumns) > 0 && len(info.ValueColumns) > 0 {
		return "gauge"
	}
	return "snapshot"
}

func inferOntologyClass(info *TableInfo, tableName string) string {
	nameBased := utils.ClassifyMetricName(tableName)
	if nameBased != "other" {
		return capitalize(nameBased)
	}
	var colNames string
	for _, c := range info.Columns {
		colNames += " " + c.Name
	}
	return capitalize(utils.ClassifyMetricName(strings.TrimSpace(colNames)))
}

func capitalize(s string) string {
	if s == "" {
		return ""
	}
	m := map[string]string{
		"cpu": "CPU", "memory": "Memory", "disk": "Disk", "io": "IO",
		"query": "QueryPerformance", "blocking": "Blocking",
		"replication": "Replication", "backup": "Backup",
		"tempdb": "TempDB", "network": "Network", "index": "IndexHealth",
	}
	if v, ok := m[s]; ok {
		return v
	}
	if len(s) > 0 {
		return strings.ToUpper(s[:1]) + s[1:]
	}
	return s
}

func splitSQL(sql string) []string {
	var statements []string
	current := ""
	depth := 0
	for _, ch := range sql {
		switch ch {
		case ';':
			if depth == 0 {
				statements = append(statements, current)
				current = ""
			} else {
				current += string(ch)
			}
		case '(':
			depth++
			current += string(ch)
		case ')':
			depth--
			current += string(ch)
		default:
			current += string(ch)
		}
	}
	if strings.TrimSpace(current) != "" {
		statements = append(statements, current)
	}
	return statements
}

func contains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

func parseInt(s string) (int, error) {
	var n int
	for _, ch := range s {
		if ch >= '0' && ch <= '9' {
			n = n*10 + int(ch-'0')
		} else {
			break
		}
	}
	return n, nil
}
