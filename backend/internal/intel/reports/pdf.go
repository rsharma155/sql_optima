// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Health Intelligence Engine
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package reports


import (
	"bytes"
	"fmt"
	"os/exec"

	"github.com/rsharma155/sql_optima/internal/models"
)

func (g *ReportGenerator) GeneratePDF(analysis *models.IntelligenceReportResponse, reportType string, rawData map[string]interface{}, dynamicThresholds map[string]interface{}, serverConfig map[string]interface{}) ([]byte, error) {
	htmlContent, err := g.GenerateHTML(analysis, reportType, rawData, dynamicThresholds, serverConfig)
	if err != nil {
		return nil, fmt.Errorf("generating HTML for PDF: %w", err)
	}

	if hasWkhtmltopdf() {
		return wkhtmltopdfConvert(htmlContent)
	}

	if hasChromeHeadless() {
		return chromeHeadlessConvert(htmlContent)
	}

	return nil, fmt.Errorf("PDF generation requires wkhtmltopdf or Chrome headless installed on the system")
}

func hasWkhtmltopdf() bool {
	_, err := exec.LookPath("wkhtmltopdf")
	return err == nil
}

func hasChromeHeadless() bool {
	_, err := exec.LookPath("chrome")
	if err != nil {
		_, err = exec.LookPath("google-chrome")
	}
	if err != nil {
		_, err = exec.LookPath("chromium")
	}
	if err != nil {
		_, err = exec.LookPath("chromium-browser")
	}
	return err == nil
}

func wkhtmltopdfConvert(html string) ([]byte, error) {
	cmd := exec.Command("wkhtmltopdf", "--encoding", "UTF-8", "-", "-")
	cmd.Stdin = bytes.NewReader([]byte(html))
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("wkhtmltopdf: %w", err)
	}
	return out, nil
}

func chromeHeadlessConvert(html string) ([]byte, error) {
	chromePath := findChromePath()
	cmd := exec.Command(chromePath,
		"--headless",
		"--disable-gpu",
		"--no-margins",
		"--print-to-pdf=-",
		"--run-all-compositor-stages-before-draw",
	)
	cmd.Stdin = bytes.NewReader([]byte(html))
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("chrome headless PDF: %w", err)
	}
	return out, nil
}

func findChromePath() string {
	for _, name := range []string{"google-chrome", "chrome", "chromium", "chromium-browser"} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	return "chrome"
}
