// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Rule engine execution runner for best practices evaluation.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/expr-lang/expr"
	"github.com/rsharma155/sql_optima/internal/ruleengine/collectors"
	"github.com/rsharma155/sql_optima/internal/ruleengine/models"
	"github.com/rsharma155/sql_optima/internal/ruleengine/postgres"
)

const (
	WorkerPoolSize      = 5
	RuleTimeout         = 30 * time.Second
	PostgresFuncTimeout = 10 * time.Second
)

type Runner struct {
	pgClient     *postgres.PGClient
	sqlServerCol *collectors.SQLServerCollector
	pgCol        *collectors.PostgresCollector
	workerPool   int
	rules        []models.Rule
	mu           sync.RWMutex
	wg           sync.WaitGroup
	stopChan     chan struct{}
}

func NewRunner(pgClient *postgres.PGClient, workerPool int) *Runner {
	return &Runner{
		pgClient:   pgClient,
		workerPool: workerPool,
		stopChan:   make(chan struct{}),
	}
}

func (r *Runner) SetSQLServerCollector(col *collectors.SQLServerCollector) {
	r.mu.Lock()
	r.sqlServerCol = col
	r.mu.Unlock()
}

func (r *Runner) SetPostgresCollector(col *collectors.PostgresCollector) {
	r.mu.Lock()
	r.pgCol = col
	r.mu.Unlock()
}

func (r *Runner) Start(ctx context.Context, serverID int, instanceType string) error {
	log.Printf("[Engine] Starting rule engine for server_id=%d, instance_type=%s", serverID, instanceType)

	rules, err := r.pgClient.GetEnabledRules(ctx, instanceType)
	if err != nil {
		return fmt.Errorf("failed to load rules: %w", err)
	}

	if len(rules) == 0 {
		log.Printf("[Engine] No enabled rules found")
		return nil
	}

	r.mu.Lock()
	r.rules = rules
	r.mu.Unlock()

	runID, err := r.pgClient.StartRuleRun(ctx, serverID)
	if err != nil {
		return fmt.Errorf("failed to start rule run: %w", err)
	}

	log.Printf("[Engine] Started run_id=%d with %d rules", runID, len(rules))

	rulesChan := make(chan models.Rule, len(rules))
	for _, rule := range rules {
		rulesChan <- rule
	}
	close(rulesChan)

	resultsChan := make(chan models.DetectionPayload, len(rules))

	for i := 0; i < r.workerPool; i++ {
		r.wg.Add(1)
		go r.worker(ctx, i, rulesChan, resultsChan)
	}

	go func() {
		r.wg.Wait()
		close(resultsChan)
	}()

	go r.resultProcessor(ctx, resultsChan, runID, serverID)

	log.Printf("[Engine] Dispatched %d rules to worker pool", len(rules))
	return nil
}

func (r *Runner) worker(ctx context.Context, workerID int, rulesChan <-chan models.Rule, resultsChan chan<- models.DetectionPayload) {
	defer r.wg.Done()

	for rule := range rulesChan {
		r.processRule(ctx, workerID, rule, resultsChan)
	}
}

func (r *Runner) processRule(ctx context.Context, workerID int, rule models.Rule, resultsChan chan<- models.DetectionPayload) {
	payload := models.DetectionPayload{
		RunID:      0,
		RuleID:     rule.RuleID,
		RuleName:   rule.RuleName,
		Category:   rule.Category,
		DetectedAt: time.Now(),
	}

	ruleCtx, cancel := context.WithTimeout(ctx, RuleTimeout)
	defer cancel()

	var results []map[string]interface{}
	var err error

	r.mu.RLock()
	sqlCol := r.sqlServerCol
	pgCol := r.pgCol
	r.mu.RUnlock()

	detectionSQL := rule.DetectionSQL
	if rule.TargetDBType == "postgres" && rule.DetectionSQLPG != "" {
		detectionSQL = rule.DetectionSQLPG
	}

	switch rule.TargetDBType {
	case "postgres":
		if pgCol != nil {
			results, _, err = pgCol.ExecuteRule(ruleCtx, detectionSQL)
		} else {
			err = fmt.Errorf("postgres collector not available")
		}
	case "sqlserver":
		if sqlCol != nil {
			results, _, err = sqlCol.ExecuteRule(ruleCtx, detectionSQL)
		} else {
			err = fmt.Errorf("sqlserver collector not available")
		}
	default:
		err = fmt.Errorf("unknown target_db_type: %s", rule.TargetDBType)
	}

	if err != nil {
		log.Printf("[Worker-%d] Rule %s (ID=%s) failed: %v", workerID, rule.RuleName, rule.RuleID, err)
		payload.Error = err.Error()
		resultsChan <- payload
		return
	}

	// Process dynamic evaluation if results exist
	if len(results) > 0 {
		env := results[0] // Use first row as environment

		// Normalise nil values to float64(0) so expressions don't crash with <nil> operands.
		// Also coerce string-encoded numbers to int64/float64 so comparison operators work.
		for k, v := range env {
			if v == nil {
				env[k] = float64(0)
				continue
			}
			if s, ok := v.(string); ok {
				if i, err := strconv.ParseInt(s, 10, 64); err == nil {
					env[k] = i
					continue
				}
				if f, err := strconv.ParseFloat(s, 64); err == nil {
					env[k] = f
				}
			}
		}

		// Step A: Calculate Recommended Value using expected_calc
		if rule.ExpectedCalc != "" {
			program, err := expr.Compile(rule.ExpectedCalc, expr.Env(env))
			if err != nil {
				log.Printf("[Worker-%d] Rule %s: failed to compile expected_calc: %v", workerID, rule.RuleName, err)
			} else {
				result, err := expr.Run(program, env)
				if err != nil {
					log.Printf("[Worker-%d] Rule %s: failed to run expected_calc: %v", workerID, rule.RuleName, err)
				} else {
					env["Recommended"] = result
					log.Printf("[Worker-%d] Rule %s: calculated Recommended=%v", workerID, rule.RuleName, result)
				}
			}
		}

		// Step B: Evaluate Status using evaluation_logic
		if rule.EvaluationLogic != "" {
			program, err := expr.Compile(rule.EvaluationLogic, expr.Env(env))
			if err != nil {
				log.Printf("[Worker-%d] Rule %s: failed to compile evaluation_logic: %v", workerID, rule.RuleName, err)
			} else {
				result, err := expr.Run(program, env)
				if err != nil {
					log.Printf("[Worker-%d] Rule %s: failed to run evaluation_logic: %v", workerID, rule.RuleName, err)
				} else {
					statusStr := fmt.Sprintf("%v", result)
					env["EvaluatedStatus"] = statusStr
					log.Printf("[Worker-%d] Rule %s: evaluated status=%s", workerID, rule.RuleName, statusStr)
				}
			}
		}

		// Step C: Extract current value for display
		if val, ok := env["CurrentValue"]; ok {
			payload.CurrentValue = fmt.Sprintf("%v", val)
		} else if val, ok := env["current_value"]; ok {
			payload.CurrentValue = fmt.Sprintf("%v", val)
		} else if len(env) > 0 {
			// Use first non-system key as current value
			for k, v := range env {
				if k != "Recommended" && k != "EvaluatedStatus" && k != "recommended_value" && k != "status" {
					payload.CurrentValue = fmt.Sprintf("%v", v)
					break
				}
			}
		}

		// If Recommended was calculated, store it in env for storage
		if rec, ok := env["Recommended"]; ok {
			env["recommended_value"] = fmt.Sprintf("%v", rec)
			payload.RecommendedValue = fmt.Sprintf("%v", rec)
		}
		// Store the evaluated status
		if status, ok := env["EvaluatedStatus"]; ok {
			payload.Status = fmt.Sprintf("%v", status)
		}
	}

	payload.RawResults = results

	select {
	case resultsChan <- payload:
	default:
	}
}

func (r *Runner) resultProcessor(ctx context.Context, resultsChan <-chan models.DetectionPayload, runID int, serverID int) {
	for payload := range resultsChan {
		if payload.Error != "" {
			log.Printf("[Engine] Rule %s (ID=%s) error: %s", payload.RuleName, payload.RuleID, payload.Error)
			continue
		}

		// Enhance raw results with dynamic evaluation
		if len(payload.RawResults) > 0 {
			// Add evaluated status and recommended to the first result row
			if payload.Status != "" {
				payload.RawResults[0]["EvaluatedStatus"] = payload.Status
			}
			if payload.RecommendedValue != "" {
				payload.RawResults[0]["recommended_value"] = payload.RecommendedValue
			}
			if payload.CurrentValue != "" {
				payload.RawResults[0]["CurrentValue"] = payload.CurrentValue
			}
		}

		jsonPayload, err := json.Marshal(payload.RawResults)
		if err != nil {
			log.Printf("[Engine] Failed to marshal payload for rule %s: %v", payload.RuleID, err)
			continue
		}

		storeCtx, cancel := context.WithTimeout(ctx, PostgresFuncTimeout)
		err = r.pgClient.StoreRawResult(storeCtx, runID, payload.RuleID, serverID, jsonPayload)
		cancel()

		if err != nil {
			log.Printf("[Engine] Failed to store result for rule %s: %v", payload.RuleID, err)
		}
	}

	evalCtx, cancel := context.WithTimeout(ctx, PostgresFuncTimeout)
	err := r.pgClient.EvaluateRun(evalCtx, runID)
	cancel()

	if err != nil {
		log.Printf("[Engine] Failed to evaluate run %d: %v", runID, err)
		return
	}

	log.Printf("[Engine] Run %d completed and evaluated", runID)
}

func (r *Runner) Stop() {
	close(r.stopChan)
	r.wg.Wait()
	log.Printf("[Engine] Runner stopped")
}
