// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: SQL Server Agent job monitoring with status, schedules, and failure tracking.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"log/slog"
	"context"
	"strings"
	"time"

	"github.com/rsharma155/sql_optima/internal/models"
)

func (c *SqlServerRepository) FetchAgentJobs(ctx context.Context, instanceName string) models.JobMetrics {
	var metrics models.JobMetrics
	metrics.InstanceName = instanceName
	metrics.Timestamp = time.Now().Format("2006-01-02 15:04:05")

	c.mutex.RLock()
	db, ok := c.conns[strings.ToUpper(instanceName)]
	c.mutex.RUnlock()
	if !ok || db == nil {
		slog.Error("[SQLSERVER] Agent Jobs Engine Ping Failed: Database pointer absolutely nil for", "val", instanceName)
		return metrics
	}

	if err := db.Ping(); err != nil {
		slog.Info("[SQLSERVER] Agent Jobs Connection Loop broken on", "target", instanceName, "err", err)
		return metrics
	}

	// 1. Jobs Overview (Summary)
	summaryQuery := `
		SELECT /* SQL_OPTIMA */   
			COUNT(*) AS TotalJobs,
			SUM(CASE WHEN enabled = 1 THEN 1 ELSE 0 END) AS EnabledJobs,
			SUM(CASE WHEN enabled = 0 THEN 1 ELSE 0 END) AS DisabledJobs
		FROM msdb.dbo.sysjobs WITH (NOLOCK)
	`
	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	if err := db.QueryRowContext(ctx, summaryQuery).Scan(&metrics.Summary.TotalJobs, &metrics.Summary.EnabledJobs, &metrics.Summary.DisabledJobs); err != nil {
		slog.Error("[SQLSERVER] FetchAgentJobs: Failed to fetch summary", "target", instanceName, "err", err)
		metrics.Summary.TotalJobs = -1 // Indicate error state
		metrics.LastError = "failed to fetch job summary"
		return metrics
	}

	// Pull running jobs explicit checks
	runningQuery := `SELECT /* SQL_OPTIMA */   COUNT(*) FROM msdb.dbo.sysjobactivity WITH (NOLOCK) WHERE start_execution_date IS NOT NULL AND stop_execution_date IS NULL`
	ctx, cancel = WithQueryTimeout(ctx, 0)
	defer cancel()
	if err := db.QueryRowContext(ctx, runningQuery).Scan(&metrics.Summary.RunningJobs); err != nil {
		slog.Error("[SQLSERVER] FetchAgentJobs: Failed to fetch running jobs", "target", instanceName, "err", err)
		metrics.LastError = "failed to fetch running jobs"
		return metrics
	}

	// Failed in last 24h
	failedQuery := `
		/* SQL_OPTIMA */ SELECT   COUNT(*) FROM msdb.dbo.sysjobhistory h WITH (NOLOCK)
		JOIN msdb.dbo.sysjobs j WITH (NOLOCK) ON h.job_id = j.job_id
		WHERE h.run_status = 0 AND h.run_date >= CAST(CONVERT(VARCHAR(8), GETUTCDATE()-1, 112) AS INT) AND h.step_id = 0
	`
	ctx, cancel = WithQueryTimeout(ctx, 0)
	defer cancel()
	if err := db.QueryRowContext(ctx, failedQuery).Scan(&metrics.Summary.FailedJobs); err != nil {
		slog.Error("[SQLSERVER] FetchAgentJobs: Failed to fetch failed jobs", "target", instanceName, "err", err)
		metrics.LastError = "failed to fetch failed jobs"
		return metrics
	}

	// 2. Job List
	listQuery := `
		/* SQL_OPTIMA */ SELECT   
			ISNULL(j.name, 'Unknown') AS JobName,
			ISNULL(c.name, 'Uncategorized') AS Category,
			ISNULL(j.description, '') AS Description,
			CAST(j.enabled AS BIT),
			ISNULL(SUSER_SNAME(j.owner_sid), 'Unknown') AS Owner,
			ISNULL(CONVERT(VARCHAR, j.date_created, 120), '') AS date_created,
			CASE 
				WHEN ja.start_execution_date IS NOT NULL AND ja.stop_execution_date IS NULL THEN 'Running'
				ELSE 'Idle'
			END AS CurrentStatus,
			ISNULL(h.run_date, 0),
			ISNULL(h.run_time, 0),
			CASE ISNULL(h.run_status, -1)
				WHEN 0 THEN 'Failed'
				WHEN 1 THEN 'Succeeded'
				WHEN 2 THEN 'Retry'
				WHEN 3 THEN 'Canceled'
				ELSE 'Unknown'
			END AS LastRunStatus
		FROM msdb.dbo.sysjobs j WITH (NOLOCK)
		LEFT JOIN msdb.dbo.syscategories c WITH (NOLOCK) ON j.category_id = c.category_id
		LEFT JOIN (
			SELECT /* SQL_OPTIMA */   job_id, MAX(session_id) as session_id FROM msdb.dbo.sysjobactivity WITH (NOLOCK) GROUP BY job_id
		) max_ja ON j.job_id = max_ja.job_id
		LEFT JOIN msdb.dbo.sysjobactivity ja WITH (NOLOCK) ON max_ja.job_id = ja.job_id AND max_ja.session_id = ja.session_id
		LEFT JOIN (
			SELECT /* SQL_OPTIMA */   job_id, MAX(instance_id) AS instance_id FROM msdb.dbo.sysjobhistory WITH (NOLOCK) WHERE step_id = 0 GROUP BY job_id
		) max_h ON j.job_id = max_h.job_id
		LEFT JOIN msdb.dbo.sysjobhistory h WITH (NOLOCK) ON max_h.instance_id = h.instance_id
	`
	ctx, cancel = WithQueryTimeout(ctx, 0)
	defer cancel()
	listRows, errL := db.QueryContext(ctx, listQuery)
	if errL == nil {
		defer listRows.Close()
		for listRows.Next() {
			var j models.JobDetail
			if err := listRows.Scan(&j.JobName, &j.Category, &j.Description, &j.Enabled, &j.Owner, &j.CreatedDate, &j.CurrentStatus, &j.LastRunDate, &j.LastRunTime, &j.LastRunStatus); err == nil {
				metrics.Jobs = append(metrics.Jobs, j)
			}
		}
	}

	// 3. Next Run Schedules
	schedQuery := `
		/* SQL_OPTIMA */ SELECT   
			ISNULL(j.name, 'Unknown') AS JobName,
			ISNULL(j.enabled, 0),
			ISNULL(s.name, 'N/A') AS ScheduleName,
			ISNULL(s.enabled, 0),
			CONVERT(VARCHAR, msdb.dbo.agent_datetime(js.next_run_date, js.next_run_time), 120) AS NextRunDateTime
		FROM msdb.dbo.sysjobs j WITH (NOLOCK)
		JOIN msdb.dbo.sysjobschedules js WITH (NOLOCK) ON j.job_id = js.job_id
		JOIN msdb.dbo.sysschedules s WITH (NOLOCK) ON js.schedule_id = s.schedule_id
	`
	ctx, cancel = WithQueryTimeout(ctx, 0)
	defer cancel()
	schedRows, errS := db.QueryContext(ctx, schedQuery)
	if errS == nil {
		defer schedRows.Close()
		for schedRows.Next() {
			var s models.JobSchedule
			var enabled, schedEnabled int
			var nextRun *string
			if err := schedRows.Scan(&s.JobName, &enabled, &s.ScheduleName, &schedEnabled, &nextRun); err == nil {
				s.JobEnabled = enabled == 1
				s.Status = map[bool]string{true: "Active", false: "Disabled"}[schedEnabled == 1]
				s.NextRunDateTime = nextRun
				metrics.Schedules = append(metrics.Schedules, s)
			}
		}
	}

	// 4. Job Failures (Last 100 to limit payload sizes natively)
	failQuery := `
		/* SQL_OPTIMA */ SELECT   TOP 100
			ISNULL(j.name, 'Unknown'),
			ISNULL(h.step_name, 'Unknown'),
			ISNULL(SUBSTRING(h.message, 1, 300), 'No Trace'),
			ISNULL(h.run_date, 0),
			ISNULL(h.run_time, 0)
		FROM msdb.dbo.sysjobhistory h WITH (NOLOCK)
		JOIN msdb.dbo.sysjobs j WITH (NOLOCK) ON h.job_id = j.job_id
		WHERE h.run_status = 0 AND h.step_id > 0
		ORDER BY h.run_date DESC, h.run_time DESC
	`
	ctx, cancel = WithQueryTimeout(ctx, 0)
	defer cancel()
	failRows, errF := db.QueryContext(ctx, failQuery)
	if errF == nil {
		defer failRows.Close()
		for failRows.Next() {
			var f models.JobFailure
			if err := failRows.Scan(&f.JobName, &f.StepName, &f.Message, &f.RunDate, &f.RunTime); err == nil {
				metrics.Failures = append(metrics.Failures, f)
			}
		}
	}

	return metrics
}
