package models

import "time"

// UIWidget represents a single dashboard widget stored in the optima_ui_widgets registry.
type UIWidget struct {
	WidgetID         string    `json:"widget_id"`
	DashboardSection string    `json:"dashboard_section"`
	Title            string    `json:"title"`
	ChartType        string    `json:"chart_type"` // line, gauge, grid, doughnut, bar
	CurrentSQL       string    `json:"current_sql,omitempty"`
	DefaultSQL       string    `json:"default_sql,omitempty"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// UIWidgetPublic is the non-admin view (excludes raw SQL).
type UIWidgetPublic struct {
	WidgetID         string    `json:"widget_id"`
	DashboardSection string    `json:"dashboard_section"`
	Title            string    `json:"title"`
	ChartType        string    `json:"chart_type"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// WidgetQueryRequest is the body for POST /api/dashboard/query/execute.
type WidgetQueryRequest struct {
	WidgetID   string            `json:"widget_id"`
	Parameters map[string]string `json:"parameters"` // e.g. {"server_name": "PG-01", "database": "mydb"}
}

// WidgetUpdateRequest is the body for PUT /api/admin/widgets/{id}.
type WidgetUpdateRequest struct {
	CurrentSQL string `json:"current_sql"`
}
