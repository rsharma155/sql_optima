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
	"github.com/rsharma155/sql_optima/internal/repository"
	"github.com/rsharma155/sql_optima/internal/ruleengine/models"
	"github.com/rsharma155/sql_optima/internal/ruleengine/oscontext"
	"github.com/rsharma155/sql_optima/internal/security/sqlsandbox"
	"github.com/rsharma155/sql_optima/internal/sqlserver"

	_ "github.com/lib/pq"
)

const (
	ruleEvalHTTPTimeout   = 6 * time.Minute
	ruleEvalRunTimeout    = 5 * time.Minute
	ruleQueryTimeout      = 10 * time.Second
	ruleTargetPingTimeout = 5 * time.Second
)

type RulesHandler struct {
	pgPool       *pgxpool.Pool
	cfg          *config.Config
	osIngestFunc func(context.Context) bool
	pgRepo       *repository.PgRepository
}

// ruleEngineVerbose enables detailed SQL/row logging (set RULEENGINE_DEBUG=1). Default off for production.
func ruleEngineVerbose() bool {
	v := strings.TrimSpace(os.Getenv("RULEENGINE_DEBUG"))
	return v == "1" || strings.EqualFold(v, "true")
}

func NewRulesHandler(pgPool *pgxpool.Pool, cfg *config.Config, osIngestFunc func(context.Context) bool, pgRepo *repository.PgRepository) *RulesHandler {
	return &RulesHandler{pgPool: pgPool, cfg: cfg, osIngestFunc: osIngestFunc, pgRepo: pgRepo}
}

// NewRulesHandlerFromConfig returns a handler with no Timescale pool (legacy helper).
// Prefer NewRulesHandler(metricsSvc.GetTimescaleDBPool(), cfg) so the DB name and host come from persisted setup or compose-backed ConnectMetricsTimescale, not a duplicate env DSN.
func NewRulesHandlerFromConfig(cfg *config.Config) (*RulesHandler, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}
	return NewRulesHandler(nil, cfg, nil, nil), nil
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
			if strings.EqualFold(inst.Name, instanceName) {
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

	ctx, cancel := context.WithTimeout(r.Context(), ruleEvalHTTPTimeout)
	defer cancel()

	var entries []models.DashboardEntry
	var err error

	// Always evaluate rules on-the-fly to get fresh current values (requires TimescaleDB).
	// Use a dedicated evaluation context so the HTTP handler timeout does not cancel mid-run.
	// Do not delete prior results up front — partial runs would otherwise show blank Current values.
	if serverID != uuid.Nil && instanceType != "" && h.pgPool != nil {
		slog.Info("[BestPracticesHandler] Triggering on-demand evaluation for server", "val", serverID)
		evalCtx, evalCancel := context.WithTimeout(context.WithoutCancel(ctx), ruleEvalRunTimeout)
		if err := h.evaluateRulesForServer(evalCtx, serverID, instanceType); err != nil {
			slog.Error("[BestPracticesHandler] Evaluation failed", "err", err)
		}
		evalCancel()
		entries, err = h.getDashboard(ctx, serverID, dbType)
	} else {
		entries, err = h.getDashboard(ctx, serverID, dbType)
		if serverID != uuid.Nil && instanceType != "" && h.pgPool == nil {
			slog.Warn("[BestPracticesHandler] Skipping on-demand evaluation for server %s: TimescaleDB unavailable", "val", serverID)
		}
	}
	if err != nil {
		slog.Error("[BestPracticesHandler] Error", "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to fetch best practices"})
		return
	}

	// Compute coverage metadata (which rules are missing from the response).
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
	if strings.EqualFold(dbType, "postgres") && serverID != uuid.Nil && h.pgPool != nil {
		osEval, err := oscontext.Load(ctx, h.pgPool, serverID, oscontext.DefaultMaxAge)
		if err == nil {
			resp["os_collector_configured"] = osEval.Available
			resp["os_metrics_ingest_enabled"] = h.isOSIngestEnabled(ctx)
		}
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
	rawPayloadByRule := map[string][]byte{}
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
		rawPayloadByRule[ruleID] = rawPayload
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

	evalContextByRule := map[string]json.RawMessage{}
	ctxRows, err := h.pgPool.Query(ctx, `
		SELECT DISTINCT ON (rule_id) rule_id, context
		FROM ruleengine.rule_results_evaluated
		WHERE server_id = $1 AND target_db_type = $2 AND context IS NOT NULL
		ORDER BY rule_id, capture_timestamp DESC
	`, serverID, dbType)
	if err == nil {
		defer ctxRows.Close()
		for ctxRows.Next() {
			var ruleID string
			var ctxJSON []byte
			if err := ctxRows.Scan(&ruleID, &ctxJSON); err != nil || len(ctxJSON) == 0 {
				continue
			}
			evalContextByRule[ruleID] = ctxJSON
		}
		_ = ctxRows.Err()
	}

	for i := range entries {
		if strings.TrimSpace(entries[i].CurrentValue) == "" {
			if cv := currentValueFromRawPayload(rawPayloadByRule[entries[i].RuleID]); cv != "" {
				entries[i].CurrentValue = cv
			}
		}
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
		if raw, ok := evalContextByRule[entries[i].RuleID]; ok {
			entries[i].ContextTags = mergeEvalContextTags(entries[i].ContextTags, raw)
		}
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

// EvaluateServer runs all enabled rules for a monitored instance and stores results in TimescaleDB.
func (h *RulesHandler) EvaluateServer(ctx context.Context, serverID uuid.UUID, instanceType string) error {
	return h.evaluateRulesForServer(ctx, serverID, instanceType)
}

func (h *RulesHandler) evaluateRulesForServer(ctx context.Context, serverID uuid.UUID, instanceType string) error {
	// Detach from the HTTP request context so server write timeouts / client disconnects
	// do not cancel in-flight detection SQL against the monitored instance.
	ctx = context.WithoutCancel(ctx)

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
		ORDER BY priority, rule_id
	`, instanceType)
	if err != nil {
		return fmt.Errorf("failed to fetch rules: %w", err)
	}
	defer rows.Close()

	var rules []ruleEvalRow
	for rows.Next() {
		var r ruleEvalRow
		if err := rows.Scan(&r.RuleID, &r.RuleName, &r.Category, &r.DetectionSQL, &r.DetectionSQLPg,
			&r.EvaluationLogic, &r.ExpectedCalc, &r.RecommendedValue, &r.TargetDBType); err != nil {
			slog.Warn("[RulesHandler] Skipping rule row (scan failed)", "err", err)
			continue
		}
		rules = append(rules, r)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("failed to read rules: %w", err)
	}

	if len(rules) == 0 {
		slog.Info("[RulesHandler] No rules found for type", "val", instanceType)
		return nil
	}

	connStr := h.getTargetConnString(inst)

	var db *sql.DB
	closeDB := false
	if strings.EqualFold(instanceType, "postgres") && h.pgRepo != nil {
		if pooled, ok := h.pgRepo.GetConn(inst.Name); ok && pooled != nil {
			db = pooled
		}
	}
	if db == nil {
		if instanceType == "sqlserver" {
			db, err = sqlserver.OpenMetricsPool(connStr)
			if err != nil {
				return fmt.Errorf("failed to open sql server metrics pool: %w", err)
			}
		} else {
			db, err = sql.Open("postgres", connStr)
			if err != nil {
				return fmt.Errorf("failed to open postgres connection: %w", err)
			}
		}
		closeDB = true
		pingCtx, pingCancel := context.WithTimeout(ctx, ruleTargetPingTimeout)
		defer pingCancel()
		if err := db.PingContext(pingCtx); err != nil {
			return fmt.Errorf("target database unreachable: %w", err)
		}
	}
	if closeDB {
		defer db.Close()
	}

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

	var osEval oscontext.RuleEval
	tryOS := strings.EqualFold(instanceType, "postgres") && h.isOSIngestEnabled(ctx)
	if tryOS {
		osEval, _ = oscontext.Load(ctx, h.pgPool, serverID, oscontext.DefaultMaxAge)
	}

	// Process each rule individually
	for _, r := range rules {
		if ctx.Err() != nil {
			slog.Warn("[RulesHandler] evaluation context expired before all rules finished", "server_id", serverID)
			break
		}

		query := r.detectionQuery(instanceType)
		if strings.TrimSpace(query) == "" {
			h.storeRuleEvaluation(ctx, runID, serverID, r, instanceType, nil, "N/A", "no detection SQL configured", r.RecommendedValue.String, false, osEval)
			continue
		}

		dialect := "postgres"
		if instanceType == "sqlserver" {
			dialect = "sqlserver"
		}
		wrapped, serr := sqlsandbox.WrapWithRowLimit(dialect, query, sqlsandbox.DefaultMaxRows)
		if serr != nil {
			slog.Error("[RulesHandler] Sandbox rejected SQL", "target", r.RuleID, "err", serr)
			h.storeRuleEvaluation(ctx, runID, serverID, r, instanceType, nil, "N/A", "detection SQL rejected: "+truncateErrMsg(serr, 220), r.RecommendedValue.String, false, osEval)
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
		qctx, cancel := context.WithTimeout(ctx, ruleQueryTimeout)
		targetRows, err := db.QueryContext(qctx, wrapped)
		if err != nil {
			cancel()
			slog.Error("[RulesHandler] Query failed", "target", r.RuleID, "err", err)
			h.storeRuleEvaluation(ctx, runID, serverID, r, instanceType, nil, "N/A", "query failed: "+truncateErrMsg(err, 220), r.RecommendedValue.String, false, osEval)
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
					row[col] = normalizeScannedValue(values[i])
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

		if strings.EqualFold(instanceType, "postgres") {
			if tryOS {
				osEval.MergeInto(env)
			} else {
				env["os_available"] = float64(0)
			}
		}
		osEnriched := strings.EqualFold(instanceType, "postgres") && tryOS && osEval.Available && oscontext.IsOSAwareRule(r.RuleID)

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
				} else if f, ok := exprResultFloat(result); ok {
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
					if s, ok := coerceDisplayValue(v); ok {
						currentValue = s
						foundValue = true
						if ruleEngineVerbose() {
							slog.Info(fmt.Sprintf("[RulesHandler] %s current value from column '%s': %s", r.RuleID, colName, currentValue))
						}
						break
					}
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
						if s, ok := coerceDisplayValue(v); ok {
							currentValue = s
							if ruleEngineVerbose() {
								slog.Info(fmt.Sprintf("[RulesHandler] %s current value from first column '%s': %s", r.RuleID, col, currentValue))
							}
							break
						}
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

		h.storeRuleEvaluation(ctx, runID, serverID, r, instanceType, results, status, currentValue, recommendedValue, osEnriched, osEval)
	}

	slog.Info("[RulesHandler] Evaluated", "arg1", len(rules), "arg2", serverID)
	return nil
}

type ruleEvalRow struct {
	RuleID           string
	RuleName         string
	Category         string
	DetectionSQL     sql.NullString
	DetectionSQLPg   sql.NullString
	EvaluationLogic  sql.NullString
	ExpectedCalc     sql.NullString
	RecommendedValue sql.NullString
	TargetDBType     string
}

// detectionQuery returns the detection SQL to run on the monitored instance.
// PostgreSQL rules from 03_additional_pg_rules.sql store SQL only in detection_sql_pg
// (detection_sql is NULL); base rules in 02_rule_engine.sql use detection_sql.
func (r ruleEvalRow) detectionQuery(instanceType string) string {
	if strings.EqualFold(instanceType, "postgres") {
		if r.DetectionSQLPg.Valid {
			if q := strings.TrimSpace(r.DetectionSQLPg.String); q != "" {
				return q
			}
		}
		if r.DetectionSQL.Valid {
			return strings.TrimSpace(r.DetectionSQL.String)
		}
		return ""
	}
	if r.DetectionSQL.Valid {
		return strings.TrimSpace(r.DetectionSQL.String)
	}
	return ""
}

func truncateErrMsg(err error, max int) string {
	if err == nil {
		return ""
	}
	s := strings.TrimSpace(err.Error())
	if max > 0 && len(s) > max {
		return s[:max] + "…"
	}
	return s
}

func (h *RulesHandler) storeRuleEvaluation(
	ctx context.Context,
	runID int,
	serverID uuid.UUID,
	r ruleEvalRow,
	instanceType string,
	results []map[string]interface{},
	status, currentValue, recommendedValue string,
	osEnriched bool,
	osEval oscontext.RuleEval,
) {
	if h.pgPool == nil {
		return
	}
	storeCtx := context.WithoutCancel(ctx)
	if strings.TrimSpace(recommendedValue) == "" && r.RecommendedValue.Valid {
		recommendedValue = r.RecommendedValue.String
	}
	if strings.TrimSpace(currentValue) == "" && len(results) > 0 {
		currentValue = extractCurrentFromResultRow(results[0])
	}
	if strings.TrimSpace(currentValue) == "" && len(results) > 0 {
		currentValue = fmt.Sprintf("%.0f", float64(len(results)))
	}

	_, err := h.pgPool.Exec(storeCtx, `
		INSERT INTO ruleengine.rule_results_raw
		(run_id, server_id, rule_id, raw_payload, capture_timestamp)
		VALUES ($1, $2, $3, $4::jsonb, NOW())
	`, runID, serverID, r.RuleID, string(buildRawRulePayload(results, currentValue, recommendedValue, status, osEnriched)))
	if err != nil {
		slog.Error("[RulesHandler] Failed to store raw result", "target", r.RuleID, "err", err)
	}

	var evalContext []byte
	if osEnriched {
		evalContext, _ = json.Marshal(map[string]interface{}{
			"os_enriched":  true,
			"total_ram_gb": osEval.TotalRAMGB,
		})
	}

	_, err = h.pgPool.Exec(storeCtx, `
		INSERT INTO ruleengine.rule_results_evaluated 
		(run_id, server_id, rule_id, target_db_type, status, current_value, recommended, context, capture_timestamp)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb, NOW())
	`, runID, serverID, r.RuleID, instanceType, status, currentValue, recommendedValue, evalContext)
	if err != nil {
		slog.Error("[RulesHandler] Failed to store result", "target", r.RuleID, "err", err)
	} else if ruleEngineVerbose() {
		slog.Info(fmt.Sprintf("[RulesHandler] Stored result for %s: current=%s, recommended=%s, status=%s", r.RuleID, currentValue, recommendedValue, status))
	}
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

func (h *RulesHandler) isOSIngestEnabled(ctx context.Context) bool {
	if h.osIngestFunc != nil {
		return h.osIngestFunc(ctx)
	}
	v := strings.TrimSpace(os.Getenv("OS_METRICS_INGEST_ENABLED"))
	return v == "1" || strings.EqualFold(v, "true")
}

func normalizeScannedValue(val interface{}) interface{} {
	if val == nil {
		return nil
	}
	switch v := val.(type) {
	case []byte:
		if len(v) == 0 {
			return nil
		}
		s := string(v)
		if i, err := strconv.ParseInt(s, 10, 64); err == nil {
			return i
		}
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return f
		}
		return s
	case sql.NullString:
		if !v.Valid {
			return nil
		}
		return v.String
	case sql.NullInt64:
		if !v.Valid {
			return nil
		}
		return v.Int64
	case sql.NullFloat64:
		if !v.Valid {
			return nil
		}
		return v.Float64
	case sql.NullBool:
		if !v.Valid {
			return nil
		}
		return v.Bool
	default:
		return v
	}
}

func extractCurrentFromResultRow(row map[string]interface{}) string {
	if row == nil {
		return ""
	}
	priority := []string{
		"setting", "value", "current_value", "metric", "result",
		"cnt", "lag_bytes", "minutes_since_log_backup", "dead_pct",
		"long_tx_count", "effective_wal_buffers_pages",
	}
	for _, key := range priority {
		if v, ok := row[key]; ok {
			if s, ok := coerceDisplayValue(v); ok {
				return s
			}
		}
	}
	skip := map[string]struct{}{
		"name": {}, "rule_name": {}, "category": {}, "description": {}, "rule_id": {},
		"worst_table": {}, "schemaname": {}, "relname": {},
	}
	for col, v := range row {
		if _, s := skip[strings.ToLower(col)]; s {
			continue
		}
		if s, ok := coerceDisplayValue(v); ok {
			return s
		}
	}
	return ""
}

func currentValueFromRawPayload(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ""
	}
	if cv, ok := payload["CurrentValue"].(string); ok && strings.TrimSpace(cv) != "" {
		return cv
	}
	rows, ok := payload["rows"].([]interface{})
	if !ok || len(rows) == 0 {
		return ""
	}
	rowMap, ok := rows[0].(map[string]interface{})
	if !ok {
		return ""
	}
	return extractCurrentFromResultRow(rowMap)
}

// coerceDisplayValue formats a detection SQL cell for dashboard/export display.
func coerceDisplayValue(v interface{}) (string, bool) {
	if v == nil {
		return "", false
	}
	switch val := v.(type) {
	case float64:
		if val == float64(int64(val)) {
			return fmt.Sprintf("%.0f", val), true
		}
		return fmt.Sprintf("%g", val), true
	case float32:
		return fmt.Sprintf("%g", val), true
	case int64:
		return fmt.Sprintf("%d", val), true
	case int:
		return fmt.Sprintf("%d", val), true
	case int32:
		return fmt.Sprintf("%d", val), true
	case []byte:
		s := strings.TrimSpace(string(val))
		if s == "" {
			return "", false
		}
		return s, true
	case string:
		s := strings.TrimSpace(val)
		if s == "" {
			return "", false
		}
		return s, true
	case bool:
		return fmt.Sprintf("%v", val), true
	default:
		s := strings.TrimSpace(fmt.Sprintf("%v", val))
		if s == "" {
			return "", false
		}
		return s, true
	}
}

func exprResultFloat(result interface{}) (float64, bool) {
	switch v := result.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case int32:
		return float64(v), true
	default:
		return 0, false
	}
}

func mergeEvalContextTags(ruleTags json.RawMessage, evalContext []byte) json.RawMessage {
	merged := map[string]interface{}{}
	if len(ruleTags) > 0 {
		_ = json.Unmarshal(ruleTags, &merged)
	}
	var eval map[string]interface{}
	if err := json.Unmarshal(evalContext, &eval); err == nil {
		for k, v := range eval {
			merged[k] = v
		}
	}
	b, err := json.Marshal(merged)
	if err != nil {
		return evalContext
	}
	return b
}

func buildRawRulePayload(results []map[string]interface{}, currentValue, recommendedValue, status string, osEnriched bool) []byte {
	payload := map[string]interface{}{
		"rows":              results,
		"CurrentValue":      currentValue,
		"recommended_value": recommendedValue,
		"EvaluatedStatus":   status,
	}
	if osEnriched {
		payload["os_enriched"] = true
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
