// File: internal/plugin/plugin.go
// Purpose: MCP/Plugin integration interface for external tools
// Package: github.com/rsharma155/sqlplan-analyzer/internal/plugin
package plugin

import (
	"context"
	"fmt"

	"github.com/rsharma155/sqlplan-analyzer/analyzer"
	db "github.com/rsharma155/sqlplan-analyzer/db"
	"github.com/rsharma155/sqlplan-analyzer/models"
	"github.com/rsharma155/sqlplan-analyzer/reporter"
)

type Config struct {
	EnableRules        bool
	EnableScoring    bool
	EnableNarrative   bool
	EnableCostAnalysis bool
}

type Plugin struct {
	analyzer *analyzer.Analyzer
	config   Config
}

func New(cfg Config) *Plugin {
	analyzerCfg := analyzer.Config{
		EnableRules:        cfg.EnableRules,
		EnableScoring:    cfg.EnableScoring,
		EnableNarrative:  cfg.EnableNarrative,
		EnableCostAnalysis: cfg.EnableCostAnalysis,
	}

	return &Plugin{
		analyzer: analyzer.New(analyzerCfg),
		config:   cfg,
	}
}

func (p *Plugin) AnalyzeFile(ctx context.Context, filepath string) (*models.PlanAnalysis, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	plan, err := p.analyzer.ParseFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse file: %w", err)
	}

	return p.analyzer.Analyze(plan), nil
}

func (p *Plugin) AnalyzeBytes(ctx context.Context, data []byte) (*models.PlanAnalysis, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	plan, err := p.analyzer.ParsePlan(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse plan: %w", err)
	}

	return p.analyzer.Analyze(plan), nil
}

func (p *Plugin) AnalyzeWithConfig(ctx context.Context, data []byte, cfg analyzer.Config) (*models.PlanAnalysis, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	customAnalyzer := analyzer.New(cfg)
	plan, err := customAnalyzer.ParsePlan(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse plan: %w", err)
	}

	return customAnalyzer.Analyze(plan), nil
}

type Analyzer interface {
	AnalyzeFile(ctx context.Context, filepath string) (*models.PlanAnalysis, error)
	AnalyzeBytes(ctx context.Context, data []byte) (*models.PlanAnalysis, error)
}

func (p *Plugin) GenerateHTMLReport(plan *models.PlanAnalysis) string {
	r := reporter.NewHTMLReporter()
	return r.Generate(plan)
}

func (p *Plugin) AnalyzeBytesForHTML(ctx context.Context, data []byte) (string, error) {
	plan, err := p.AnalyzeBytes(ctx, data)
	if err != nil {
		return "", err
	}
	return p.GenerateHTMLReport(plan), nil
}

func (p *Plugin) AnalyzeFromDB(ctx context.Context, dbDriver db.Driver, connString string, query string) (string, error) {
	reader, err := db.NewReader(db.Config{Driver: dbDriver, ConnectionString: connString})
	if err != nil {
		return "", fmt.Errorf("failed to create database reader: %w", err)
	}
	defer reader.Close()

	if err := reader.Ping(ctx); err != nil {
		return "", fmt.Errorf("failed to connect to database: %w", err)
	}

	xmlData, err := reader.FetchPlanXML(ctx, query)
	if err != nil {
		return "", fmt.Errorf("failed to fetch plan from database: %w", err)
	}

	return p.AnalyzeBytesForHTML(ctx, xmlData)
}

func RegisterAsMCP() interface{} {
	return &Plugin{
		analyzer: analyzer.New(analyzer.DefaultConfig()),
		config:   Config{},
	}
}

type ToolDefinition struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema struct {
		Type       string `json:"type"`
		Properties map[string]struct {
			Type        string `json:"type"`
			Description string `json:"description"`
		} `json:"properties"`
		Required []string `json:"required"`
	} `json:"inputSchema"`
}

func GetToolDefinitions() []ToolDefinition {
	return []ToolDefinition{
		{
			Name:        "analyze_sql_plan",
			Description: "Analyzes a SQL Server execution plan for performance bottlenecks",
			InputSchema: struct {
				Type       string `json:"type"`
				Properties map[string]struct {
					Type        string `json:"type"`
					Description string `json:"description"`
				} `json:"properties"`
				Required []string `json:"required"`
			}{
				Type: "object",
				Properties: map[string]struct {
					Type        string `json:"type"`
					Description string `json:"description"`
				}{
					"file_path": {
						Type:        "string",
						Description: "Path to the .sqlplan file",
					},
				},
				Required: []string{"file_path"},
			},
		},
	}
}
