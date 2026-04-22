package result

import (
	"encoding/json"
	"time"
)

type Result struct {
	RunID        int64           `json:"run_id"`
	ServerID     int             `json:"server_id"`
	RuleID       string          `json:"rule_id"`
	TargetDBType string          `json:"target_db_type"`
	Status       string          `json:"status"`
	CurrentValue string          `json:"current_value"`
	Recommended  string          `json:"recommended"`
	Severity     string          `json:"severity"`
	Confidence   float64         `json:"confidence"`
	Context      json.RawMessage `json:"context"`
	EvaluatedAt  time.Time       `json:"evaluated_at"`
}
