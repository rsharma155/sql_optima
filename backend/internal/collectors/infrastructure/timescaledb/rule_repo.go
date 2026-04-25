package timescaledb

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rsharma155/sql_optima/internal/collectors/domain/ruleengine"
)

type RuleRepository struct {
	pool *pgxpool.Pool
}

func NewRuleRepository(pool *pgxpool.Pool) *RuleRepository {
	return &RuleRepository{pool: pool}
}

func (r *RuleRepository) GetMSSQLIgnoreRules(ctx context.Context) ([]ruleengine.IgnoreRule, error) {
	query := `SELECT rule_type, rule_value FROM ruleengine.sqlserver_ignore_rules`
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []ruleengine.IgnoreRule
	for rows.Next() {
		var rule ruleengine.IgnoreRule
		if err := rows.Scan(&rule.Type, &rule.Value); err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, nil
}

func (r *RuleRepository) GetPGIgnoreRules(ctx context.Context) ([]ruleengine.IgnoreRule, error) {
	query := `SELECT rule_type, rule_value FROM ruleengine.pg_ignore_rules`
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []ruleengine.IgnoreRule
	for rows.Next() {
		var rule ruleengine.IgnoreRule
		if err := rows.Scan(&rule.Type, &rule.Value); err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, nil
}
