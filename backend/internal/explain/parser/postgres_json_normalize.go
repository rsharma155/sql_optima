package parser

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// pgBufferKeyToSnake maps PostgreSQL per-node buffer field names to types.Buffers JSON keys.
var pgBufferKeyToSnake = map[string]string{
	"Shared Hit Blocks":      "shared_hit",
	"Shared Read Blocks":     "shared_read",
	"Shared Dirtied Blocks":  "shared_dirtied",
	"Shared Written Blocks":  "shared_written",
	"Local Hit Blocks":       "local_hit",
	"Local Read Blocks":      "local_read",
	"Local Dirtied Blocks":   "local_dirtied",
	"Local Written Blocks":   "local_written",
	"Temp Read Blocks":       "temp_read",
	"Temp Written Blocks":    "temp_written",
}

// pgNodeFieldAliases maps normalized snake_case PostgreSQL names to types.PlanNode JSON tags.
var pgNodeFieldAliases = map[string]string{
	"hash_buckets":      "buckets",
	"hash_batches":      "batches",
	"peak_memory_usage": "memory_usage",
}

// pgExplainIgnoredNodeFields: PostgreSQL-only fields we skip so json.Unmarshal into PlanNode stays predictable.
var pgExplainIgnoredNodeFields = map[string]struct{}{
	"original_hash_buckets": {},
	"original_hash_batches": {},
	"parent_relationship":   {},
	"parallel_aware":        {},
	"async_capable":         {},
	"single_copy":           {},
	"inner_unique":          {},
	"strategy":              {},
	"partial_mode":          {},
	"workers":               {},
}

// stringyPlanKeys: unmarshaled as strings (or []string joined) on types.PlanNode but PostgreSQL may send string arrays.
var stringyPlanKeys = map[string]struct{}{
	"hash_cond":    {},
	"merge_cond":   {},
	"index_cond":   {},
	"recheck_cond": {},
	"join_filter":  {},
	"filter":       {},
}

// postgresKeyToSnake maps JSON keys to snake_case matching types.Plan / types.PlanNode json tags.
// Handles: PostgreSQL "Node Type" / "Total Cost", already-snake keys, and camelCase from clients (NodeType, totalCost).
func postgresKeyToSnake(k string) string {
	switch k {
	case "Plan":
		return "plan"
	case "Plans":
		return "plans"
	default:
		if strings.Contains(k, " ") {
			return strings.ToLower(strings.ReplaceAll(k, " ", "_"))
		}
		if strings.Contains(k, "_") {
			return strings.ToLower(k)
		}
		return camelJSONKeyToSnake(k)
	}
}

// camelJSONKeyToSnake turns NodeType -> node_type, TotalCost -> total_cost, PlanRows -> plan_rows.
func camelJSONKeyToSnake(s string) string {
	if s == "" {
		return s
	}
	runes := []rune(s)
	var b strings.Builder
	b.Grow(len(s) + 4)
	for i, r := range runes {
		if i > 0 && r >= 'A' && r <= 'Z' {
			prev := runes[i-1]
			if prev >= 'a' && prev <= 'z' {
				b.WriteByte('_')
			}
		}
		b.WriteRune(r)
	}
	return strings.ToLower(b.String())
}

// NormalizePostgresExplainJSON rewrites PostgreSQL EXPLAIN (FORMAT JSON) into the shape expected by json.Unmarshal into types.Plan.
// It unwraps single-element arrays, common API wrappers (e.g. execution_plan_json), Title Case keys, buffer stats, and a few field aliases.
func NormalizePostgresExplainJSON(raw []byte) ([]byte, error) {
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	root, queryHint, ok := findExplainPlanWrapper(v)
	if !ok {
		return nil, fmt.Errorf("not a valid EXPLAIN (FORMAT JSON) payload: need an object with a \"Plan\" property, or a wrapper like \"execution_plan_json\" containing it (optional: a one-element JSON array)")
	}
	norm := normalizeExplainValue(root)
	out, ok := norm.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("internal: normalized plan is not a JSON object")
	}
	if queryHint != "" {
		if _, exists := out["query"]; !exists {
			out["query"] = queryHint
		}
	}
	return json.Marshal(out)
}

func unwrapJSONArray(v interface{}) interface{} {
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
		// Multiple statements or batched JSON: pick the first object that looks like EXPLAIN output.
		for _, el := range arr {
			if m, ok := el.(map[string]interface{}); ok {
				if r, _, ok2 := findExplainPlanWrapperInArrayBranch(m); ok2 {
					return r
				}
			}
		}
		return arr[0]
	}
}

// findExplainPlanWrapperInArrayBranch avoids calling unwrapJSONArray first (caller is already unwrapping an array).
func findExplainPlanWrapperInArrayBranch(m map[string]interface{}) (map[string]interface{}, string, bool) {
	if hasPlanObject(m) {
		return m, extractQueryFromWrapper(m), true
	}
	outerQuery := extractQueryFromWrapper(m)
	for _, key := range []string{
		"execution_plan_json", "executionPlanJson", "execution_plan", "executionPlan",
		"explain_plan", "explainPlan", "plan_json", "planJson",
		"plan_root", "planRoot",
	} {
		if inner, ok := m[key].(map[string]interface{}); ok {
			r, q, ok := findExplainPlanWrapper(inner)
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
			r, q, ok := findExplainPlanWrapper(inner)
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

func hasPlanObject(m map[string]interface{}) bool {
	for _, k := range []string{"Plan", "plan"} {
		sub, ok := m[k].(map[string]interface{})
		if ok && sub != nil {
			return true
		}
	}
	return false
}

func extractQueryFromWrapper(m map[string]interface{}) string {
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

// findExplainPlanWrapper returns the object that carries Plan + planning/execution times (EXPLAIN root), optional SQL hint from outer wrappers.
func findExplainPlanWrapper(v interface{}) (map[string]interface{}, string, bool) {
	v = unwrapJSONArray(v)
	m, ok := v.(map[string]interface{})
	if !ok {
		return nil, "", false
	}
	if hasPlanObject(m) {
		return m, extractQueryFromWrapper(m), true
	}
	outerQuery := extractQueryFromWrapper(m)
	for _, key := range []string{
		"execution_plan_json", "executionPlanJson", "execution_plan", "executionPlan",
		"explain_plan", "explainPlan", "plan_json", "planJson",
		"plan_root", "planRoot",
	} {
		if inner, ok := m[key].(map[string]interface{}); ok {
			r, q, ok := findExplainPlanWrapper(inner)
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
			r, q, ok := findExplainPlanWrapper(inner)
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

func normalizeExplainValue(v interface{}) interface{} {
	switch x := v.(type) {
	case map[string]interface{}:
		return normalizeExplainObject(x)
	case []interface{}:
		out := make([]interface{}, len(x))
		for i := range x {
			out[i] = normalizeExplainValue(x[i])
		}
		return out
	default:
		return v
	}
}

func normalizeExplainObject(m map[string]interface{}) map[string]interface{} {
	buf := make(map[string]interface{})
	out := make(map[string]interface{})

	for k, raw := range m {
		if sub, ok := pgBufferKeyToSnake[k]; ok {
			if n, ok := toInt(raw); ok {
				buf[sub] = n
			}
			continue
		}

		val := normalizeExplainValue(raw)
		nk := postgresKeyToSnake(k)

		if _, drop := pgExplainIgnoredNodeFields[nk]; drop {
			continue
		}
		if alias, ok := pgNodeFieldAliases[nk]; ok {
			nk = alias
		}

		if nk == "sort_key" || nk == "group_key" {
			if coerced := coerceToStringSliceForJSON(val); coerced != nil {
				out[nk] = coerced
			}
			continue
		}

		if _, isStr := stringyPlanKeys[nk]; isStr {
			if s, ok := stringifyPGCond(val); ok {
				if s != "" {
					out[nk] = s
				}
				continue
			}
			// e.g. boolean or object — omit so json.Unmarshal into string does not fail
			continue
		}

		val = coerceJSONNumbers(val)
		out[nk] = val
	}

	if len(buf) > 0 {
		out["buffers"] = buf
	}
	mergeTypoJSONKeys(out)
	return hoistMisnestedPlanNode(out)
}

// pgJSONKeyTypoToCanonical fixes keys produced by broken serializers (e.g. "nodetype" instead of "node_type").
var pgJSONKeyTypoToCanonical = map[string]string{
	"nodetype":       "node_type",
	"totalcost":      "total_cost",
	"startupcost":    "startup_cost",
	"planrows":       "plan_rows",
	"planwidth":      "plan_width",
	"relationname":   "relation_name",
	"indexname":      "index_name",
	"actualrows":     "actual_rows",
	"actualloops":    "actual_loops",
	"actualtotaltime": "actual_total_time",
	"actualstartuptime": "actual_startup_time",
}

func mergeTypoJSONKeys(out map[string]interface{}) {
	for typo, canon := range pgJSONKeyTypoToCanonical {
		v, ok := out[typo]
		if !ok {
			continue
		}
		if cur, has := out[canon]; has {
			if s, ok := cur.(string); ok && strings.TrimSpace(s) != "" {
				delete(out, typo)
				continue
			}
			if f, ok := cur.(float64); ok && f != 0 {
				delete(out, typo)
				continue
			}
			if i, ok := cur.(int); ok && i != 0 {
				delete(out, typo)
				continue
			}
		}
		out[canon] = v
		delete(out, typo)
	}
}

// isExplainStatementRoot is true for the top-level EXPLAIN JSON object (plan + planning/execution times).
func isExplainStatementRoot(m map[string]interface{}) bool {
	_, hasPT := m["planning_time"]
	_, hasET := m["execution_time"]
	if !hasPT && !hasET {
		return false
	}
	_, hasPlan := m["plan"]
	return hasPlan
}

// hoistMisnestedPlanNode flattens { "plan": { "node_type": "...", ... } } when the outer object is only
// that single key (some exporters double-wrap children in the Plans array). Never hoists the real EXPLAIN root.
func hoistMisnestedPlanNode(out map[string]interface{}) map[string]interface{} {
	if isExplainStatementRoot(out) {
		return out
	}
	if len(out) != 1 {
		return out
	}
	inner, ok := out["plan"].(map[string]interface{})
	if !ok || len(inner) == 0 {
		return out
	}
	innerNT, _ := inner["node_type"].(string)
	if strings.TrimSpace(innerNT) == "" {
		return out
	}
	return inner
}

func stringifyPGCond(v interface{}) (string, bool) {
	switch t := v.(type) {
	case nil:
		return "", true
	case string:
		return t, true
	case []interface{}:
		if len(t) == 0 {
			return "", true
		}
		var parts []string
		for _, e := range t {
			if s, ok := e.(string); ok {
				parts = append(parts, s)
			}
		}
		if len(parts) == 0 {
			return "", true
		}
		return strings.Join(parts, " "), true
	default:
		return "", false
	}
}

func coerceToStringSliceForJSON(v interface{}) interface{} {
	switch t := v.(type) {
	case nil:
		return nil
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return nil
		}
		return []interface{}{s}
	case []interface{}:
		out := make([]interface{}, 0, len(t))
		for _, e := range t {
			switch x := e.(type) {
			case string:
				out = append(out, x)
			case nil:
				continue
			default:
				out = append(out, fmt.Sprint(x))
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	default:
		s := strings.TrimSpace(fmt.Sprint(t))
		if s == "" || s == "<nil>" {
			return nil
		}
		return []interface{}{s}
	}
}

// coerceJSONNumbers converts json.Number (Decoder UseNumber) and nested maps/slices to plain float64/int-friendly values.
func coerceJSONNumbers(v interface{}) interface{} {
	switch t := v.(type) {
	case json.Number:
		if i, err := t.Int64(); err == nil {
			if i >= -1<<31 && i < 1<<31 {
				return int(i)
			}
			return float64(i)
		}
		f, _ := t.Float64()
		return f
	case map[string]interface{}:
		for k, vv := range t {
			t[k] = coerceJSONNumbers(vv)
		}
		return t
	case []interface{}:
		for i := range t {
			t[i] = coerceJSONNumbers(t[i])
		}
		return t
	default:
		return v
	}
}

func toInt(v interface{}) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int32:
		return int(n), true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			f, err2 := n.Float64()
			if err2 != nil {
				return 0, false
			}
			return int(f), true
		}
		return int(i), true
	case string:
		i, err := strconv.Atoi(strings.TrimSpace(n))
		if err != nil {
			return 0, false
		}
		return i, true
	default:
		return 0, false
	}
}

// UnwrapExplainJSON returns one EXPLAIN (FORMAT JSON) root object as JSON bytes, preserving PostgreSQL key names ("Plan", "Node Type", …).
// It unwraps single-element top-level arrays and common API envelopes (execution_plan_json, etc.). Use this for tools that parse native EXPLAIN JSON;
// use NormalizePostgresExplainJSON when unmarshaling into types.Plan.
func UnwrapExplainJSON(raw []byte) ([]byte, error) {
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	root, _, ok := findExplainPlanWrapper(v)
	if !ok {
		return nil, fmt.Errorf("not a valid EXPLAIN (FORMAT JSON) payload: need an object with a \"Plan\" property, or a wrapper such as \"execution_plan_json\"")
	}
	return json.Marshal(root)
}
