package models

import (
	"encoding/json"
	"time"
)

type Config struct {
	InstanceID    int           `json:"instance_id"`
	InstanceName  string        `json:"instance_name"`
	InstanceType  string        `json:"instance_type"` // "sqlserver" or "postgres"
	ConnectionStr string        `json:"connection_str"`
	DBName        string        `json:"db_name"`
	Name          string        `json:"name"`
	Password      string        `json:"password"`
	WorkerCount   int           `json:"worker_count"`
	PollInterval  time.Duration `json:"poll_interval"`
}

type Rule struct {
	RuleID          string          `json:"rule_id"`
	RuleName        string          `json:"rule_name"`
	Category        string          `json:"category"`
	Description     string          `json:"description"`
	DetectionSQL    string          `json:"detection_sql"`
	DetectionSQLPG  string          `json:"detection_sql_pg"`
	FixScript       string          `json:"fix_script"`
	FixScriptPG     string          `json:"fix_script_pg"`
	ExpectedCalc    string          `json:"expected_calc"`    // Dynamic formula for recommended value
	EvaluationLogic string          `json:"evaluation_logic"` // Dynamic formula for status
	RecommendedVal  string          `json:"recommended_value"`
	ComparisonType  string          `json:"comparison_type"` // "exact", "range", "threshold"
	ThresholdValue  json.RawMessage `json:"threshold_value"`
	TargetDBType    string          `json:"target_db_type"` // "sqlserver" or "postgres"
	IsEnabled       bool            `json:"is_enabled"`
	Priority        int             `json:"priority"`
}

type DetectionPayload struct {
	RunID            int                      `json:"run_id"`
	RuleID           string                   `json:"rule_id"`
	ServerID         int                      `json:"server_id"`
	RuleName         string                   `json:"rule_name"`
	Category         string                   `json:"category"`
	RawResults       []map[string]interface{} `json:"raw_results"`
	CurrentValue     string                   `json:"current_value"`
	RecommendedValue string                   `json:"recommended_value"`
	Status           string                   `json:"status"`
	DetectedAt       time.Time                `json:"detected_at"`
	Error            string                   `json:"error,omitempty"`
}

type RuleResult struct {
	RunID            int             `json:"run_id"`
	RuleID           string          `json:"rule_id"`
	ServerID         int             `json:"server_id"`
	TargetDBType     string          `json:"target_db_type"`
	RuleName         string          `json:"rule_name"`
	Category         string          `json:"category"`
	Status           string          `json:"status"` // "OK", "WARNING", "CRITICAL", "ERROR"
	CurrentValue     string          `json:"current_value"`
	RecommendedValue string          `json:"recommended_value"`
	Description      string          `json:"description"`
	FixScript        string          `json:"fix_script"`
	JSONPayload      json.RawMessage `json:"json_payload"`
	EvaluatedAt      time.Time       `json:"evaluated_at"`
}

type DashboardEntry struct {
	RuleID           string    `json:"rule_id"`
	RuleName         string    `json:"rule_name"`
	Category         string    `json:"category"`
	Status           string    `json:"status"`
	CurrentValue     string    `json:"current_value"`
	RecommendedValue string    `json:"recommended_value"`
	Description      string    `json:"description"`
	FixScript        string    `json:"fix_script"`
	LastCheck        time.Time `json:"last_check"`
}
