// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Redis-backed Asynq queue for scheduling collector tasks with Redis scheduler integration.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package queue

import (
	"log/slog"
	"context"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
	"github.com/rsharma155/sql_optima/internal/service"
)

// StartScheduler enqueues live/historical collector tasks (runs in the API process when using Redis).
func StartScheduler(redisAddr string, svc *service.MetricsService) (*asynq.Scheduler, error) {
	opt := asynq.RedisClientOpt{Addr: redisAddr}
	sch := asynq.NewScheduler(opt, &asynq.SchedulerOpts{})
	
	// Live/RTD collector is intentionally NOT scheduled.
	// Live endpoints should run only when the user opens the RTD page (frontend polling),
	// plus one optional warm-up scrape on API startup.
	
	ctx := context.Background()
	interval := svc.GetCollectorInterval(ctx, "Asynq Historical", 1*time.Minute)
	
	// Schedule with initial interval
	entryID, err := sch.Register(fmt.Sprintf("@every %s", interval), asynq.NewTask(TypeHistorical, nil))
	if err != nil {
		return nil, err
	}

	go func() {
		// Periodically check for frequency changes in a separate loop since asynq.Scheduler doesn't support easy dynamic rescheduling
		// For now, we'll just log and let the initial schedule run. 
		// Real dynamic rescheduling with asynq usually requires Unregister/Register.
		
		go func() {
			ticker := time.NewTicker(5 * time.Minute)
			for range ticker.C {
				newInterval := svc.GetCollectorInterval(ctx, "Asynq Historical", 1*time.Minute)
				if newInterval != interval && newInterval > 0 {
					slog.Info("[asynq] Frequency changed from", "arg1", interval, "arg2", newInterval)
					_ = sch.Unregister(entryID)
					newEntryID, err := sch.Register(fmt.Sprintf("@every %s", newInterval), asynq.NewTask(TypeHistorical, nil))
					if err == nil {
						entryID = newEntryID
						interval = newInterval
					}
				}
			}
		}()

		if err := sch.Run(); err != nil {
			slog.Info("[asynq] scheduler stopped", "err", err)
		}
	}()
	return sch, nil
}

// NewServerWithMux builds an Asynq server and mux with collector handlers.
func NewServerWithMux(redisAddr string, svc *service.MetricsService) (*asynq.Server, *asynq.ServeMux) {
	mux := asynq.NewServeMux()
	RegisterHandlers(mux, svc)
	srv := asynq.NewServer(
		asynq.RedisClientOpt{Addr: redisAddr},
		asynq.Config{Concurrency: 4},
	)
	return srv, mux
}
