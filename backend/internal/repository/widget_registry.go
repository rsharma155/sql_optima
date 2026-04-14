// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Widget registry repository for dashboard customization persistence.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rsharma155/sql_optima/internal/models"
	"github.com/rsharma155/sql_optima/internal/security/sqlsandbox"
)

// WidgetRepository handles CRUD operations for the optima_ui_widgets registry
// and dynamic query execution against TimescaleDB.
type WidgetRepository struct {
	pool *pgxpool.Pool
}

// NewWidgetRepository creates a new widget registry repository.
func NewWidgetRepository(pool *pgxpool.Pool) *WidgetRepository {
	return &WidgetRepository{pool: pool}
}

// Pool returns the underlying database pool (used by admin endpoints).
func (r *WidgetRepository) Pool() *pgxpool.Pool {
	return r.pool
}

// GetAllWidgets returns all widget metadata (public view, no SQL).
func (r *WidgetRepository) GetAllWidgets(ctx context.Context) ([]models.UIWidgetPublic, error) {
	query := `SELECT widget_id, dashboard_section, title, chart_type, updated_at FROM optima_ui_widgets ORDER BY dashboard_section, widget_id`
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch widgets: %w", err)
	}
	defer rows.Close()

	var widgets []models.UIWidgetPublic
	for rows.Next() {
		var w models.UIWidgetPublic
		if err := rows.Scan(&w.WidgetID, &w.DashboardSection, &w.Title, &w.ChartType, &w.UpdatedAt); err != nil {
			log.Printf("[WidgetRepo] Scan error: %v", err)
			continue
		}
		widgets = append(widgets, w)
	}
	return widgets, rows.Err()
}

// GetWidgetsByInstance returns all widgets with their current SQL for a specific instance type.
func (r *WidgetRepository) GetWidgetsByInstance(instanceName string) ([]models.UIWidget, error) {
	query := `SELECT widget_id, dashboard_section, title, chart_type, COALESCE(current_sql, default_sql) as sql, default_sql, updated_at FROM optima_ui_widgets ORDER BY dashboard_section, widget_id`
	rows, err := r.pool.Query(context.Background(), query)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch widgets: %w", err)
	}
	defer rows.Close()

	var widgets []models.UIWidget
	for rows.Next() {
		var w models.UIWidget
		if err := rows.Scan(&w.WidgetID, &w.DashboardSection, &w.Title, &w.ChartType, &w.CurrentSQL, &w.DefaultSQL, &w.UpdatedAt); err != nil {
			log.Printf("[WidgetRepo] Scan error: %v", err)
			continue
		}
		widgets = append(widgets, w)
	}
	return widgets, rows.Err()
}

// GetWidgetByID returns a single widget with its SQL (admin view).
func (r *WidgetRepository) GetWidgetByID(ctx context.Context, widgetID string) (*models.UIWidget, error) {
	query := `SELECT widget_id, dashboard_section, title, chart_type, current_sql, default_sql, updated_at FROM optima_ui_widgets WHERE widget_id = $1`
	var w models.UIWidget
	err := r.pool.QueryRow(ctx, query, widgetID).Scan(&w.WidgetID, &w.DashboardSection, &w.Title, &w.ChartType, &w.CurrentSQL, &w.DefaultSQL, &w.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("widget %s not found: %w", widgetID, err)
	}
	return &w, nil
}

// UpdateWidgetSQL updates the current_sql for a widget.
func (r *WidgetRepository) UpdateWidgetSQL(ctx context.Context, widgetID, newSQL string) error {
	query := `UPDATE optima_ui_widgets SET current_sql = $1, updated_at = $2 WHERE widget_id = $3`
	result, err := r.pool.Exec(ctx, query, newSQL, time.Now().UTC(), widgetID)
	if err != nil {
		return fmt.Errorf("failed to update widget %s: %w", widgetID, err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("widget %s not found", widgetID)
	}
	return nil
}

// RestoreWidgetDefault copies default_sql back into current_sql.
func (r *WidgetRepository) RestoreWidgetDefault(ctx context.Context, widgetID string) error {
	query := `UPDATE optima_ui_widgets SET current_sql = default_sql, updated_at = $1 WHERE widget_id = $2`
	result, err := r.pool.Exec(ctx, query, time.Now().UTC(), widgetID)
	if err != nil {
		return fmt.Errorf("failed to restore widget %s: %w", widgetID, err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("widget %s not found", widgetID)
	}
	return nil
}

// ExecuteWidgetQuery fetches the current_sql for a widget, injects parameters,
// executes it against TimescaleDB, and returns dynamic JSON results.
func (r *WidgetRepository) ExecuteWidgetQuery(ctx context.Context, widgetID string, params map[string]string) ([]map[string]interface{}, error) {
	widget, err := r.GetWidgetByID(ctx, widgetID)
	if err != nil {
		return nil, err
	}

	sql := widget.CurrentSQL

	// Safely inject parameters using named placeholders {{key}}
	for key, value := range params {
		placeholder := "{{" + key + "}}"
		// Validate: only allow alphanumeric + underscore values to prevent SQL injection
		if !isValidParamValue(value) {
			return nil, fmt.Errorf("invalid parameter value for '%s': contains disallowed characters", key)
		}
		sql = strings.ReplaceAll(sql, placeholder, value)
	}

	// Verify no unresolved placeholders remain
	if strings.Contains(sql, "{{") {
		return nil, fmt.Errorf("unresolved parameter placeholders in query")
	}

	wrapped, err := sqlsandbox.WrapWithRowLimit("postgres", sql, sqlsandbox.DefaultMaxRows)
	if err != nil {
		return nil, fmt.Errorf("widget sql sandbox: %w", err)
	}

	rows, err := r.pool.Query(ctx, wrapped)
	if err != nil {
		return nil, fmt.Errorf("query execution failed for widget %s: %w", widgetID, err)
	}
	defer rows.Close()

	// Get column names
	fieldDescriptions := rows.FieldDescriptions()
	columnNames := make([]string, len(fieldDescriptions))
	for i, fd := range fieldDescriptions {
		columnNames[i] = string(fd.Name)
	}

	// Scan rows into dynamic maps
	var results []map[string]interface{}
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			log.Printf("[WidgetRepo] Row scan error: %v", err)
			continue
		}
		rowMap := make(map[string]interface{})
		for i, col := range columnNames {
			if values[i] == nil {
				rowMap[col] = nil
			} else {
				rowMap[col] = values[i]
			}
		}
		results = append(results, rowMap)
	}

	return results, rows.Err()
}

// isValidParamValue checks that a parameter value only contains safe characters.
// Allows: alphanumeric, underscore, hyphen, dot, space, forward-slash, colon, asterisk.
func isValidParamValue(v string) bool {
	for _, c := range v {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
			c == '_' || c == '-' || c == '.' || c == ' ' || c == '/' || c == ':' || c == '*') {
			return false
		}
	}
	return true
}
