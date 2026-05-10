// File: internal/exporter/json.go
// Purpose: JSON export functionality
// Package: github.com/rsharma155/sqlplan-analyzer/internal/exporter
package exporter

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/rsharma155/sqlplan-analyzer/models"
)

type JSONExporter struct {
	PrettyPrint bool
	Compact   bool
}

func NewJSONExporter() *JSONExporter {
	return &JSONExporter{
		PrettyPrint: true,
		Compact:   false,
	}
}

func (e *JSONExporter) Export(plan *models.PlanAnalysis, output io.Writer) error {
	encoder := json.NewEncoder(output)

	if e.PrettyPrint {
	 encoder.SetIndent("", "  ")
	}

	return encoder.Encode(plan)
}

func (e *JSONExporter) ExportToFile(plan *models.PlanAnalysis, filepath string) error {
	file, err := os.Create(filepath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	return e.Export(plan, file)
}

func (e *JSONExporter) ExportString(plan *models.PlanAnalysis) (string, error) {
	encoder := json.NewEncoder(&stringBuilder{})

	if e.PrettyPrint {
		encoder.SetIndent("", "  ")
	}

	err := encoder.Encode(plan)
	if err != nil {
		return "", err
	}

	return "", nil
}

type stringBuilder struct {
	result string
}

func (sb *stringBuilder) Write(p []byte) (n int, err error) {
	sb.result += string(p)
	return len(p), nil
}

func ExportJSON(plan *models.PlanAnalysis) ([]byte, error) {
	exporter := NewJSONExporter()
	exporter.PrettyPrint = true

	result, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JSON: %w", err)
	}

	return result, nil
}
