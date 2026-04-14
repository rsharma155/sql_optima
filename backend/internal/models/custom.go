// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Custom JSON type implementation for flexible data storage and database serialization.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package models

import (
	"database/sql/driver"
	"encoding/json"
	"time"
)

type JSON map[string]interface{}

func (j JSON) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

func (j *JSON) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}
	b, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(b, j)
}

type UserDashboard struct {
	ID            int       `json:"id"`
	UserID        int       `json:"user_id"`
	DashboardName string    `json:"dashboard_name"`
	DashboardType string    `json:"dashboard_type"`
	LayoutConfig  JSON      `json:"layout_config"`
	IsDefault     bool      `json:"is_default"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type DashboardWidget struct {
	ID          int       `json:"id"`
	DashboardID int       `json:"dashboard_id"`
	WidgetType  string    `json:"widget_type"`
	WidgetTitle string    `json:"widget_title"`
	MetricName  string    `json:"metric_name"`
	ChartType   string    `json:"chart_type"`
	PositionX   int       `json:"position_x"`
	PositionY   int       `json:"position_y"`
	Width       int       `json:"width"`
	Height      int       `json:"height"`
	Config      JSON      `json:"config"`
	CreatedAt   time.Time `json:"created_at"`
}

type AlertThreshold struct {
	ID                 int       `json:"id"`
	UserID             int       `json:"user_id"`
	MetricName         string    `json:"metric_name"`
	ThresholdName      string    `json:"threshold_name"`
	ThresholdType      string    `json:"threshold_type"`
	ConditionType      string    `json:"condition_type"`
	WarningThreshold   float64   `json:"warning_threshold"`
	CriticalThreshold  *float64  `json:"critical_threshold"`
	EvaluationInterval string    `json:"evaluation_interval"`
	EvaluationWindow   string    `json:"evaluation_window"`
	IsEnabled          bool      `json:"is_enabled"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type NotificationChannel struct {
	ID          int       `json:"id"`
	UserID      int       `json:"user_id"`
	ChannelName string    `json:"channel_name"`
	ChannelType string    `json:"channel_type"`
	Config      JSON      `json:"config"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type AlertSubscription struct {
	ID          int       `json:"id"`
	ThresholdID int       `json:"threshold_id"`
	ChannelID   int       `json:"channel_id"`
	CreatedAt   time.Time `json:"created_at"`
}

type AlertHistory struct {
	ID             int        `json:"id"`
	ThresholdID    *int       `json:"threshold_id"`
	InstanceName   string     `json:"instance_name"`
	MetricName     string     `json:"metric_name"`
	MetricValue    float64    `json:"metric_value"`
	Severity       string     `json:"severity"`
	Message        *string    `json:"message"`
	Acknowledged   bool       `json:"acknowledged"`
	AcknowledgedBy *int       `json:"acknowledged_by"`
	AcknowledgedAt *time.Time `json:"acknowledged_at"`
	CreatedAt      time.Time  `json:"created_at"`
}

type MonitoredServer struct {
	ID                  int       `json:"id"`
	UserID              int       `json:"user_id"`
	ServerName          string    `json:"server_name"`
	ServerType          string    `json:"server_type"`
	Host                string    `json:"host"`
	Port                int       `json:"port"`
	DatabaseName        *string   `json:"database_name"`
	ConnectionEncrypted *string   `json:"connection_string_encrypted"`
	IsActive            bool      `json:"is_active"`
	CollectionEnabled   bool      `json:"collection_enabled"`
	CollectionInterval  string    `json:"collection_interval"`
	Tags                JSON      `json:"tags"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

type MetricCollectionSetting struct {
	ID                 int       `json:"id"`
	ServerID           int       `json:"server_id"`
	MetricCategory     string    `json:"metric_category"`
	IsEnabled          bool      `json:"is_enabled"`
	CollectionInterval string    `json:"collection_interval"`
	RetentionPeriod    string    `json:"retention_period"`
	Config             JSON      `json:"config"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type DashboardExport struct {
	ID         int       `json:"id"`
	UserID     int       `json:"user_id"`
	ExportName string    `json:"export_name"`
	ExportType string    `json:"export_type"`
	ExportData JSON      `json:"export_data"`
	CreatedAt  time.Time `json:"created_at"`
}

type DashboardExportRequest struct {
	ExportName string `json:"export_name"`
	ExportType string `json:"export_type"`
	IDs        []int  `json:"ids"`
}

type DashboardImportRequest struct {
	ImportName string `json:"import_name"`
	ImportData JSON   `json:"import_data"`
}

type AlertEvaluationResult struct {
	ThresholdID int     `json:"threshold_id"`
	MetricName  string  `json:"metric_name"`
	MetricValue float64 `json:"metric_value"`
	Severity    string  `json:"severity"`
	Message     string  `json:"message"`
	Instance    string  `json:"instance"`
}

type DashboardLayout struct {
	Columns int                `json:"columns"`
	Widgets []WidgetLayoutItem `json:"widgets"`
}

type WidgetLayoutItem struct {
	ID         string `json:"id"`
	X          int    `json:"x"`
	Y          int    `json:"y"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	MetricName string `json:"metric_name"`
	ChartType  string `json:"chart_type"`
	Title      string `json:"title"`
}
