package hot

import (
	"context"
	"log"
	"time"
)

func (tl *TimescaleLogger) LogSQLServerJobDetails(ctx context.Context, instanceName string, jobs []map[string]interface{}) error {
	if len(jobs) == 0 {
		return nil
	}

	timestamp := time.Now().UTC()
	insertCount := 0

	tl.mu.Lock()
	defer tl.mu.Unlock()

	for _, job := range jobs {
		jobName := getStr(job, "job_name")
		enabled := getBool(job, "enabled")
		owner := getStr(job, "owner")
		createdDate := getStr(job, "created_date")
		currentStatus := getStr(job, "current_status")
		lastRunDate := getStr(job, "last_run_date")
		lastRunTime := getStr(job, "last_run_time")
		lastRunStatus := getStr(job, "last_run_status")

		prevJob := tl.prevJobDetails[jobName]
		shouldInsert := true
		if prevJob != nil {
			if getBool(prevJob, "enabled") == enabled &&
				getStr(prevJob, "owner") == owner &&
				getStr(prevJob, "current_status") == currentStatus &&
				getStr(prevJob, "last_run_date") == lastRunDate &&
				getStr(prevJob, "last_run_time") == lastRunTime &&
				getStr(prevJob, "last_run_status") == lastRunStatus {
				shouldInsert = false
			}
		}

		if shouldInsert {
			_, err := tl.pool.Exec(ctx, `
				INSERT INTO sqlserver_job_details (capture_timestamp, server_instance_name, job_name, job_enabled, job_owner, created_date, current_status, last_run_date, last_run_time, last_run_status)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
				timestamp, instanceName,
				jobName, enabled, owner, createdDate,
				currentStatus, lastRunDate, lastRunTime, lastRunStatus)
			if err != nil {
				return err
			}
			insertCount++
		}

		tl.prevJobDetails[jobName] = map[string]interface{}{
			"enabled":         enabled,
			"owner":           owner,
			"current_status":  currentStatus,
			"last_run_date":   lastRunDate,
			"last_run_time":   lastRunTime,
			"last_run_status": lastRunStatus,
		}
	}

	log.Printf("[TSLogger] LogSQLServerJobDetails: inserted %d rows for %s", insertCount, instanceName)
	return nil
}

func (tl *TimescaleLogger) LogSQLServerJobSchedules(ctx context.Context, instanceName string, schedules []map[string]interface{}) error {
	if len(schedules) == 0 {
		return nil
	}
	tx, err := tl.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	timestamp := time.Now().UTC()
	for _, sched := range schedules {
		_, err := tx.Exec(ctx, `
			INSERT INTO sqlserver_agent_schedules (capture_timestamp, server_instance_name, job_name, job_enabled, schedule_name, status)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			timestamp, instanceName,
			getStr(sched, "job_name"), getBool(sched, "job_enabled"), getStr(sched, "schedule_name"), getStr(sched, "status"))
		if err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func (tl *TimescaleLogger) LogSQLServerJobFailures(ctx context.Context, instanceName string, failures []map[string]interface{}) error {
	if len(failures) == 0 {
		return nil
	}

	timestamp := time.Now().UTC()
	insertCount := 0

	tl.mu.Lock()
	defer tl.mu.Unlock()

	for _, fail := range failures {
		jobName := getStr(fail, "job_name")
		stepName := getStr(fail, "step_name")
		message := getStr(fail, "message")
		runDate := getStr(fail, "run_date")
		runTime := getStr(fail, "run_time")

		failKey := jobName + "||" + stepName + "||" + runDate + "||" + runTime

		prevMsg, exists := tl.prevJobFailures[failKey]
		shouldInsert := true
		if exists && prevMsg == message {
			shouldInsert = false
		}

		if shouldInsert {
			_, err := tl.pool.Exec(ctx, `
				INSERT INTO sqlserver_job_failures (capture_timestamp, server_instance_name, job_name, step_name, error_message, run_date, run_time)
				VALUES ($1, $2, $3, $4, $5, $6, $7)`,
				timestamp, instanceName,
				jobName, stepName, message, runDate, runTime)
			if err != nil {
				return err
			}
			insertCount++
		}

		tl.prevJobFailures[failKey] = message
	}

	log.Printf("[TSLogger] LogSQLServerJobFailures: inserted %d rows for %s", insertCount, instanceName)
	return nil
}

func (tl *TimescaleLogger) LogSQLServerJobMetrics(ctx context.Context, instanceName string, jobMetrics map[string]interface{}) error {
	timestamp := time.Now().UTC()

	_, err := tl.pool.Exec(ctx, `
		INSERT INTO sqlserver_job_metrics (capture_timestamp, server_instance_name, total_jobs, enabled_jobs, disabled_jobs, running_jobs, failed_jobs_24h, error_message)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		timestamp, instanceName,
		getInt(jobMetrics, "total_jobs"),
		getInt(jobMetrics, "enabled_jobs"),
		getInt(jobMetrics, "disabled_jobs"),
		getInt(jobMetrics, "running_jobs"),
		getInt(jobMetrics, "failed_jobs_24h"),
		getStr(jobMetrics, "error_message"))
	return err
}

func (tl *TimescaleLogger) GetSQLServerJobMetrics(ctx context.Context, instanceName string, limit int) ([]map[string]interface{}, error) {
	if limit <= 0 {
		limit = 100
	}

	query := `
		SELECT capture_timestamp, total_jobs, enabled_jobs, disabled_jobs, running_jobs, failed_jobs_24h, error_message
		FROM sqlserver_job_metrics
		WHERE server_instance_name = $1
		ORDER BY capture_timestamp DESC
		LIMIT $2
	`

	rows, err := tl.pool.Query(ctx, query, instanceName, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var ts time.Time
		var totalJobs, enabledJobs, disabledJobs, runningJobs, failedJobs int
		var errorMsg string

		if err := rows.Scan(&ts, &totalJobs, &enabledJobs, &disabledJobs, &runningJobs, &failedJobs, &errorMsg); err != nil {
			continue
		}

		results = append(results, map[string]interface{}{
			"timestamp":       ts,
			"total_jobs":      totalJobs,
			"enabled_jobs":    enabledJobs,
			"disabled_jobs":   disabledJobs,
			"running_jobs":    runningJobs,
			"failed_jobs_24h": failedJobs,
			"error_message":   errorMsg,
		})
	}
	return results, rows.Err()
}
