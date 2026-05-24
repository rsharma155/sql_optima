// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Health Intelligence Engine
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package config




import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	AppName    string
	AppVersion string
	Debug      bool
	Port       string

	DatabaseURL string
	RedisURL    string

	AnalysisTimeoutSeconds int
	MaxHistoricalPoints   int
	ForecastHorizonDays    int
	AnomalyStdThreshold    float64

	RiskWeightsPerformance  float64
	RiskWeightsCapacity     float64
	RiskWeightsAvailability float64
	RiskWeightsReplication  float64
	RiskWeightsMaintenance  float64
	RiskWeightsQuery        float64

	ReportOutputDir string
	TemplateDir     string
}

func Load() *Config {
	return &Config{
		AppName:    getEnv("SQLOPTIMA_APP_NAME", "Intelligence Report Sqloptima"),
		AppVersion: getEnv("SQLOPTIMA_APP_VERSION", "1.0.0"),
		Debug:      getEnvBool("SQLOPTIMA_DEBUG", false),
		Port:       getEnv("SQLOPTIMA_PORT", "8000"),

		DatabaseURL: getEnv("SQLOPTIMA_DATABASE_URL", "postgres://postgres:postgres@localhost:5432/intelligence_report_sqloptima?sslmode=disable"),
		RedisURL:    getEnv("SQLOPTIMA_REDIS_URL", "redis://localhost:6379/0"),

		AnalysisTimeoutSeconds: getEnvInt("SQLOPTIMA_ANALYSIS_TIMEOUT", 300),
		MaxHistoricalPoints:    getEnvInt("SQLOPTIMA_MAX_HISTORICAL_POINTS", 100000),
		ForecastHorizonDays:    getEnvInt("SQLOPTIMA_FORECAST_HORIZON_DAYS", 90),
		AnomalyStdThreshold:    getEnvFloat("SQLOPTIMA_ANOMALY_STD_THRESHOLD", 2.5),

		RiskWeightsPerformance:  getEnvFloat("SQLOPTIMA_RISK_PERFORMANCE", 0.25),
		RiskWeightsCapacity:     getEnvFloat("SQLOPTIMA_RISK_CAPACITY", 0.20),
		RiskWeightsAvailability: getEnvFloat("SQLOPTIMA_RISK_AVAILABILITY", 0.20),
		RiskWeightsReplication:  getEnvFloat("SQLOPTIMA_RISK_REPLICATION", 0.15),
		RiskWeightsMaintenance:  getEnvFloat("SQLOPTIMA_RISK_MAINTENANCE", 0.10),
		RiskWeightsQuery:        getEnvFloat("SQLOPTIMA_RISK_QUERY", 0.10),

		ReportOutputDir: getEnv("SQLOPTIMA_REPORT_OUTPUT_DIR", "reports_output"),
		TemplateDir:     getEnv("SQLOPTIMA_TEMPLATE_DIR", "internal/templates"),
	}
}

func (c *Config) RiskWeights() map[string]float64 {
	return map[string]float64{
		"performance":  c.RiskWeightsPerformance,
		"capacity":     c.RiskWeightsCapacity,
		"availability": c.RiskWeightsAvailability,
		"replication":  c.RiskWeightsReplication,
		"maintenance":  c.RiskWeightsMaintenance,
		"query":        c.RiskWeightsQuery,
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

func getEnvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return i
}

func getEnvFloat(key string, fallback float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return fallback
	}
	return f
}

func DSN() string {
	cfg := Load()
	return fmt.Sprintf("%s?sslmode=disable", cfg.DatabaseURL)
}