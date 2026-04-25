package evaluation

type Context map[string]float64

type Result struct {
	RuleID         string             `json:"rule_id"`
	Severity       string             `json:"severity"`
	Confidence     float64            `json:"confidence"`
	Message        string             `json:"message"`
	Recommendation string             `json:"recommendation"`
	Context        map[string]float64 `json:"context"`
}

type Evaluator interface {
	Evaluate(ctx Context) Result
}
