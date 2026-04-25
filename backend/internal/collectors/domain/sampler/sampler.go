package sampler

import (
	"fmt"
	"github.com/rsharma155/sql_optima/internal/collectors/domain"
	"sort"
)

func SampleMSSQL(metrics []domain.MSSQLCombinedMetric, n int) []domain.MSSQLCombinedMetric {
	if len(metrics) <= n {
		return metrics
	}

	byCPU := make([]domain.MSSQLCombinedMetric, len(metrics))
	copy(byCPU, metrics)
	sort.Slice(byCPU, func(i, j int) bool {
		return byCPU[i].TotalCPUMs > byCPU[j].TotalCPUMs
	})

	byReads := make([]domain.MSSQLCombinedMetric, len(metrics))
	copy(byReads, metrics)
	sort.Slice(byReads, func(i, j int) bool {
		return byReads[i].TotalLogicalReads > byReads[j].TotalLogicalReads
	})

	byDuration := make([]domain.MSSQLCombinedMetric, len(metrics))
	copy(byDuration, metrics)
	sort.Slice(byDuration, func(i, j int) bool {
		return byDuration[i].TotalElapsedMs > byDuration[j].TotalElapsedMs
	})

	seen := make(map[string]bool)
	var sampled []domain.MSSQLCombinedMetric

	limit := n
	if len(metrics) < n {
		limit = len(metrics)
	}

	add := func(m domain.MSSQLCombinedMetric) {
		key := fmt.Sprintf("%d-%d-%s-%s-%s", m.QueryHash, m.PlanHash, m.DatabaseName, m.LoginName, m.ApplicationName)
		if !seen[key] {
			seen[key] = true
			sampled = append(sampled, m)
		}
	}

	for i := 0; i < limit; i++ {
		add(byCPU[i])
		add(byReads[i])
		add(byDuration[i])
	}

	return sampled
}

func SamplePG(metrics []domain.PGCombinedMetric, n int) []domain.PGCombinedMetric {
	if len(metrics) <= n {
		return metrics
	}

	byTime := make([]domain.PGCombinedMetric, len(metrics))
	copy(byTime, metrics)
	sort.Slice(byTime, func(i, j int) bool {
		return byTime[i].TotalExecTime > byTime[j].TotalExecTime
	})

	byCalls := make([]domain.PGCombinedMetric, len(metrics))
	copy(byCalls, metrics)
	sort.Slice(byCalls, func(i, j int) bool {
		return byCalls[i].Calls > byCalls[j].Calls
	})

	byReads := make([]domain.PGCombinedMetric, len(metrics))
	copy(byReads, metrics)
	sort.Slice(byReads, func(i, j int) bool {
		return byReads[i].SharedBlksRead > byReads[j].SharedBlksRead
	})

	seen := make(map[int64]bool)
	var sampled []domain.PGCombinedMetric

	limit := n
	if len(metrics) < n {
		limit = len(metrics)
	}

	add := func(m domain.PGCombinedMetric) {
		if !seen[m.QueryID] {
			seen[m.QueryID] = true
			sampled = append(sampled, m)
		}
	}

	for i := 0; i < limit; i++ {
		add(byTime[i])
		add(byCalls[i])
		add(byReads[i])
	}

	return sampled
}
