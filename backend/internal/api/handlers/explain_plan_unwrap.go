// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Utility for unwrapping and simplifying complex EXPLAIN plan output for better readability.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"encoding/json"
	"fmt"
	"strings"
)

// unwrapExplainJSONPreserveKeys returns one EXPLAIN (FORMAT JSON) root object as JSON bytes,
// keeping PostgreSQL key names ("Plan", "Node Type", …). It unwraps single-element arrays and
// envelopes such as execution_plan_json. Duplicated here so the server builds even when the
// replaced pg_explain_analyze module is older than parser.UnwrapExplainJSON.
// Keep array / wrapper behavior aligned with pg_explain_analyze/parser.unwrapJSONArray + findExplainPlanWrapper.
func unwrapExplainJSONPreserveKeys(raw []byte) ([]byte, error) {
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	root, _, ok := findExplainPlanWrapperForAdvisor(v)
	if !ok {
		return nil, fmt.Errorf("not a valid EXPLAIN (FORMAT JSON) payload: need an object with a \"Plan\" property, or a wrapper such as \"execution_plan_json\"")
	}
	return json.Marshal(root)
}

func unwrapJSONArrayForAdvisor(v interface{}) interface{} {
	for {
		arr, ok := v.([]interface{})
		if !ok {
			return v
		}
		if len(arr) == 0 {
			return v
		}
		if len(arr) == 1 {
			v = arr[0]
			continue
		}
		for _, el := range arr {
			if m, ok := el.(map[string]interface{}); ok {
				if r, _, ok2 := findExplainPlanWrapperInArrayBranchForAdvisor(m); ok2 {
					return r
				}
			}
		}
		return arr[0]
	}
}

func findExplainPlanWrapperInArrayBranchForAdvisor(m map[string]interface{}) (map[string]interface{}, string, bool) {
	if hasPlanObjectForAdvisor(m) {
		return m, extractQueryFromWrapperForAdvisor(m), true
	}
	outerQuery := extractQueryFromWrapperForAdvisor(m)
	for _, key := range []string{
		"execution_plan_json", "executionPlanJson", "execution_plan", "executionPlan",
		"explain_plan", "explainPlan", "plan_json", "planJson",
		"plan_root", "planRoot",
	} {
		if inner, ok := m[key].(map[string]interface{}); ok {
			r, q, ok := findExplainPlanWrapperForAdvisor(inner)
			if ok {
				if outerQuery != "" {
					return r, outerQuery, true
				}
				return r, q, true
			}
		}
	}
	for _, key := range []string{"result", "data", "payload", "body", "explain", "output", "explain_output"} {
		if inner, ok := m[key].(map[string]interface{}); ok {
			r, q, ok := findExplainPlanWrapperForAdvisor(inner)
			if ok {
				if outerQuery != "" {
					return r, outerQuery, true
				}
				return r, q, true
			}
		}
	}
	return nil, "", false
}

func hasPlanObjectForAdvisor(m map[string]interface{}) bool {
	for _, k := range []string{"Plan", "plan"} {
		sub, ok := m[k].(map[string]interface{})
		if ok && sub != nil {
			return true
		}
	}
	return false
}

func extractQueryFromWrapperForAdvisor(m map[string]interface{}) string {
	for _, key := range []string{"query_text", "queryText", "query", "sql", "sql_text", "Sql"} {
		if s, ok := m[key].(string); ok {
			s = strings.TrimSpace(s)
			if s != "" {
				return s
			}
		}
	}
	return ""
}

func findExplainPlanWrapperForAdvisor(v interface{}) (map[string]interface{}, string, bool) {
	v = unwrapJSONArrayForAdvisor(v)
	m, ok := v.(map[string]interface{})
	if !ok {
		return nil, "", false
	}
	if hasPlanObjectForAdvisor(m) {
		return m, extractQueryFromWrapperForAdvisor(m), true
	}
	outerQuery := extractQueryFromWrapperForAdvisor(m)
	for _, key := range []string{
		"execution_plan_json", "executionPlanJson", "execution_plan", "executionPlan",
		"explain_plan", "explainPlan", "plan_json", "planJson",
		"plan_root", "planRoot",
	} {
		if inner, ok := m[key].(map[string]interface{}); ok {
			r, q, ok := findExplainPlanWrapperForAdvisor(inner)
			if ok {
				if outerQuery != "" {
					return r, outerQuery, true
				}
				return r, q, true
			}
		}
	}
	for _, key := range []string{"result", "data", "payload", "body", "explain", "output", "explain_output"} {
		if inner, ok := m[key].(map[string]interface{}); ok {
			r, q, ok := findExplainPlanWrapperForAdvisor(inner)
			if ok {
				if outerQuery != "" {
					return r, outerQuery, true
				}
				return r, q, true
			}
		}
	}
	return nil, "", false
}
