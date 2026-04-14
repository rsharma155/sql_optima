// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL client for rule engine operations.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rsharma155/sql_optima/internal/ruleengine/models"
)

var (
	QueryGetRules = "SELECT rule_id, rule_name, category, description, detection_sql, detection_sql_pg, fix_script, fix_script_pg, expected_calc, evaluation_logic, comparison_type, threshold_value, is_enabled, priority, target_db_type FROM ruleengine.rules WHERE is_enabled = true;"
	QueryStartRun = "SELECT ruleengine.start_rule_run($1);"
	QueryStoreRaw = "SELECT ruleengine.store_raw_result($1, $2, $3, $4);"
	QueryEvaluate = "SELECT ruleengine.evaluate_run($1);"
)

type PGClient struct {
	pool *pgxpool.Pool
}

func NewPGClient(ctx context.Context, connStr string) (*PGClient, error) {
	config, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse postgres config: %w", err)
	}

	config.MaxConns = 10
	config.MinConns = 2
	config.MaxConnLifetime = time.Hour
	config.MaxConnIdleTime = 30 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create pgxpool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping postgres: %w", err)
	}

	return &PGClient{pool: pool}, nil
}

func (c *PGClient) Close() {
	if c.pool != nil {
		c.pool.Close()
	}
}

func (c *PGClient) GetEnabledRules(ctx context.Context, instanceType string) ([]models.Rule, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	query := "SELECT rule_id, rule_name, category, description, detection_sql, detection_sql_pg, fix_script, fix_script_pg, comparison_type, threshold_value, is_enabled, priority, target_db_type FROM ruleengine.rules WHERE is_enabled = true AND target_db_type = $1;"

	rows, err := c.pool.Query(ctx, query, instanceType)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch rules: %w", err)
	}
	defer rows.Close()

	var rules []models.Rule
	for rows.Next() {
		var r models.Rule
		var thresh json.RawMessage
		if err := rows.Scan(&r.RuleID, &r.RuleName, &r.Category, &r.Description, &r.DetectionSQL, &r.DetectionSQLPG, &r.FixScript, &r.FixScriptPG, &r.ExpectedCalc, &r.EvaluationLogic, &r.ComparisonType, &thresh, &r.IsEnabled, &r.Priority, &r.TargetDBType); err != nil {
			return nil, fmt.Errorf("failed to scan rule: %w", err)
		}
		r.ThresholdValue = thresh
		rules = append(rules, r)
	}

	return rules, rows.Err()
}

func (c *PGClient) StartRuleRun(ctx context.Context, serverID int) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var runID int
	err := c.pool.QueryRow(ctx, QueryStartRun, serverID).Scan(&runID)
	if err != nil {
		return 0, fmt.Errorf("failed to start rule run: %w", err)
	}

	return runID, nil
}

func (c *PGClient) StoreRawResult(ctx context.Context, runID int, ruleID string, serverID int, payload json.RawMessage) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	_, err := c.pool.Exec(ctx, QueryStoreRaw, runID, ruleID, serverID, payload)
	if err != nil {
		return fmt.Errorf("failed to store raw result for rule %s: %w", ruleID, err)
	}

	return nil
}

func (c *PGClient) EvaluateRun(ctx context.Context, runID int) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	_, err := c.pool.Exec(ctx, QueryEvaluate, runID)
	if err != nil {
		return fmt.Errorf("failed to evaluate run %d: %w", runID, err)
	}

	return nil
}

func (c *PGClient) GetDashboardView(ctx context.Context, serverID int) ([]models.DashboardEntry, error) {
	query := `SELECT rule_id, rule_name, category, status, current_value, recommended_value, description, fix_script, last_check FROM ruleengine.v_best_practices_dashboard WHERE server_id = $1 ORDER BY category, rule_name;`

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	rows, err := c.pool.Query(ctx, query, serverID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch dashboard: %w", err)
	}
	defer rows.Close()

	var entries []models.DashboardEntry
	for rows.Next() {
		var e models.DashboardEntry
		if err := rows.Scan(&e.RuleID, &e.RuleName, &e.Category, &e.Status, &e.CurrentValue, &e.RecommendedValue, &e.Description, &e.FixScript, &e.LastCheck); err != nil {
			return nil, fmt.Errorf("failed to scan dashboard entry: %w", err)
		}
		entries = append(entries, e)
	}

	return entries, rows.Err()
}
