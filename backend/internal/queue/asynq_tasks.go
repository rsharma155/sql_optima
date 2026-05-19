// Package queue implements background task processing logic.
package queue

import (
	"log/slog"
	"context"

	"github.com/hibiken/asynq"
	"github.com/rsharma155/sql_optima/internal/service"
)

// Task names
const (
	TypeHistorical = "metrics:historical"
)

// HandleHistoricalCollectionTask returns a handler for historical collection tasks.
func HandleHistoricalCollectionTask(svc *service.MetricsService) asynq.HandlerFunc {
	return func(ctx context.Context, t *asynq.Task) error {
		slog.Info("[Queue] Starting background historical collection...")
		svc.TriggerBackgroundCollectorsOnce()
		return nil
	}
}

// RegisterHandlers registers all task handlers with the asynq server.
func RegisterHandlers(mux *asynq.ServeMux, svc *service.MetricsService) {
	mux.HandleFunc(TypeHistorical, HandleHistoricalCollectionTask(svc))
}

// NewHistoricalCollectionTask creates a new background metrics task.
func NewHistoricalCollectionTask() (*asynq.Task, error) {
	return asynq.NewTask(TypeHistorical, nil), nil
}
