// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Health Intelligence Engine
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package ontology




import (
	"strings"

	"github.com/rsharma155/sql_optima/internal/intel/schema_parser"
)

func BuildOntology(sqlScript string) *OntologyGraph {
	tables := schema_parser.ParseMultipleTables(sqlScript)
	graph := NewOntologyGraph()

	for _, table := range tables {
		mappedOntology := mapTableToOntology(table)

		for _, col := range table.Columns {
			colOntology := mappedOntology
			if v, ok := ColumnOntologyMap[col.Name]; ok {
				colOntology = v
			}
			mapping := MetricMapping{
				SourceTable:   table.Name,
				SourceColumn:  col.Name,
				OntologyClass: colOntology,
				MetricName:    deriveMetricName(table.Name, col.Name),
				MetricType:    table.InferredMetricType,
				IsValue:       col.IsMetricValue,
				IsDimension:   col.IsDimension,
				IsTimestamp:   col.IsTimestamp,
			}
			graph.AddMapping(mapping)
		}
	}
	return graph
}

func BuildOntologyFromTableList(tableNames []string) *OntologyGraph {
	graph := NewOntologyGraph()
	for _, name := range tableNames {
		ontology := "Other"
		if v, ok := TableToOntology[name]; ok {
			ontology = v
		}
		graph.AddMapping(MetricMapping{
			SourceTable:   name,
			SourceColumn:  "*",
			OntologyClass: ontology,
			MetricName:    name + ".*",
			MetricType:    "gauge",
			IsValue:       true,
		})
	}
	return graph
}

func mapTableToOntology(table *schema_parser.TableInfo) string {
	if v, ok := TableToOntology[table.Name]; ok {
		return v
	}
	parts := strings.Split(table.Name, ".")
	if v, ok := TableToOntology[parts[len(parts)-1]]; ok {
		return v
	}
	if table.InferredOntologyClass != "other" && table.InferredOntologyClass != "" {
		return normalizeClassName(table.InferredOntologyClass)
	}

	candidateScores := make(map[string]int)
	for cls, signatures := range OntologySignatures {
		score := 0
		for _, col := range table.Columns {
			colLower := strings.ToLower(col.Name)
			for _, sig := range signatures {
				if strings.Contains(colLower, sig) {
					score += 2
				}
			}
		}
		nameLower := strings.ToLower(table.Name)
		for _, sig := range signatures {
			if strings.Contains(nameLower, sig) {
				score += 3
			}
		}
		if score > 0 {
			candidateScores[cls] = score
		}
	}

	if len(candidateScores) > 0 {
		bestCls := ""
		bestScore := 0
		for cls, score := range candidateScores {
			if score > bestScore {
				bestScore = score
				bestCls = cls
			}
		}
		return bestCls
	}
	return "Other"
}

func deriveMetricName(tableName, columnName string) string {
	return tableName + "." + columnName
}

func normalizeClassName(name string) string {
	mapping := map[string]string{
		"cpu": "CPU", "memory": "Memory", "disk": "Disk", "io": "IO",
		"query": "QueryPerformance", "blocking": "Blocking",
		"replication": "Replication", "backup": "Backup",
		"tempdb": "TempDB", "network": "Network", "index": "IndexHealth",
	}
	if v, ok := mapping[strings.ToLower(name)]; ok {
		return v
	}
	if len(name) > 0 {
		return strings.ToUpper(name[:1]) + name[1:]
	}
	return name
}