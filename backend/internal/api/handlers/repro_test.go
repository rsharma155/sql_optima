package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rsharma155/sql_optima/internal/service"
	"github.com/rsharma155/sql_optima/internal/storage/hot"
)

func TestBlockingDetails_Reproduction(t *testing.T) {
	// Setup a real but "empty" MetricsService and TimescaleLogger
	// We want to see if it panics or returns 500 when the pool is nil

	// Create a HotStorage with nil pool
	tsStorage := &hot.HotStorage{}

	svc := service.NewMetricsService(nil, nil, nil, tsStorage)
	h := NewSqlServerHandlers(svc, nil)

	instance := "Local SQL Server"
	from := "2026-05-03T00:31:00.000Z"
	to := "2026-05-03T01:31:00.000Z"

	req, _ := http.NewRequest("GET", "/api/sqlserver/blocking/details?instance="+instance+"&from="+from+"&to="+to, nil)
	rr := httptest.NewRecorder()

	// Use a defer to catch potential panic
	defer func() {
		if r := recover(); r != nil {
			t.Logf("Handler panicked as expected: %v", r)
		}
	}()

	h.BlockingDetails(rr, req)

	if rr.Code == http.StatusInternalServerError {
		t.Logf("Reproduced 500 Internal Server Error: %s", rr.Body.String())
	} else if rr.Code != http.StatusOK {
		t.Errorf("Handler returned unexpected status: %v", rr.Code)
	}
}
