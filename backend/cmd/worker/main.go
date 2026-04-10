// Worker process: runs Asynq consumers for Redis-scheduled collector tasks.
// Requires REDIS_ADDR. Run the API with the same REDIS_ADDR to enqueue via the embedded scheduler, or use this binary alone if another process schedules tasks.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/queue"
	"github.com/rsharma155/sql_optima/internal/repository"
	"github.com/rsharma155/sql_optima/internal/service"
	"github.com/rsharma155/sql_optima/internal/storage/hot"
)

func main() {
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		log.Fatal("REDIS_ADDR is required for cmd/worker")
	}

	configPath, _, _ := config.ResolveDataPaths()
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	pgRepo := repository.NewPgRepository(cfg)
	msRepo := repository.NewMssqlRepository(cfg)
	tsHotStorage, err := hot.New(nil)
	if err != nil {
		log.Printf("Timescale optional: %v", err)
		tsHotStorage = nil
	}
	metricsSvc := service.NewMetricsService(pgRepo, msRepo, cfg, tsHotStorage)

	srv, mux := queue.NewServerWithMux(redisAddr, metricsSvc)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		srv.Shutdown()
	}()

	log.Printf("Asynq worker listening on Redis %s", redisAddr)
	if err := srv.Run(mux); err != nil {
		log.Fatalf("worker: %v", err)
	}
}
