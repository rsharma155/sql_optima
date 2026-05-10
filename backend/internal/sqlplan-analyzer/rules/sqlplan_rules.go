// File: internal/rules/rules.go
// Purpose: Rule engine interface and base types for anti-pattern detection
// Package: github.com/rsharma155/sqlplan-analyzer/internal/rules
package rules

import (
	"github.com/rsharma155/sqlplan-analyzer/models"
)

type RuleType int

const (
	RuleTypeAccessPath RuleType = iota
	RuleTypeEstimation
	RuleTypeJoin
	RuleTypeMemory
	RuleTypePredicate
	RuleTypeParallelism
	RuleTypeIndex
	RuleTypeTempDB
	RuleTypeCompute
	RuleTypeBlocking
	RuleTypeWarning
)

type Rule interface {
	Name() string
	Description() string
	Type() RuleType
	Severity() models.Severity
	Evaluate(plan *models.PlanAnalysis) []models.Finding
	Enabled() bool
}

type BaseRule struct {
	name        string
	description string
	ruleType    RuleType
	severity    models.Severity
	enabled    bool
}

func (r *BaseRule) Name() string     { return r.name }
func (r *BaseRule) Description() string { return r.description }
func (r *BaseRule) Type() RuleType  { return r.ruleType }
func (r *BaseRule) Severity() models.Severity { return r.severity }
func (r *BaseRule) Enabled() bool  { return r.enabled }

func CreateBaseRule(name, desc string, rt RuleType, sev models.Severity) BaseRule {
	return BaseRule{
		name:        name,
		description: desc,
		ruleType:    rt,
		severity:    sev,
		enabled:    true,
	}
}

type Registry struct {
	rules map[string]Rule
}

func NewRegistry() *Registry {
	return &Registry{
		rules: make(map[string]Rule),
	}
}

func (r *Registry) Register(rule Rule) {
	r.rules[rule.Name()] = rule
}

func (r *Registry) Get(name string) (Rule, bool) {
	rule, ok := r.rules[name]
	return rule, ok
}

func (r *Registry) GetAll() []Rule {
	result := make([]Rule, 0, len(r.rules))
	for _, rule := range r.rules {
		result = append(result, rule)
	}
	return result
}

func (r *Registry) GetByType(ruleType RuleType) []Rule {
	result := make([]Rule, 0)
	for _, rule := range r.rules {
		if rule.Type() == ruleType {
			result = append(result, rule)
		}
	}
	return result
}

func (r *Registry) EvaluateAll(plan *models.PlanAnalysis) []models.Finding {
	findings := make([]models.Finding, 0)
	for _, rule := range r.rules {
		if rule.Enabled() {
			findings = append(findings, rule.Evaluate(plan)...)
		}
	}
	return findings
}

type Engine struct {
	registry *Registry
}

func NewEngine() *Engine {
	engine := &Engine{
		registry: NewRegistry(),
	}
	engine.registerDefaultRules()
	return engine
}

func (e *Engine) registerDefaultRules() {
	e.registry.Register(NewTableScanRule())
	e.registry.Register(NewIndexScanRule())
	e.registry.Register(NewKeyLookupRule())
	e.registry.Register(NewImplicitConversionRule())
	e.registry.Register(NewSpillRule())
	e.registry.Register(NewMissingIndexRule())
	e.registry.Register(NewParallelismSkewRule())
	e.registry.Register(NewCardinalityEstimationRule())
	e.registry.Register(NewParameterSniffingRule())
	e.registry.Register(NewBadNestedLoopRule())
	e.registry.Register(NewNonSargableRule())
	e.registry.Register(NewScalarUDFRule())
	e.registry.Register(NewHashSpillRule())
}

func (e *Engine) Evaluate(plan *models.PlanAnalysis) []models.Finding {
	return e.registry.EvaluateAll(plan)
}

func (e *Engine) Registry() *Registry {
	return e.registry
}

func (e *Engine) EnableRule(name string) bool {
	_, ok := e.registry.Get(name)
	return ok
}

func (e *Engine) DisableRule(name string) bool {
	_, ok := e.registry.Get(name)
	return ok
}
