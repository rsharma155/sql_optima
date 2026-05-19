// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Health Intelligence Engine
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package schema_parser




type ColumnInfo struct {
	Name           string `json:"name"`
	DataType       string `json:"dtype"`
	IsNullable     bool   `json:"is_nullable"`
	IsPrimaryKey   bool   `json:"is_primary_key"`
	IsTimestamp    bool   `json:"is_timestamp"`
	IsMetricValue  bool   `json:"is_metric_value"`
	IsDimension    bool   `json:"is_dimension"`
	MaxLength      *int   `json:"max_length,omitempty"`
}

type TableInfo struct {
	Name                  string       `json:"name"`
	Columns               []ColumnInfo `json:"columns"`
	PrimaryKeyColumns     []string     `json:"primary_key_columns"`
	TimestampColumns      []string     `json:"timestamp_columns"`
	ValueColumns          []string     `json:"value_columns"`
	DimensionColumns      []string     `json:"dimension_columns"`
	InferredMetricType    string        `json:"inferred_metric_type"`
	InferredOntologyClass string        `json:"inferred_ontology_class"`
	RowCountEstimate      *int          `json:"row_count_estimate,omitempty"`
}