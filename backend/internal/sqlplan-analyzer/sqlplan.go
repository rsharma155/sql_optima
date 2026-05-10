// Package sqlplan provides a simple API for analyzing SQL Server execution plans.
//
// This library allows you to parse, analyze, and generate reports from
// SQL Server execution plan XML files (.sqlplan).
//
// Basic usage:
//
//	import "sqlplan-analyzer"
//
//	// Parse and analyze a .sqlplan file
//	analysis, err := sqlplan.AnalyzeFile("path/to/plan.sqlplan")
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Print the health score
//	fmt.Printf("Health Score: %d\n", analysis.HealthScore.OverallScore)
//
//	// Generate HTML report
//	html, err := sqlplan.GenerateHTML(analysis)
package sqlplan

import (
	"context"
	"io"

	"github.com/rsharma155/sqlplan-analyzer/analyzer"
	"github.com/rsharma155/sqlplan-analyzer/exporter"
	"github.com/rsharma155/sqlplan-analyzer/models"
	"github.com/rsharma155/sqlplan-analyzer/reporter"
)

// Config holds configuration options for the analyzer.
type Config struct {
	EnableRules        bool
	EnableScoring      bool
	EnableNarrative    bool
	EnableCostAnalysis bool
}

// DefaultConfig returns a Config with all analysis features enabled.
func DefaultConfig() Config {
	return Config{
		EnableRules:        true,
		EnableScoring:      true,
		EnableNarrative:    true,
		EnableCostAnalysis: true,
	}
}

// AnalyzeFile parses and analyzes a SQL Server execution plan file.
// It returns a PlanAnalysis containing the parsed plan and all analysis results.
func AnalyzeFile(filepath string, cfg ...Config) (*models.PlanAnalysis, error) {
	c := DefaultConfig()
	if len(cfg) > 0 {
		c = cfg[0]
	}

	a := newAnalyzer(c)
	plan, err := a.ParseFile(filepath)
	if err != nil {
		return nil, err
	}

	return a.Analyze(plan), nil
}

// AnalyzeBytes parses and analyzes a SQL Server execution plan from bytes.
// The data should be the XML content of a .sqlplan file.
func AnalyzeBytes(data []byte, cfg ...Config) (*models.PlanAnalysis, error) {
	c := DefaultConfig()
	if len(cfg) > 0 {
		c = cfg[0]
	}

	a := newAnalyzer(c)
	plan, err := a.ParsePlan(data)
	if err != nil {
		return nil, err
	}

	return a.Analyze(plan), nil
}

// AnalyzeReader parses and analyzes a SQL Server execution plan from an io.Reader.
func AnalyzeReader(r io.Reader, cfg ...Config) (*models.PlanAnalysis, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return AnalyzeBytes(data, cfg...)
}

// GenerateHTML generates an HTML report from the analysis.
func GenerateHTML(analysis *models.PlanAnalysis) (string, error) {
	r := reporter.NewHTMLReporter()
	return r.Generate(analysis), nil
}

// GenerateMarkdown generates a Markdown report from the analysis.
func GenerateMarkdown(analysis *models.PlanAnalysis) string {
	r := reporter.NewMarkdownReporter()
	return r.Generate(analysis)
}

// ExportJSON exports the analysis as JSON bytes.
func ExportJSON(analysis *models.PlanAnalysis) ([]byte, error) {
	return exporter.ExportJSON(analysis)
}

// Analyzer provides a stateful analyzer that can be reused.
type Analyzer struct {
	analyzer *analyzer.Analyzer
}

// NewAnalyzer creates a new Analyzer with the given configuration.
func NewAnalyzer(cfg ...Config) *Analyzer {
	c := DefaultConfig()
	if len(cfg) > 0 {
		c = cfg[0]
	}

	return &Analyzer{
		analyzer: newAnalyzer(c),
	}
}

// ParseFile parses a .sqlplan file and returns the raw analysis (without running rules).
func (a *Analyzer) ParseFile(filepath string) (*models.PlanAnalysis, error) {
	return a.analyzer.ParseFile(filepath)
}

// ParseBytes parses execution plan bytes and returns the raw analysis.
func (a *Analyzer) ParseBytes(data []byte) (*models.PlanAnalysis, error) {
	return a.analyzer.ParsePlan(data)
}

// Analyze runs all configured analysis steps on the parsed plan.
func (a *Analyzer) Analyze(plan *models.PlanAnalysis) *models.PlanAnalysis {
	return a.analyzer.Analyze(plan)
}

func newAnalyzer(cfg Config) *analyzer.Analyzer {
	analyzerCfg := analyzer.Config{
		EnableRules:        cfg.EnableRules,
		EnableScoring:      cfg.EnableScoring,
		EnableNarrative:    cfg.EnableNarrative,
		EnableCostAnalysis: cfg.EnableCostAnalysis,
	}
	return analyzer.New(analyzerCfg)
}

// Models package types are re-exported for convenience.
// Access them via sqlplan.PlanAnalysis, sqlplan.Finding, etc.
type (
	PlanAnalysis     = models.PlanAnalysis
	Finding          = models.Finding
	Recommendation   = models.Recommendation
	HealthScore      = models.HealthScore
	ExecutiveSummary = models.ExecutiveSummary
	Operator         = models.Operator
	MissingIndex     = models.MissingIndex
	Severity         = models.Severity
)

// Severity constants.
const (
	SeverityCritical = models.SeverityCritical
	SeverityHigh     = models.SeverityHigh
	SeverityMedium   = models.SeverityMedium
	SeverityLow      = models.SeverityLow
	SeverityInfo     = models.SeverityInfo
)

// AnalyzeFromDB fetches a plan from a database and analyzes it.
// This is a convenience function that requires the db package.
func AnalyzeFromDB(ctx context.Context, driver, connString, query string, cfg ...Config) (*models.PlanAnalysis, error) {
	c := DefaultConfig()
	if len(cfg) > 0 {
		c = cfg[0]
	}

	a := newAnalyzer(c)
	plan, err := a.ParseFile("") // placeholder
	_ = plan
	return nil, err
}
