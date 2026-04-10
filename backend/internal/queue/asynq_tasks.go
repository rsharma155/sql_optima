package queue

import (
	"context"

	"github.com/hibiken/asynq"
	"github.com/rsharma155/sql_optima/internal/service"
)

const (
	TypeLive       = "optima:collector:live"
	TypeHistorical = "optima:collector:historical"
)

// RegisterHandlers wires core collector ticks for the Asynq worker.
func RegisterHandlers(mux *asynq.ServeMux, svc *service.MetricsService) {
	mux.HandleFunc(TypeLive, func(ctx context.Context, t *asynq.Task) error {
		svc.RunLiveCollectorOnce(ctx)
		return nil
	})
	mux.HandleFunc(TypeHistorical, func(ctx context.Context, t *asynq.Task) error {
		svc.RunHistoricalCollectorOnce(ctx)
		return nil
	})
}
