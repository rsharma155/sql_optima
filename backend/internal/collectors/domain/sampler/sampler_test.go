package sampler

import (
	"github.com/rsharma155/sql_optima/internal/collectors/domain"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestSampleMSSQL_Correctness(t *testing.T) {
	metrics := []domain.MSSQLCombinedMetric{
		{QueryHash: 1, TotalCPUMs: 100},
		{QueryHash: 2, TotalCPUMs: 200},
		{QueryHash: 3, TotalCPUMs: 300},
	}

	sampled := SampleMSSQL(metrics, 2)
	assert.Len(t, sampled, 3) // It should keep all if total < 2*3? Wait, my logic might be slightly different.
	// Actually, it takes top 2 by CPU, top 2 by Reads, top 2 by Duration.

	metrics = make([]domain.MSSQLCombinedMetric, 100)
	for i := 0; i < 100; i++ {
		metrics[i] = domain.MSSQLCombinedMetric{
			QueryHash:         int64(i),
			TotalCPUMs:        int64(i),
			TotalLogicalReads: int64(i),
			TotalElapsedMs:    int64(i),
		}
	}

	sampled = SampleMSSQL(metrics, 10)
	// Top 10 by CPU (90-99), Top 10 by Reads (90-99), Top 10 by Duration (90-99).
	// They are the same, so union should be 10.
	assert.Len(t, sampled, 10)
}
