// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Rule engine handlers for best practices evaluation and configuration compliance checking.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"log/slog"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/expr-lang/expr"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/domain/rules"
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
	instanceName := r.URL.Query().Get("instance")
	dbType := r.URL.Query().Get("db_type")

	if serverIDStr == "" && instanceName == "" && dbType == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "server_id, instance or db_type is required"})
		return
	}

	var serverID uuid.UUID
	var instanceType string

	if instanceName != "" {
		for i := range h.cfg.Instances {
			inst := &h.cfg.Instances[i]
			if inst.Name == instanceName {
				serverID = inst.ServerID
				instanceType = inst.Type
				break
			}
		}
	}

	if serverID == uuid.Nil && serverIDStr != "" {
		var err error
		serverID, err = uuid.Parse(serverIDStr)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid server_id (must be UUID)"})
			return
		}
	}

	if instanceType == "" && serverID != uuid.Nil {
		for i := range h.cfg.Instances {
			inst := &h.cfg.Instances[i]
			if inst.ServerID == serverID {
				instanceType = inst.Type
				break
			}
		}
	}

	if instanceType == "" && dbType != "" {
		instanceType = dbType
	}
	if dbType == "" && instanceType != "" {
		dbType = instanceType
	}

	dbType = strings.ToLower(strings.TrimSpace(dbType))
	instanceType = strings.ToLower(strings.TrimSpace(instanceType))

	if dbType == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "unable to determine target database type"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	entries, err := h.getDashboard(ctx, serverID, dbType)
	if err != nil {
		slog.Error("[BestPracticesHandler] Error", "err", err)
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
			slog.Error("[BestPracticesHandler] Failed to list enabled rules", "err", err)
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
	if serverID != uuid.Nil && instanceType != "" && h.pgPool != nil {
		slog.Info("[BestPracticesHandler] Triggering on-demand evaluation for server", "val", serverID)
		// Delete old results for this server to get fresh evaluation
		if _, err := h.pgPool.Exec(ctx, `DELETE FROM ruleengine.rule_results_evaluated WHERE server_id = $1`, serverID); err != nil {
			slog.Info("[BestPracticesHandler] Delete prior results", "err", err)
		}
		if err := h.evaluateRulesForServer(ctx, serverID, instanceType); err != nil {
			slog.Error("[BestPracticesHandler] Evaluation failed", "err", err)
		}
		// Fetch results after evaluation
		entries, _ = h.getDashboard(ctx, serverID, dbType)
	} else if serverID != uuid.Nil && instanceType != "" && h.pgPool == nil {
		slog.Warn("[BestPracticesHandler] Skipping on-demand evaluation for server %s: TimescaleDB unavailable", "val", serverID)
	}

	w.Header().Set("Content-Type", "application/json")
	resp := map[string]interface{}{
		"server_id":      serverID,
		"target_db_type": dbType,
		"count":          len(entries),
		"rules_total":    rulesTotal,
		"missing_rules":  missingRuleIDs,
		"best_practices": entries,
	}
	if h.pgPool == nil {
		resp["warning"] = "TimescaleDB not connected. Rule evaluation requires the metrics repository."
	}
	json.NewEncoder(w).Encode(resp)
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

func (h *RulesHandler) getDashboard(ctx context.Context, serverID uuid.UUID, dbType string) ([]models.DashboardEntry, error) {
	if h.pgPool == nil {
		return []models.DashboardEntry{}, nil
	}

	var rows pgx.Rows
	var err error

	// Normalize status to ensure proper comparison - uppercase
	if dbType != "" && serverID != uuid.Nil {
		// Latest evaluated result per rule for a specific server + db type
		query := `
			SELECT 
				r.rule_id,
				r.rule_name,
				r.category,
				r.severity,
				UPPER(COALESCE(e.status, 'OK')) AS status,
				e.current_value,
				COALESCE(e.recommended, r.recommended_value) AS recommended_value,
				r.description,
				CASE WHEN $1 = 'postgres' THEN COALESCE(NULLIF(BTRIM(r.fix_script_pg), ''), r.fix_script) ELSE COALESCE(NULLIF(BTRIM(r.fix_script), ''), r.fix_script_pg) END AS fix_script,
				e.capture_timestamp
			FROM ruleengine.rules r
			LEFT JOIN LATERAL (
				SELECT rule_id, server_id, UPPER(status) as status, current_value, recommended, capture_timestamp
				FROM ruleengine.rule_results_evaluated
				WHERE target_db_type = $1 AND server_id = $2 AND rule_id = r.rule_id
				ORDER BY capture_timestamp DESC
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
				r.severity,
				UPPER(COALESCE(e.status, 'OK')) AS status,
				e.current_value,
				COALESCE(e.recommended, r.recommended_value) AS recommended_value,
				r.description,
				CASE WHEN $1 = 'postgres' THEN COALESCE(NULLIF(BTRIM(r.fix_script_pg), ''), r.fix_script) ELSE COALESCE(NULLIF(BTRIM(r.fix_script), ''), r.fix_script_pg) END AS fix_script,
				e.capture_timestamp
			FROM ruleengine.rules r
			LEFT JOIN LATERAL (
				SELECT rule_id, server_id, UPPER(status) as status, current_value, recommended, capture_timestamp
				FROM ruleengine.rule_results_evaluated
				WHERE target_db_type = $1 AND rule_id = r.rule_id
				ORDER BY capture_timestamp DESC
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
	} else if serverID != uuid.Nil {
		query := `
			SELECT 
				r.rule_id,
				r.rule_name,
				r.category,
				r.severity,
				UPPER(COALESCE(e.status, 'OK')) AS status,
				e.current_value,
				COALESCE(e.recommended, r.recommended_value) AS recommended_value,
				r.description,
				CASE WHEN COALESCE(s.db_type, 'sqlserver') = 'postgres' THEN COALESCE(NULLIF(BTRIM(r.fix_script_pg), ''), r.fix_script) ELSE COALESCE(NULLIF(BTRIM(r.fix_script), ''), r.fix_script_pg) END AS fix_script,
				e.capture_timestamp
			FROM ruleengine.rules r
			LEFT JOIN LATERAL (
				SELECT rule_id, server_id, UPPER(status) as status, current_value, recommended, capture_timestamp
				FROM ruleengine.rule_results_evaluated
				WHERE rule_id = r.rule_id AND server_id = $1
				ORDER BY capture_timestamp DESC
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
				r.severity,
				UPPER(COALESCE(e.status, 'OK')) AS status,
				e.current_value,
				COALESCE(e.recommended, r.recommended_value) AS recommended_value,
				r.description,
				COALESCE(NULLIF(BTRIM(r.fix_script), ''), r.fix_script_pg) AS fix_script,
				e.capture_timestamp
			FROM ruleengine.rules r
			LEFT JOIN LATERAL (
				SELECT rule_id, UPPER(status) as status, current_value, recommended, capture_timestamp
				FROM ruleengine.rule_results_evaluated e2
				WHERE e2.rule_id = r.rule_id
				ORDER BY capture_timestamp DESC
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
		slog.Error("[RulesHandler] Query failed", "err", err)
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
			&e.Severity,
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
		e.Remediation = renderRuleRemediation(e.FixScript, e.RecommendedValue)
		if lastCheck.Valid {
			e.LastCheck = lastCheck.Time
		}
		entries = append(entries, e)
	}

	if entries == nil {
		entries = []models.DashboardEntry{}
	}

	if serverID != uuid.Nil && dbType != "" && len(entries) > 0 {
		if err := h.enrichDashboardEntries(ctx, serverID, dbType, entries); err != nil {
			slog.Error("[RulesHandler] Failed to enrich best practices payload", "err", err)
		}
	}

	return entries, nil
}

func (h *RulesHandler) enrichDashboardEntries(ctx context.Context, serverID uuid.UUID, dbType string, entries []models.DashboardEntry) error {
	if h.pgPool == nil || len(entries) == 0 {
		return nil
	}

	evidenceByRule := map[string]string{}
	evidenceRows, err := h.pgPool.Query(ctx, `
		SELECT DISTINCT ON (rr.rule_id)
			rr.rule_id,
			rr.raw_payload
		FROM ruleengine.rule_results_raw rr
		WHERE rr.server_id = $1
		  AND EXISTS (
			SELECT 1
			FROM ruleengine.rules r
			WHERE r.rule_id = rr.rule_id
			  AND r.target_db_type = $2
		  )
		ORDER BY rr.rule_id, rr.capture_timestamp DESC
	`, serverID, dbType)
	if err != nil {
		return err
	}
	defer evidenceRows.Close()

	for evidenceRows.Next() {
		var ruleID string
		var rawPayload []byte
		if err := evidenceRows.Scan(&ruleID, &rawPayload); err != nil {
			continue
		}
		evidenceByRule[ruleID] = buildRuleEvidence(rawPayload)
	}
	if err := evidenceRows.Err(); err != nil {
		return err
	}

	historyByRule := map[string][]models.RuleHistoryPoint{}
	historyRows, err := h.pgPool.Query(ctx, `
		SELECT rule_id, status, current_value, capture_timestamp
		FROM (
			SELECT
				rule_id,
				UPPER(status) AS status,
				current_value,
				capture_timestamp,
				ROW_NUMBER() OVER (PARTITION BY rule_id ORDER BY capture_timestamp DESC) AS rn
			FROM ruleengine.rule_results_evaluated
			WHERE server_id = $1 AND target_db_type = $2
		) history
		WHERE rn <= 8
		ORDER BY rule_id, capture_timestamp DESC
	`, serverID, dbType)
	if err != nil {
		return err
	}
	defer historyRows.Close()

	for historyRows.Next() {
		var ruleID string
		var point models.RuleHistoryPoint
		var currentValue sql.NullString
		if err := historyRows.Scan(&ruleID, &point.Status, &currentValue, &point.EvaluatedAt); err != nil {
			continue
		}
		if currentValue.Valid {
			point.CurrentValue = currentValue.String
		}
		historyByRule[ruleID] = append(historyByRule[ruleID], point)
	}
	if err := historyRows.Err(); err != nil {
		return err
	}

	for i := range entries {
		if evidence := strings.TrimSpace(evidenceByRule[entries[i].RuleID]); evidence != "" {
			entries[i].Evidence = evidence
		} else {
			entries[i].Evidence = fallbackRuleEvidence(entries[i])
		}
		entries[i].History = historyByRule[entries[i].RuleID]
		if strings.TrimSpace(entries[i].Remediation) == "" {
			entries[i].Remediation = renderRuleRemediation(entries[i].FixScript, entries[i].RecommendedValue)
		}
		entries[i].WhyThisMatters = rules.GenerateWhyThisMatters(entries[i].RuleID)
		entries[i].ImpactDetail = rules.GenerateImpact(entries[i].RuleID)

		if strings.TrimSpace(entries[i].Impact) == "" {
			if entries[i].ImpactDetail != "" {
				entries[i].Impact = entries[i].ImpactDetail
			} else {
				entries[i].Impact = entries[i].Description
			}
		}

		entries[i].RiskLevel = rules.GetRiskLevel(entries[i].RuleID)
		entries[i].ConfidenceNote = rules.GetConfidenceNote(entries[i].RuleID)
	}

	return nil
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

func (h *RulesHandler) evaluateRulesForServer(ctx context.Context, serverID uuid.UUID, instanceType string) error {
	if h.pgPool == nil || h.cfg == nil {
		return fmt.Errorf("rule engine not initialized")
	}

	var inst *config.Instance
	for i := range h.cfg.Instances {
		if h.cfg.Instances[i].ServerID == serverID {
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
		slog.Info("[RulesHandler] No rules found for type", "val", instanceType)
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
			slog.Error("[RulesHandler] Sandbox rejected SQL", "target", r.RuleID, "err", serr)
			continue
		}
		if ruleEngineVerbose() {
			// Do not log raw SQL; log only length + a stable prefix (useful to detect placeholders / empty SQL).
			prefix := query
			if len(prefix) > 80 {
				prefix = prefix[:80] + "..."
			}
			slog.Info(fmt.Sprintf("[RulesHandler] Executing rule %s (dialect=%s, sql_len=%d, sql_prefix=%q)", r.RuleID, dialect, len(query), prefix))
		}
		qctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		targetRows, err := db.QueryContext(qctx, wrapped)
		if err != nil {
			cancel()
			slog.Error("[RulesHandler] Query failed", "target", r.RuleID, "err", err)
			continue
		}

		results := make([]map[string]interface{}, 0)
		columns := []string{}
		if targetRows != nil {
			cols, _ := targetRows.Columns()
			columns = cols
			if ruleEngineVerbose() {
				slog.Info("[RulesHandler]", "arg1", r.RuleID, "arg2", cols)
			}
			values := make([]interface{}, len(cols))
			valuePtrs := make([]interface{}, len(cols))
			for i := range values {
				valuePtrs[i] = &values[i]
			}
			for targetRows.Next() {
				if err := targetRows.Scan(valuePtrs...); err != nil {
					slog.Error("[RulesHandler] Scan error", "target", r.RuleID, "err", err)
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
				slog.Info("[RulesHandler]", "arg1", r.RuleID, "arg2", len(results))
			}
			targetRows.Close()
		}
		cancel()

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
				slog.Info("[RulesHandler]", "arg1", r.RuleID, "arg2", len(env))
			}
		} else {
			if ruleEngineVerbose() {
				slog.Info("[RulesHandler] No results for rule", "arg1", r.RuleID, "arg2", query)
			}
		}

		// Evaluate expected_calc to get recommended value.
		// Only run when we have actual metric rows; otherwise the expression
		// variables (dead_pct, lag_bytes, etc.) are absent from env and the
		// expr engine panics with "invalid operation: <nil> > int".
		if len(results) > 0 && r.ExpectedCalc.Valid && r.ExpectedCalc.String != "" {
			if ruleEngineVerbose() {
				slog.Info("[RulesHandler] Evaluating expected_calc", "arg1", r.RuleID, "arg2", r.ExpectedCalc.String)
			}
			program, err := expr.Compile(r.ExpectedCalc.String)
			if err != nil {
				slog.Error("[RulesHandler] Failed to compile expected_calc", "target", r.RuleID, "err", err)
			} else {
				result, err := expr.Run(program, safeExprEnv(env))
				if err != nil {
					slog.Error("[RulesHandler] Failed to run expected_calc", "target", r.RuleID, "err", err)
				} else if f, ok := result.(float64); ok {
					recommendedValue = fmt.Sprintf("%.0f", f)
					env["Recommended"] = f
					env["Expected"] = f
					if ruleEngineVerbose() {
						slog.Info("[RulesHandler]", "arg1", r.RuleID, "arg2", recommendedValue)
					}
				}
			}
		}

		// Evaluate evaluation_logic to get status.
		// Same guard: skip when there are no metric rows to evaluate.
		if len(results) > 0 && r.EvaluationLogic.Valid && r.EvaluationLogic.String != "" {
			if ruleEngineVerbose() {
				slog.Info("[RulesHandler] Evaluating evaluation_logic", "arg1", r.RuleID, "arg2", r.EvaluationLogic.String)
			}
			program, err := expr.Compile(r.EvaluationLogic.String)
			if err != nil {
				slog.Error("[RulesHandler] Failed to compile evaluation_logic", "target", r.RuleID, "err", err)
			} else {
				result, err := expr.Run(program, safeExprEnv(env))
				if err != nil {
					slog.Error("[RulesHandler] Failed to run evaluation_logic", "target", r.RuleID, "err", err)
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
						slog.Info(fmt.Sprintf("[RulesHandler] %s status evaluated: %s (type: %T)", r.RuleID, status, result))
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
						slog.Info(fmt.Sprintf("[RulesHandler] %s current value from column '%s': %s", r.RuleID, colName, currentValue))
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
							slog.Info(fmt.Sprintf("[RulesHandler] %s current value from first column '%s': %s", r.RuleID, col, currentValue))
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
			INSERT INTO ruleengine.rule_results_raw
			(run_id, server_id, rule_id, raw_payload, capture_timestamp)
			VALUES ($1, $2, $3, $4::jsonb, NOW())
		`, runID, serverID, r.RuleID, string(buildRawRulePayload(results, currentValue, recommendedValue, status)))
		if err != nil {
			slog.Error("[RulesHandler] Failed to store raw result", "target", r.RuleID, "err", err)
		}

		_, err = h.pgPool.Exec(ctx, `
			INSERT INTO ruleengine.rule_results_evaluated 
			(run_id, server_id, rule_id, target_db_type, status, current_value, recommended, capture_timestamp)
			VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
		`, runID, serverID, r.RuleID, instanceType, status, currentValue, recommendedValue)
		if err != nil {
			slog.Error("[RulesHandler] Failed to store result", "target", r.RuleID, "err", err)
		} else {
			if ruleEngineVerbose() {
				slog.Info(fmt.Sprintf("[RulesHandler] Stored result for %s: current=%s, recommended=%s, status=%s", r.RuleID, currentValue, recommendedValue, status))
			}
		}
	}

	slog.Info("[RulesHandler] Evaluated", "arg1", len(rules), "arg2", serverID)
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

func buildRawRulePayload(results []map[string]interface{}, currentValue, recommendedValue, status string) []byte {
	payload := map[string]interface{}{
		"rows":              results,
		"CurrentValue":      currentValue,
		"recommended_value": recommendedValue,
		"EvaluatedStatus":   status,
	}
	if evidence := summariseEvidenceRows(results); evidence != "" {
		payload["EvidenceSummary"] = evidence
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return []byte(`{}`)
	}
	return b
}

func buildRuleEvidence(rawPayload []byte) string {
	if len(rawPayload) == 0 {
		return ""
	}

	var payloadMap map[string]interface{}
	if err := json.Unmarshal(rawPayload, &payloadMap); err == nil {
		if evidence, ok := payloadMap["EvidenceSummary"].(string); ok && strings.TrimSpace(evidence) != "" {
			return evidence
		}
		if rows, ok := payloadMap["rows"].([]interface{}); ok {
			return summariseEvidenceInterfaces(rows)
		}
	}

	var rows []interface{}
	if err := json.Unmarshal(rawPayload, &rows); err == nil {
		return summariseEvidenceInterfaces(rows)
	}

	return ""
}

func summariseEvidenceInterfaces(rows []interface{}) string {
	items := make([]map[string]interface{}, 0, len(rows))
	for _, row := range rows {
		if m, ok := row.(map[string]interface{}); ok {
			items = append(items, m)
		}
	}
	return summariseEvidenceRows(items)
}

func summariseEvidenceRows(rows []map[string]interface{}) string {
	if len(rows) == 0 {
		return ""
	}

	first := rows[0]
	parts := make([]string, 0, 4)
	for _, key := range []string{
		"failing_jobs_24h", "disabled_count", "memory_grants", "single_use_pct",
		"affected_databases", "unhealthy_replicas", "sample_jobs", "sample_databases",
		"page_verify_mode", "sample_replicas", "health_states",
	} {
		if val, ok := first[key]; ok {
			formatted := formatEvidenceValue(val)
			if formatted == "" {
				continue
			}
			parts = append(parts, fmt.Sprintf("%s: %s", humanizeEvidenceKey(key), formatted))
		}
		if len(parts) == 4 {
			break
		}
	}

	if len(parts) == 0 {
		for key, val := range first {
			switch strings.ToLower(key) {
			case "currentvalue", "evaluatedstatus", "recommended", "recommended_value":
				continue
			}
			formatted := formatEvidenceValue(val)
			if formatted == "" {
				continue
			}
			parts = append(parts, fmt.Sprintf("%s: %s", humanizeEvidenceKey(key), formatted))
			if len(parts) == 4 {
				break
			}
		}
	}

	if len(parts) == 0 {
		return ""
	}
	if len(rows) > 1 {
		parts = append(parts, fmt.Sprintf("rows: %d", len(rows)))
	}
	return strings.Join(parts, " | ")
}

func formatEvidenceValue(val interface{}) string {
	switch v := val.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(v)
	case []byte:
		return strings.TrimSpace(string(v))
	case float64:
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", v), "0"), ".")
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", v))
	}
}

func humanizeEvidenceKey(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	key = strings.ReplaceAll(key, "_", " ")
	key = strings.ReplaceAll(key, "-", " ")
	words := strings.Fields(strings.ToLower(key))
	for i, word := range words {
		if word == "" {
			continue
		}
		words[i] = strings.ToUpper(word[:1]) + word[1:]
	}
	return strings.Join(words, " ")
}

func renderRuleRemediation(fixScript, recommendedValue string) string {
	fix := strings.TrimSpace(fixScript)
	rec := strings.TrimSpace(recommendedValue)
	if fix != "" && rec != "" {
		replacer := strings.NewReplacer("<RecommendedMB>", rec, "<Recommended>", rec, "<Value>", rec)
		return replacer.Replace(fix)
	}
	if fix != "" {
		return fix
	}
	if rec != "" {
		return fmt.Sprintf("Set the configuration to %s.", rec)
	}
	return ""
}

func fallbackRuleEvidence(entry models.DashboardEntry) string {
	if strings.TrimSpace(entry.CurrentValue) == "" && strings.TrimSpace(entry.RecommendedValue) == "" {
		return ""
	}
	if strings.TrimSpace(entry.RecommendedValue) == "" {
		return fmt.Sprintf("Current value observed: %s", entry.CurrentValue)
	}
	return fmt.Sprintf("Current value observed: %s | Recommended baseline: %s", entry.CurrentValue, entry.RecommendedValue)
}
