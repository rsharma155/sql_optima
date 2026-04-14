// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Rewrite engine type definitions.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package rewrite

type QueryVariant struct {
	Query        string
	AppliedRules []string
	IsOriginal   bool
}

type Result struct {
	OriginalQuery string
	QueryVariants []QueryVariant
}

func NewResult(query string) *Result {
	return &Result{
		OriginalQuery: query,
		QueryVariants: []QueryVariant{
			{Query: query, AppliedRules: []string{}, IsOriginal: true},
		},
	}
}
