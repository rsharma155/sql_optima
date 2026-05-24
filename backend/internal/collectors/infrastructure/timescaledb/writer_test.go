package timescaledb

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTimescaleWriter_WriteMSSQLMetrics_Empty(t *testing.T) {
	writer := &TimescaleWriter{}
	err := writer.WriteMSSQLMetrics(context.Background(), nil)
	assert.NoError(t, err)
}

func TestTimescaleWriter_WritePGMetrics_Empty(t *testing.T) {
	writer := &TimescaleWriter{}
	err := writer.WritePGMetrics(context.Background(), nil)
	assert.NoError(t, err)
}

// Note: Testing actual batch execution requires a complex mock for pgxpool.Pool 
// and pgx.Batch. pgxmock/v3 supports pgxpool but batching is often tricky.
// We will focus on the empty cases and basic structure for now to establish coverage.
