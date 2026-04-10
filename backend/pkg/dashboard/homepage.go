package dashboard

import "time"

// Severity is a small, UI-friendly severity label.
type Severity string

const (
	SeverityOK       Severity = "OK"
	SeverityWarning  Severity = "WARNING"
	SeverityCritical Severity = "CRITICAL"
)

// HealthInputs are the raw signals used to compute health score.
// All pointer fields are optional; nil means "unknown / not collected".
type HealthInputs struct {
	BlockingSessions     *int
	MaxLogUsedPercent    *float64
	TempdbUsedPercent    *float64
	MemoryGrantsPending  *int
	FailedLoginsLast5Min *int
	PLE                 *float64
	DominantWaitIsWriteLog *bool
}

type HealthScore struct {
	Score    int      `json:"score"`
	Severity Severity `json:"severity"`
	Label    string   `json:"label"`
}

// ComputeHealthScore computes a 0–100 score using the deduction rules in Upgrade_main_dashboard.md.
// Missing (nil) inputs do not affect the score.
func ComputeHealthScore(in HealthInputs) HealthScore {
	score := 100

	if in.BlockingSessions != nil && *in.BlockingSessions > 0 {
		score -= 15
	}
	if in.MaxLogUsedPercent != nil && *in.MaxLogUsedPercent > 80.0 {
		score -= 20
	}
	if in.TempdbUsedPercent != nil && *in.TempdbUsedPercent > 80.0 {
		score -= 20
	}
	if in.MemoryGrantsPending != nil && *in.MemoryGrantsPending > 0 {
		score -= 15
	}
	if in.FailedLoginsLast5Min != nil && *in.FailedLoginsLast5Min > 0 {
		score -= 10
	}
	if in.PLE != nil && *in.PLE < 300.0 {
		score -= 10
	}
	if in.DominantWaitIsWriteLog != nil && *in.DominantWaitIsWriteLog {
		score -= 10
	}

	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}

	out := HealthScore{Score: score}
	switch {
	case score >= 80:
		out.Severity = SeverityOK
		out.Label = "Healthy"
	case score >= 50:
		out.Severity = SeverityWarning
		out.Label = "Warning"
	default:
		out.Severity = SeverityCritical
		out.Label = "Critical"
	}
	return out
}

// HomepageV2 is the Phase-1 payload shape (DBA homepage layout).
// It is designed to be backward compatible with existing clients by introducing a new endpoint.
type HomepageV2 struct {
	InstanceName string    `json:"instance_name"`
	Timestamp    string    `json:"timestamp"`
	GeneratedAt  time.Time `json:"generated_at"`

	HealthRisk        map[string]any `json:"health_risk"`
	WorkloadCapacity  map[string]any `json:"workload_capacity"`
	RootCause         map[string]any `json:"root_cause"`
	MemoryStorage     map[string]any `json:"memory_storage_internals"`
	LiveDiagnostics   map[string]any `json:"live_diagnostics"`
	Compat            map[string]any `json:"compat,omitempty"` // optional: legacy fields for gradual frontend migration
}

