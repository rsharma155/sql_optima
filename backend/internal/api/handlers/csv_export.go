// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Generic CSV export utilities reusable across all dashboard export endpoints.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"time"
)

// csvWriter wraps http.ResponseWriter with a csv.Writer and handles the common preamble.
type csvWriter struct {
	w *csv.Writer
}

// newCSVResponse sets the response headers for a CSV file download and returns a csvWriter.
// dashboardName is used in the Content-Disposition filename, e.g. "pg_best_practices".
func newCSVResponse(w http.ResponseWriter, dashboardName, instanceName string) *csvWriter {
	now := time.Now().UTC()
	filename := fmt.Sprintf("sql_optima_%s_%s.csv", dashboardName, now.Format("20060102_150405"))

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("Cache-Control", "no-cache")

	cw := csv.NewWriter(w)

	// Standard preamble block
	_ = cw.Write([]string{"SQL Optima — Database Observability Platform"})
	_ = cw.Write([]string{"Generated At", now.Format("2006-01-02 15:04:05 UTC")})
	_ = cw.Write([]string{"Dashboard", humanDashboardName(dashboardName)})
	_ = cw.Write([]string{"Instance", instanceName})
	_ = cw.Write([]string{}) // blank separator

	return &csvWriter{w: cw}
}

// WriteSection emits an optional section sub-header row (bold-style marker) followed by a header row.
func (c *csvWriter) WriteSection(sectionTitle string, headers []string) {
	if sectionTitle != "" {
		_ = c.w.Write([]string{"--- " + sectionTitle + " ---"})
	}
	_ = c.w.Write(headers)
}

// WriteRow writes a single data row.
func (c *csvWriter) WriteRow(row []string) {
	_ = c.w.Write(row)
}

// Flush flushes all buffered data to the underlying writer.
func (c *csvWriter) Flush() {
	c.w.Flush()
}

func humanDashboardName(key string) string {
	switch key {
	case "pg_best_practices":
		return "PostgreSQL Best Practices"
	case "sqlserver_best_practices_guardrails":
		return "SQL Server Best Practices & Guardrails"
	case "sqlserver_guardrails":
		return "SQL Server Workload Guardrails"
	default:
		return key
	}
}
