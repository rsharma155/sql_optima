// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: API encoding for missing index recommendations.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package api

import (
	"encoding/json"

	"github.com/rsharma155/sql_optima/internal/missing_index/types"
)

// EncodeAnalysisResultJSON converts an analysis result to a generic JSON object for external callers.
func EncodeAnalysisResultJSON(result *types.AnalysisResult) (map[string]any, error) {
	resp := toResponse(result)
	b, err := json.Marshal(resp)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m, nil
}
