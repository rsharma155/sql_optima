package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/expr-lang/expr"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/microsoft/go-mssqldb"
	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/ruleengine/models"
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

func NewRulesHandlerFromConfig(cfg *config.Config) (*RulesHandler, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}

	tsHost := os.Getenv("TIMESCALEDB_HOST")
	tsPort := os.Getenv("TIMESCALEDB_PORT")
	tsUser := os.Getenv("DB_USER")
	tsPassword := os.Getenv("DB_PASSWORD")
	tsDatabase := os.Getenv("DB_NAME")
	tsSSLMode := os.Getenv("TIMESCALEDB_SSLMODE")

	if ruleEngineVerbose() {
		log.Printf("[RulesHandler] TIMESCALEDB_HOST: %s (DB_USER set: %v)", tsHost, tsUser != "")
	}

	if tsHost == "" {
		if v := strings.TrimSpace(os.Getenv("DB_HOST")); v != "" {
			tsHost = v
		} else {
			tsHost = "localhost"
		}
	}
	if tsPort == "" {
		if v := strings.TrimSpace(os.Getenv("DB_PORT")); v != "" {
			tsPort = v
		} else {
			tsPort = "5432"
		}
	}
	if tsDatabase == "" {
		tsDatabase = "dbmonitor_metrics"
	}
	if tsSSLMode == "" {
		tsSSLMode = "disable"
	}

	connStr := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s",
		tsUser, tsPassword, tsHost, tsPort, tsDatabase, tsSSLMode)

	pool, err := pgxpool.New(context.Background(), connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to create pool: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := pool.Ping(ctx); err != nil {
		log.Printf("[RulesHandler] Warning: TimescaleDB not available, best practices will return empty results: %v", err)
		return &RulesHandler{pgPool: nil, cfg: cfg}, nil
	}

	log.Printf("[RulesHandler] Connected to TimescaleDB for best practices dashboard")
	return &RulesHandler{pgPool: pool, cfg: cfg}, nil
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
		query := fmt.Sprintf(`
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
		`)
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
		trustCert := "true"
		if inst.TrustServerCertificate {
			trustCert = "true"
		}
		if ruleEngineVerbose() {
			log.Printf("[RulesHandler] Building SQL Server connection for instance: %s (host: %s, user: %s)", inst.Name, inst.Host, inst.User)
		}
		return fmt.Sprintf("server=%s;port=%d;user id=%s;password=%s;trustServerCertificate=%s;database=master",
			inst.Host, inst.Port, inst.User, inst.Password, trustCert)
	} else {
		sslmode := "disable"
		if inst.SSLMode != "" {
			sslmode = inst.SSLMode
		}
		if ruleEngineVerbose() {
			log.Printf("[RulesHandler] Building PostgreSQL connection for instance: %s (host: %s, user: %s)", inst.Name, inst.Host, inst.User)
		}
		return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
			inst.Host, inst.Port, inst.User, inst.Password, "postgres", sslmode)
	}
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
		db, err = sql.Open("mssql", connStr)
	} else {
		db, err = sql.Open("postgres", connStr)
	}
	if err != nil {
		return fmt.Errorf("failed to connect to target: %w", err)
	}
	defer db.Close()

	// Ensure a run exists
	var runID int
	h.pgPool.QueryRow(ctx, `
		INSERT INTO ruleengine.rule_runs(server_id, db_type) VALUES ($1, $2)
		ON CONFLICT DO NOTHING
		RETURNING run_id
	`, serverID, instanceType).Scan(&runID)
	if runID == 0 {
		h.pgPool.QueryRow(ctx, "SELECT MAX(run_id) FROM ruleengine.rule_runs WHERE server_id = $1", serverID).Scan(&runID)
	}

	// Process each rule individually
	for _, r := range rules {
		query := r.DetectionSQL
		if instanceType == "postgres" && r.DetectionSQLPg.Valid {
			query = r.DetectionSQLPg.String
		}

		if ruleEngineVerbose() {
			log.Printf("[RulesHandler] Executing query for %s: %s", r.RuleID, query)
		}

		targetRows, err := db.QueryContext(ctx, query)
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
				targetRows.Scan(valuePtrs...)
				row := make(map[string]interface{})
				for i, col := range cols {
					row[col] = values[i]
				}
				results = append(results, row)
			}
			if ruleEngineVerbose() {
				log.Printf("[RulesHandler] %s returned %d rows, first row: %v", r.RuleID, len(results), results)
			}
			targetRows.Close()
		}

		currentValue := ""
		recommendedValue := r.RecommendedValue.String
		status := "OK"

		if len(results) > 0 && len(columns) > 0 {
			env := make(map[string]interface{})
			for _, col := range columns {
				val := results[0][col]
				if val != nil {
					switch v := val.(type) {
					case float64:
						env[col] = v
					case int64:
						env[col] = float64(v)
					case int:
						env[col] = float64(v)
					case string:
						env[col] = v
					default:
						env[col] = fmt.Sprintf("%v", v)
					}
				}
			}
			if ruleEngineVerbose() {
				log.Printf("[RulesHandler] %s env for evaluation: %v", r.RuleID, env)
			}
			env["Recommended"] = 0.0

			// Evaluate expected_calc to get recommended value
			if r.ExpectedCalc.Valid && r.ExpectedCalc.String != "" {
				if ruleEngineVerbose() {
					log.Printf("[RulesHandler] Evaluating expected_calc for %s: %s", r.RuleID, r.ExpectedCalc.String)
				}
				program, err := expr.Compile(r.ExpectedCalc.String)
				if err != nil {
					log.Printf("[RulesHandler] Failed to compile expected_calc for %s: %v", r.RuleID, err)
				} else {
					result, err := expr.Run(program, env)
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

			// Evaluate evaluation_logic to get status
			if r.EvaluationLogic.Valid && r.EvaluationLogic.String != "" {
				if ruleEngineVerbose() {
					log.Printf("[RulesHandler] Evaluating evaluation_logic for %s: %s", r.RuleID, r.EvaluationLogic.String)
				}
				program, err := expr.Compile(r.EvaluationLogic.String)
				if err != nil {
					log.Printf("[RulesHandler] Failed to compile evaluation_logic for %s: %v", r.RuleID, err)
				} else {
					result, err := expr.Run(program, env)
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
		} else {
			if ruleEngineVerbose() {
				log.Printf("[RulesHandler] No results for rule %s, query: %s", r.RuleID, query)
			}
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
