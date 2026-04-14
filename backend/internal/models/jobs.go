// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: SQL Server Agent job models for job status, schedules, failures, and execution history.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package models

type JobSummary struct {
	TotalJobs    int `json:"total_jobs"`
	EnabledJobs  int `json:"enabled_jobs"`
	DisabledJobs int `json:"disabled_jobs"`
	FailedJobs   int `json:"failed_jobs"`
	RunningJobs  int `json:"running_jobs"`
}

type JobDetail struct {
	JobName        string `json:"job_name"`
	Enabled        bool   `json:"enabled"`
	Owner          string `json:"owner"`
	CreatedDate    string `json:"created_date"`
	CurrentStatus  string `json:"current_status"`
	LastRunDate    int    `json:"last_run_date"`
	LastRunTime    int    `json:"last_run_time"`
	LastRunStatus  string `json:"last_run_status"`
}

type JobSchedule struct {
	JobName         string  `json:"job_name"`
	NextRunDateTime *string `json:"next_run_datetime"`
	JobEnabled      bool    `json:"job_enabled"`
	ScheduleName    string  `json:"schedule_name"`
	Status          string  `json:"status"`
}

type JobFailure struct {
	JobName  string `json:"job_name"`
	StepName string `json:"step_name"`
	Message  string `json:"message"`
	RunDate  int    `json:"run_date"`
	RunTime  int    `json:"run_time"`
}

type JobMetrics struct {
	InstanceName string        `json:"instance_name"`
	Timestamp    string        `json:"timestamp"`
	LastError    string        `json:"last_error,omitempty"` // Expose errors to UI
	Summary      JobSummary    `json:"summary"`
	Jobs         []JobDetail   `json:"jobs"`
	Schedules    []JobSchedule `json:"schedules"`
	Failures     []JobFailure  `json:"failures"`
}
