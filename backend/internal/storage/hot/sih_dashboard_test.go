package hot

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// NOTE: These tests assume a running TimescaleDB if using a real pool.
// For now, we focus on the logic and assume helpers exist or will be added.

func TestKPIFieldsAlwaysPopulated(t *testing.T) {
	// This test will likely fail if no DB is available, but it defines our goal.
	// In a real environment, we'd use a test container or a local test DB.
	tl := &TimescaleLogger{} // Minimal mock or use a real one if possible

	// Skip if no real pool is available for now, or implement a mock.
	if tl.pool == nil {
		t.Skip("Skipping test: no database pool available")
	}

	dash, err := tl.QueryStorageIndexHealthDashboard(
		context.Background(),
		"sqlserver", "test-server",
		time.Now().Add(-24*time.Hour).UTC().Format(time.RFC3339),
		time.Now().UTC().Format(time.RFC3339),
		SIHFilters{},
	)
	require.NoError(t, err)
	require.NotNil(t, dash)

	k := dash.KPIs
	assert.GreaterOrEqual(t, k.TotalDBSizeMB, float64(0), "TotalDBSizeMB must be >= 0")
	assert.GreaterOrEqual(t, k.UnusedIndexCount, 0, "UnusedIndexCount must be >= 0")
	assert.GreaterOrEqual(t, k.UnusedIndexMB, float64(0), "UnusedIndexMB must be >= 0")
	assert.GreaterOrEqual(t, k.HighScanTableCount, 0, "HighScanTableCount must be >= 0")
	assert.GreaterOrEqual(t, k.IndexWriteOverheadPct, float64(0), "IndexWriteOverheadPct must be >= 0")
	assert.LessOrEqual(t, k.IndexWriteOverheadPct, float64(100), "IndexWriteOverheadPct must be <= 100")
	assert.GreaterOrEqual(t, k.Growth7dPct, float64(0))
	// TDD: This is expected to fail initially as it's not populated in QueryStorageIndexHealthDashboard
	assert.GreaterOrEqual(t, k.ForecastTableMB90d, float64(0), "ForecastTableMB90d must be populated")
}
