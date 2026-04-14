// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Redis-backed Asynq queue for scheduling collector tasks with Redis scheduler integration.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package queue

import (
	"log"

	"github.com/hibiken/asynq"
	"github.com/rsharma155/sql_optima/internal/service"
)

// StartScheduler enqueues live/historical collector tasks (runs in the API process when using Redis).
func StartScheduler(redisAddr string) (*asynq.Scheduler, error) {
	opt := asynq.RedisClientOpt{Addr: redisAddr}
	sch := asynq.NewScheduler(opt, &asynq.SchedulerOpts{})
	// Live/RTD collector is intentionally NOT scheduled.
	// Live endpoints should run only when the user opens the RTD page (frontend polling),
	// plus one optional warm-up scrape on API startup.
	if _, err := sch.Register("@every 1m", asynq.NewTask(TypeHistorical, nil)); err != nil {
		return nil, err
	}
	go func() {
		if err := sch.Run(); err != nil {
			log.Printf("[asynq] scheduler stopped: %v", err)
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
