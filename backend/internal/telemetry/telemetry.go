package telemetry

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

var tracingEnabled atomic.Bool

var (
	HTTPRequests = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "sql_optima_http_requests_total",
			Help: "Total HTTP requests",
		},
		[]string{"method", "path", "code"},
	)
	HTTPDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "sql_optima_http_request_duration_seconds",
			Help:    "HTTP request duration",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)
)

// MetricsHandler serves Prometheus metrics.
func MetricsHandler() http.Handler {
	return promhttp.Handler()
}

// PrometheusMiddleware records request counts and latency (path label normalized to route template when possible).
func PrometheusMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, code: http.StatusOK}
		next.ServeHTTP(sw, r)
		path := r.URL.Path
		HTTPRequests.WithLabelValues(r.Method, path, strconv.Itoa(sw.code)).Inc()
		HTTPDuration.WithLabelValues(r.Method, path).Observe(time.Since(start).Seconds())
	})
}

type statusWriter struct {
	http.ResponseWriter
	code int
}

func (s *statusWriter) WriteHeader(code int) {
	s.code = code
	s.ResponseWriter.WriteHeader(code)
}

// InitTracer configures OTLP HTTP tracing when OTEL_EXPORTER_OTLP_ENDPOINT is set.
func InitTracer(ctx context.Context, serviceName string) (shutdown func(context.Context) error, err error) {
	ep := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
	if ep == "" {
		return func(context.Context) error { return nil }, nil
	}

	exp, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, err
	}
	res, err := resource.New(ctx,
		resource.WithAttributes(semconv.ServiceName(serviceName)),
	)
	if err != nil {
		return nil, err
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{}))
	tracingEnabled.Store(true)
	slog.Info("OpenTelemetry tracing enabled", "endpoint", ep)
	return tp.Shutdown, nil
}

// WrapOTelHTTP optionally wraps the handler with otelhttp when tracing was initialized.
func WrapOTelHTTP(h http.Handler) http.Handler {
	if !tracingEnabled.Load() {
		return h
	}
	return otelhttp.NewHandler(h, "sql-optima")
}
