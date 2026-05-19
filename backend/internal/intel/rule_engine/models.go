// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Health Intelligence Engine
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package rule_engine




type RuleCondition struct {
	Metric   string  `yaml:"metric" json:"metric"`
	Operator string  `yaml:"operator" json:"operator"`
	Value    float64 `yaml:"value" json:"value"`
}

type CompiledRule struct {
	Name          string          `yaml:"name" json:"name"`
	Description   string          `yaml:"description,omitempty" json:"description,omitempty"`
	Conditions    []RuleCondition `yaml:"conditions" json:"conditions"`
	Logic         string          `yaml:"logic" json:"logic"`
	Severity      string          `yaml:"severity" json:"severity"`
	Message       string          `yaml:"message" json:"message"`
	Recommendation []string       `yaml:"recommendation" json:"recommendation"`
}