// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Rule engine handlers for best practices evaluation and configuration compliance checking.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/expr-lang/expr"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/ruleengine/models"
	"github.com/rsharma155/sql_optima/internal/security/sqlsandbox"
	"github.com/rsharma155/sql_optima/internal/sqlserver"
)

type RulesHandler struct {
	pgPool *pgxpool.Pool
	cfg    *config.Config
}

// ruleEngineVerbose enables detailed SQL/row logging (set RULEENGINE_DEBUG=1). Default off for production.
func ruleEngineVerbose() bool {
	v := strings.TrimSpace(os.Getenv("RULEENGINE_DEBUG"))
	return v == "1" || strings.EqualFold(v, "true")
}

func NewRulesHandler(pgPool *pgxpool.Pool, cfg *config.Config) *RulesHandler {
	return &RulesHandler{pgPool: pgPool, cfg: cfg}
}

// NewRulesHandlerFromConfig returns a handler with no Timescale pool (legacy helper).
// Prefer NewRulesHandler(metricsSvc.GetTimescaleDBPool(), cfg) so the DB name and host come from persisted setup or compose-backed ConnectMetricsTimescale, not a duplicate env DSN.
func NewRulesHandlerFromConfig(cfg *config.Config) (*RulesHandler, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}
	return NewRulesHandler(nil, cfg), nil
}

func (h *RulesHandler) BestPractices(w http.ResponseWriter, r *http.Request) {
	serverIDStr := r.URL.Query().Get("server_id")
	dbType := r.URL.Query().Get("db_type")

	if serverIDStr == "" && dbType == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "server_id or db_type is required"})
		return
	}

	var serverID int
	var instanceType string
	if serverIDStr != "" {
		if _, err := fmt.Sscanf(serverIDStr, "%d", &serverID); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid server_id"})
			return
		}
		// Determine instance type from config
		for i := range h.cfg.Instances {
			inst := &h.cfg.Instances[i]
			// Prefer explicit ID match; fall back to positional match (historical behavior in this codebase).
			if inst.ID == serverID || i+1 == serverID {
				instanceType = inst.Type
				break
			}
		}
		// Use instanceType as dbType to filter rules by target_db_type
		if instanceType != "" {
			dbType = instanceType
		}
	} else if dbType != "" {
		instanceType = dbType
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	entries, err := h.getDashboard(ctx, serverID, dbType)
	if err != nil {
		log.Printf("[BestPracticesHandler] Error: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to fetch best practices"})
		return
	}

	// Compute coverage metadata (which rules are missing from the response).
	// This helps diagnose "rule exists in ruleengine.rules but isn't showing up".
	rulesTotal := 0
	missingRuleIDs := []string{}
	if h.pgPool != nil && dbType != "" {
		enabledRuleIDs, err := h.getEnabledRuleIDs(ctx, dbType)
		if err != nil {
			log.Printf("[BestPracticesHandler] Failed to list enabled rules: %v", err)
		} else {
			rulesTotal = len(enabledRuleIDs)
			present := make(map[string]struct{}, len(entries))
			for _, e := range entries {
				if e.RuleID != "" {
					present[e.RuleID] = struct{}{}
				}
			}
			for _, rid := range enabledRuleIDs {
				if _, ok := present[rid]; !ok {
					missingRuleIDs = append(missingRuleIDs, rid)
				}
			}
		}
	}

	// Always evaluate rules on-the-fly to get fresh current values (requires TimescaleDB)
	if serverID > 0 && instanceType != "" && h.pgPool != nil {
		log.Printf("[BestPracticesHandler] Triggering on-demand evaluation for server %d", serverID)
		// Delete old results for this server to get fresh evaluation
		if _, err := h.pgPool.Exec(ctx, `DELETE FROM ruleengine.rule_results_evaluated WHERE server_id = $1`, serverID); err != nil {
			log.Printf("[BestPracticesHandler] Delete prior results: %v", err)
		}
		if err := h.evaluateRulesForServer(ctx, serverID, instanceType); err != nil {
			log.Printf("[BestPracticesHandler] Evaluation failed: %v", err)
		}
		// Fetch results after evaluation
		entries, _ = h.getDashboard(ctx, serverID, dbType)
	} else if serverID > 0 && instanceType != "" && h.pgPool == nil {
		log.Printf("[BestPracticesHandler] Skipping on-demand evaluation for server %d: TimescaleDB unavailable", serverID)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"server_id":      serverID,
		"target_db_type": dbType,
		"count":          len(entries),
		"rules_total":    rulesTotal,
		"missing_rules":  missingRuleIDs,
		"best_practices": entries,
	})
}

func (h *RulesHandler) getEnabledRuleIDs(ctx context.Context, dbType string) ([]string, error) {
	if h.pgPool == nil {
		return nil, fmt.Errorf("pg pool not initialized")
	}
	rows, err := h.pgPool.Query(ctx, `
		SELECT rule_id
		FROM ruleengine.rules
		WHERE is_enabled = true AND target_db_type = $1
		ORDER BY rule_id
	`, dbType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			continue
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (h *RulesHandler) getDashboard(ctx context.Context, serverID int, dbType string) ([]models.DashboardEntry, error) {
	if h.pgPool == nil {
		return []models.DashboardEntry{}, nil
	}

	var rows pgx.Rows
	var err error

	// Normalize status to ensure proper comparison - uppercase
	if dbType != "" && serverID > 0 {
		// Latest evaluated result per rule for a specific server + db type
		query := `
			SELECT 
				r.rule_id,
				r.rule_name,
				r.category,
				UPPER(COALESCE(e.status, 'OK')) AS status,
				e.current_value,
				COALESCE(e.recommended, r.recommended_value) AS recommended_value,
				r.description,
				CASE WHEN $1 = 'postgres' THEN COALESCE(NULLIF(BTRIM(r.fix_script_pg), ''), r.fix_script) ELSE COALESCE(NULLIF(BTRIM(r.fix_script), ''), r.fix_script_pg) END AS fix_script,
				e.evaluated_at
			FROM ruleengine.rules r
			LEFT JOIN LATERAL (
				SELECT rule_id, server_id, UPPER(status) as status, current_value, recommended, evaluated_at
				FROM ruleengine.rule_results_evaluated
				WHERE target_db_type = $1 AND server_id = $2 AND rule_id = r.rule_id
				ORDER BY evaluated_at DESC
				LIMIT 1
			) e ON r.rule_id = e.rule_id
			WHERE r.is_enabled = true AND r.target_db_type = $1
			ORDER BY 
				CASE UPPER(e.status) 
					WHEN 'CRITICAL' THEN 1 
					WHEN 'WARNING' THEN 2 
					ELSE 3 
				END,
				r.category, 
				r.rule_name;
		`
		rows, err = h.pgPool.Query(ctx, query, dbType, serverID)
	} else if dbType != "" {
		// Latest evaluated result per rule for a db type across any server.
		// (Useful when server_id isn't provided; still show something sane.)
		query := `
			SELECT 
				r.rule_id,
				r.rule_name,
				r.category,
				UPPER(COALESCE(e.status, 'OK')) AS status,
				e.current_value,
				COALESCE(e.recommended, r.recommended_value) AS recommended_value,
				r.description,
				CASE WHEN $1 = 'postgres' THEN COALESCE(NULLIF(BTRIM(r.fix_script_pg), ''), r.fix_script) ELSE COALESCE(NULLIF(BTRIM(r.fix_script), ''), r.fix_script_pg) END AS fix_script,
				e.evaluated_at
			FROM ruleengine.rules r
			LEFT JOIN LATERAL (
				SELECT rule_id, server_id, UPPER(status) as status, current_value, recommended, evaluated_at
				FROM ruleengine.rule_results_evaluated
				WHERE target_db_type = $1 AND rule_id = r.rule_id
				ORDER BY evaluated_at DESC
				LIMIT 1
			) e ON r.rule_id = e.rule_id
			WHERE r.is_enabled = true AND r.target_db_type = $1
			ORDER BY 
				CASE UPPER(e.status) 
					WHEN 'CRITICAL' THEN 1 
					WHEN 'WARNING' THEN 2 
					ELSE 3 
				END,
				r.category, 
				r.rule_name;
		`
		rows, err = h.pgPool.Query(ctx, query, dbType)
	} else if serverID > 0 {
		query := `
			SELECT 
				r.rule_id,
				r.rule_name,
				r.category,
				UPPER(COALESCE(e.status, 'OK')) AS status,
				e.current_value,
				COALESCE(e.recommended, r.recommended_value) AS recommended_value,
				r.description,
				CASE WHEN COALESCE(s.db_type, 'sqlserver') = 'postgres' THEN COALESCE(NULLIF(BTRIM(r.fix_script_pg), ''), r.fix_script) ELSE COALESCE(NULLIF(BTRIM(r.fix_script), ''), r.fix_script_pg) END AS fix_script,
				e.evaluated_at
			FROM ruleengine.rules r
			LEFT JOIN LATERAL (
				SELECT rule_id, server_id, UPPER(status) as status, current_value, recommended, evaluated_at
				FROM ruleengine.rule_results_evaluated
				WHERE rule_id = r.rule_id AND server_id = $1
				ORDER BY evaluated_at DESC
				LIMIT 1
			) e ON r.rule_id = e.rule_id
			LEFT JOIN ruleengine.servers s ON s.server_id = $1
			WHERE r.is_enabled = true AND r.target_db_type = $2
			ORDER BY 
				CASE UPPER(e.status) 
					WHEN 'CRITICAL' THEN 1 
					WHEN 'WARNING' THEN 2 
					ELSE 3 
				END,
				r.category, 
				r.rule_name;
		`
		rows, err = h.pgPool.Query(ctx, query, serverID, dbType)
	} else {
		query := `
			SELECT 
				r.rule_id,
				r.rule_name,
				r.category,
				UPPER(COALESCE(e.status, 'OK')) AS status,
				e.current_value,
				COALESCE(e.recommended, r.recommended_value) AS recommended_value,
				r.description,
				COALESCE(NULLIF(BTRIM(r.fix_script), ''), r.fix_script_pg) AS fix_script,
				e.evaluated_at
			FROM ruleengine.rules r
			LEFT JOIN LATERAL (
				SELECT rule_id, UPPER(status) as status, current_value, recommended, evaluated_at
				FROM ruleengine.rule_results_evaluated e2
				WHERE e2.rule_id = r.rule_id
				ORDER BY evaluated_at DESC
				LIMIT 1
			) e ON true
			WHERE r.is_enabled = true
			ORDER BY 
				CASE UPPER(e.status) 
					WHEN 'CRITICAL' THEN 1 
					WHEN 'WARNING' THEN 2 
					ELSE 3 
				END,
				r.category, 
				r.rule_name;
		`
		rows, err = h.pgPool.Query(ctx, query)
	}

	if err != nil {
		log.Printf("[RulesHandler] Query failed: %v", err)
		return []models.DashboardEntry{}, nil
	}
	defer rows.Close()

	var entries []models.DashboardEntry
	for rows.Next() {
		var e models.DashboardEntry
		var lastCheck sql.NullTime
		var currentValue, recommendedValue, fixScript sql.NullString
		if err := rows.Scan(
			&e.RuleID,
			&e.RuleName,
			&e.Category,
			&e.Status,
			&currentValue,
			&recommendedValue,
			&e.Description,
			&fixScript,
			&lastCheck,
		); err != nil {
			continue
		}
		if currentValue.Valid {
			e.CurrentValue = currentValue.String
		}
		if recommendedValue.Valid {
			e.RecommendedValue = recommendedValue.String
		}
		if fixScript.Valid {
			e.FixScript = fixScript.String
		}
		if lastCheck.Valid {
			e.LastCheck = lastCheck.Time
		}
		entries = append(entries, e)
	}

	if entries == nil {
		entries = []models.DashboardEntry{}
	}

	return entries, nil
}

func (h *RulesHandler) Close() {
	if h.pgPool != nil {
		h.pgPool.Close()
	}
}

func (h *RulesHandler) GetPgPool() interface{} {
	return h.pgPool
}

func (h *RulesHandler) getTargetConnString(inst *config.Instance) string {
	if inst.Type == "sqlserver" {
		trustCert := "false"
		if inst.TrustServerCertificate {
			trustCert = "true"
		}
		cat := strings.TrimSpace(inst.Database)
		if cat == "" {
			cat = "master"
		}
		port := inst.Port
		if port == 0 {
			port = 1433
		}
		// Never log connection details here (host/user are sensitive metadata).
		return fmt.Sprintf("server=%s;port=%d;user id=%s;password=%s;database=%s;encrypt=true;trustServerCertificate=%s;",
			inst.Host, port, inst.User, inst.Password, cat, trustCert)
	}
	sslmode := "disable"
	if inst.SSLMode != "" {
		sslmode = inst.SSLMode
	}
	dbname := strings.TrimSpace(inst.Database)
	if dbname == "" {
		dbname = "postgres"
	}
	port := inst.Port
	if port == 0 {
		port = 5432
	}
	// Never log connection details here (host/user are sensitive metadata).
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		inst.Host, port, inst.User, inst.Password, dbname, sslmode)
}

func (h *RulesHandler) evaluateRulesForServer(ctx context.Context, serverID int, instanceType string) error {
	if h.pgPool == nil || h.cfg == nil {
		return fmt.Errorf("rule engine not initialized")
	}

	var inst *config.Instance
	for i := range h.cfg.Instances {
		if h.cfg.Instances[i].ID == serverID || i+1 == serverID {
			inst = &h.cfg.Instances[i]
			break
		}
	}
	if inst == nil {
		return fmt.Errorf("server not found in config")
	}

	rows, err := h.pgPool.Query(ctx, `
		SELECT rule_id, rule_name, category, detection_sql, detection_sql_pg, 
		       evaluation_logic, expected_calc, recommended_value, target_db_type
		FROM ruleengine.rules 
		WHERE is_enabled = true AND target_db_type = $1
	`, instanceType)
	if err != nil {
		return fmt.Errorf("failed to fetch rules: %w", err)
	}
	defer rows.Close()

	type ruleRow struct {
		RuleID           string
		RuleName         string
		Category         string
		DetectionSQL     string
		DetectionSQLPg   sql.NullString
		EvaluationLogic  sql.NullString
		ExpectedCalc     sql.NullString
		RecommendedValue sql.NullString
		TargetDBType     string
	}

	var rules []ruleRow
	for rows.Next() {
		var r ruleRow
		if err := rows.Scan(&r.RuleID, &r.RuleName, &r.Category, &r.DetectionSQL, &r.DetectionSQLPg,
			&r.EvaluationLogic, &r.ExpectedCalc, &r.RecommendedValue, &r.TargetDBType); err != nil {
			continue
		}
		rules = append(rules, r)
	}

	if len(rules) == 0 {
		log.Printf("[RulesHandler] No rules found for type %s", instanceType)
		return nil
	}

	connStr := h.getTargetConnString(inst)

	var db *sql.DB
	if instanceType == "sqlserver" {
		db, err = sqlserver.OpenMetricsPool(connStr)
		if err != nil {
			return fmt.Errorf("failed to open sql server metrics pool: %w", err)
		}
	} else {
		db, err = sql.Open("postgres", connStr)
	}
	if err != nil {
		return fmt.Errorf("failed to connect to target: %w", err)
	}
	defer db.Close()

	// Ensure a run exists
	var runID int
	if _, err := h.pgPool.Exec(ctx, `
		INSERT INTO ruleengine.servers (server_id, server_name, db_type)
		VALUES ($1, $2, $3)
		ON CONFLICT (server_id) DO UPDATE
		SET
			server_name = EXCLUDED.server_name,
			db_type = EXCLUDED.db_type
	`, serverID, inst.Name, instanceType); err != nil {
		return fmt.Errorf("failed to ensure ruleengine server row: %w", err)
	}

	err = h.pgPool.QueryRow(ctx, `
		INSERT INTO ruleengine.rule_runs(server_id, db_type)
		VALUES ($1, $2)
		RETURNING run_id
	`, serverID, instanceType).Scan(&runID)
	if err != nil {
		return fmt.Errorf("failed to create rule run: %w", err)
	}

	// Process each rule individually
	for _, r := range rules {
		query := r.DetectionSQL
		if instanceType == "postgres" && r.DetectionSQLPg.Valid {
			query = r.DetectionSQLPg.String
		}

		dialect := "postgres"
		if instanceType == "sqlserver" {
			dialect = "sqlserver"
		}
		wrapped, serr := sqlsandbox.WrapWithRowLimit(dialect, query, sqlsandbox.DefaultMaxRows)
		if serr != nil {
			log.Printf("[RulesHandler] Sandbox rejected SQL for %s: %v", r.RuleID, serr)
			continue
		}
		if ruleEngineVerbose() {
			// Do not log raw SQL; log only length + a stable prefix (useful to detect placeholders / empty SQL).
			prefix := query
			if len(prefix) > 80 {
				prefix = prefix[:80] + "..."
			}
			log.Printf("[RulesHandler] Executing rule %s (dialect=%s, sql_len=%d, sql_prefix=%q)", r.RuleID, dialect, len(query), prefix)
		}
		qctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		targetRows, err := db.QueryContext(qctx, wrapped)
		cancel()
		if err != nil {
			log.Printf("[RulesHandler] Query failed for %s: %v", r.RuleID, err)
			continue
		}

		results := make([]map[string]interface{}, 0)
		columns := []string{}
		if targetRows != nil {
			cols, _ := targetRows.Columns()
			columns = cols
			if ruleEngineVerbose() {
				log.Printf("[RulesHandler] %s returned columns: %v", r.RuleID, cols)
			}
			values := make([]interface{}, len(cols))
			valuePtrs := make([]interface{}, len(cols))
			for i := range values {
				valuePtrs[i] = &values[i]
			}
			for targetRows.Next() {
				if err := targetRows.Scan(valuePtrs...); err != nil {
					log.Printf("[RulesHandler] Scan error for %s: %v", r.RuleID, err)
					continue
				}
				row := make(map[string]interface{})
				for i, col := range cols {
					row[col] = values[i]
				}
				results = append(results, row)
			}
			// Never log full result payloads (may contain sensitive values). Row counts are sufficient.
			if ruleEngineVerbose() {
				log.Printf("[RulesHandler] %s returned %d rows", r.RuleID, len(results))
			}
			targetRows.Close()
		}

		currentValue := ""
		recommendedValue := r.RecommendedValue.String
		status := "OK"

		// Build evaluation env (even if query returns 0 rows).
		// Some rules rely on COUNT/row-count semantics (e.g. COUNT >= 4), so we always provide these.
		env := make(map[string]interface{})
		env["Recommended"] = 0.0
		env["COUNT"] = float64(len(results))
		env["count"] = float64(len(results))

		if len(results) > 0 && len(columns) > 0 {
			for _, col := range columns {
				val := results[0][col]
				if val == nil {
					// NULL columns default to 0 so expressions like "cpu_count > 8" don't crash with <nil>.
					env[col] = float64(0)
					continue
				}
				switch v := val.(type) {
				case float64:
					env[col] = v
				case int64:
					env[col] = float64(v)
				case int:
					env[col] = float64(v)
				case int32:
					env[col] = float64(v)
				case []byte:
					s := string(v)
					if f, err := strconv.ParseFloat(s, 64); err == nil {
						env[col] = f
					} else {
						env[col] = s
					}
				case string:
					env[col] = v
				default:
					env[col] = fmt.Sprintf("%v", v)
				}
			}
			// Avoid logging full env map (it can contain sensitive values). Keep only key list size.
			if ruleEngineVerbose() {
				log.Printf("[RulesHandler] %s evaluation env keys=%d", r.RuleID, len(env))
			}
		} else {
			if ruleEngineVerbose() {
				log.Printf("[RulesHandler] No results for rule %s, query: %s", r.RuleID, query)
			}
		}

		// Evaluate expected_calc to get recommended value.
		// Only run when we have actual metric rows; otherwise the expression
		// variables (dead_pct, lag_bytes, etc.) are absent from env and the
		// expr engine panics with "invalid operation: <nil> > int".
		if len(results) > 0 && r.ExpectedCalc.Valid && r.ExpectedCalc.String != "" {
			if ruleEngineVerbose() {
				log.Printf("[RulesHandler] Evaluating expected_calc for %s: %s", r.RuleID, r.ExpectedCalc.String)
			}
			program, err := expr.Compile(r.ExpectedCalc.String)
			if err != nil {
				log.Printf("[RulesHandler] Failed to compile expected_calc for %s: %v", r.RuleID, err)
			} else {
				result, err := expr.Run(program, safeExprEnv(env))
				if err != nil {
					log.Printf("[RulesHandler] Failed to run expected_calc for %s: %v", r.RuleID, err)
				} else if f, ok := result.(float64); ok {
					recommendedValue = fmt.Sprintf("%.0f", f)
					env["Recommended"] = f
					if ruleEngineVerbose() {
						log.Printf("[RulesHandler] %s recommended value calculated: %s", r.RuleID, recommendedValue)
					}
				}
			}
		}

		// Evaluate evaluation_logic to get status.
		// Same guard: skip when there are no metric rows to evaluate.
		if len(results) > 0 && r.EvaluationLogic.Valid && r.EvaluationLogic.String != "" {
			if ruleEngineVerbose() {
				log.Printf("[RulesHandler] Evaluating evaluation_logic for %s: %s", r.RuleID, r.EvaluationLogic.String)
			}
			program, err := expr.Compile(r.EvaluationLogic.String)
			if err != nil {
				log.Printf("[RulesHandler] Failed to compile evaluation_logic for %s: %v", r.RuleID, err)
			} else {
				result, err := expr.Run(program, safeExprEnv(env))
				if err != nil {
					log.Printf("[RulesHandler] Failed to run evaluation_logic for %s: %v", r.RuleID, err)
				} else {
					switch val := result.(type) {
					case string:
						status = val
					case bool:
						if val {
							status = "OK"
						} else {
							status = "Warning"
						}
					default:
						status = "OK"
					}
					if ruleEngineVerbose() {
						log.Printf("[RulesHandler] %s status evaluated: %s (type: %T)", r.RuleID, status, result)
					}
				}
			}
		}

		// Extract current value - prioritize columns with specific names (avoid using "name" / labels as the metric).
		if len(results) > 0 && len(columns) > 0 {
			valueColumns := []string{
				"value_in_use", "MaxServerMemoryMB", "MAXDOP", "file_count",
				"setting", "value", "current_value", "metric", "result",
				"cnt", "lag_bytes", "minutes_since_log_backup", "dead_pct",
				"is_auto_shrink_on", "is_auto_close_on", "is_query_store_on",
				"instant_file_initialization_enabled", "TotalRAM_GB",
				"autovacuum", "wal_level", "fsync", "full_page_writes", "synchronous_commit",
			}
			foundValue := false

			// First try to find known value columns
			for _, colName := range valueColumns {
				if v, ok := results[0][colName]; ok && v != nil {
					switch val := v.(type) {
					case float64:
						currentValue = fmt.Sprintf("%.0f", val)
					case int64:
						currentValue = fmt.Sprintf("%d", val)
					case int:
						currentValue = fmt.Sprintf("%d", val)
					default:
						currentValue = fmt.Sprintf("%v", val)
					}
					foundValue = true
					if ruleEngineVerbose() {
						log.Printf("[RulesHandler] %s current value from column '%s': %s", r.RuleID, colName, currentValue)
					}
					break
				}
			}

			// If not found, use first non-nil column that is not a rule label / metadata key.
			if !foundValue {
				skipAsMetric := map[string]struct{}{
					"name": {}, "rule_name": {}, "category": {}, "description": {}, "rule_id": {},
				}
				for _, col := range columns {
					lc := strings.ToLower(col)
					if _, skip := skipAsMetric[lc]; skip {
						continue
					}
					if v := results[0][col]; v != nil {
						switch val := v.(type) {
						case float64:
							currentValue = fmt.Sprintf("%.0f", val)
						case int64:
							currentValue = fmt.Sprintf("%d", val)
						case int:
							currentValue = fmt.Sprintf("%d", val)
						default:
							currentValue = fmt.Sprintf("%v", val)
						}
						if ruleEngineVerbose() {
							log.Printf("[RulesHandler] %s current value from first column '%s': %s", r.RuleID, col, currentValue)
						}
						break
					}
				}
			}
		}

		// If still empty, fall back to COUNT (at least show something stable instead of blank).
		if strings.TrimSpace(currentValue) == "" {
			currentValue = fmt.Sprintf("%.0f", env["COUNT"].(float64))
		}

		// If expected_calc produced the same string as current (common for OK thresholds), prefer the rule's
		// human recommended_value when it adds context (e.g. "256MB" vs "262144").
		if recommendedValue != "" && currentValue != "" && recommendedValue == currentValue && r.RecommendedValue.Valid {
			rv := strings.TrimSpace(r.RecommendedValue.String)
			if rv != "" && rv != recommendedValue {
				for _, ch := range rv {
					if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') {
						recommendedValue = rv
						break
					}
				}
			}
		}

		_, err = h.pgPool.Exec(ctx, `
			INSERT INTO ruleengine.rule_results_evaluated 
			(run_id, server_id, rule_id, target_db_type, status, current_value, recommended, evaluated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
		`, runID, serverID, r.RuleID, instanceType, status, currentValue, recommendedValue)
		if err != nil {
			log.Printf("[RulesHandler] Failed to store result for %s: %v", r.RuleID, err)
		} else {
			if ruleEngineVerbose() {
				log.Printf("[RulesHandler] Stored result for %s: current=%s, recommended=%s, status=%s",
					r.RuleID, currentValue, recommendedValue, status)
			}
		}
	}

	log.Printf("[RulesHandler] Evaluated %d rules for server %d", len(rules), serverID)
	return nil
}

// safeExprEnv returns a shallow copy of env with all nil values replaced by
// float64(0). This prevents the expr engine from crashing with
// "invalid operation: <nil> > int" when a detection SQL returns NULL columns
// or when the column name doesn't match the expression variable.
func safeExprEnv(env map[string]interface{}) map[string]interface{} {
	safe := make(map[string]interface{}, len(env))
	for k, v := range env {
		if v == nil {
			safe[k] = float64(0)
		} else {
			safe[k] = v
		}
	}
	return safe
}
