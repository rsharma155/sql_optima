package enrichment

import (
	"encoding/binary"
	"github.com/rsharma155/sql_optima/internal/collectors/domain"
)

func EnrichMSSQL(snapshots []domain.MSSQLQuerySnapshot, enrichments []domain.MSSQLSessionEnrichment) []domain.MSSQLCombinedMetric {
	enrichMap := make(map[string]domain.MSSQLSessionEnrichment)
	for _, e := range enrichments {
		enrichMap[string(e.PlanHandle)] = e
	}

	var results []domain.MSSQLCombinedMetric
	for _, s := range snapshots {
		metric := domain.MSSQLCombinedMetric{
			Timestamp:          s.LastExecutionTime,
			DatabaseName:       s.DatabaseName,
			TotalExecutions:    s.TotalExecutions,
			TotalCPUMs:         s.TotalCPUMs,
			TotalElapsedMs:     s.TotalElapsedMs,
			TotalLogicalReads:  s.TotalLogicalReads,
			TotalPhysicalReads: s.TotalPhysicalReads,
			TotalRows:          s.TotalRows,
			StatementText:      s.StatementText,
			QueryTextRaw:       s.QueryTextRaw,
			QueryHash:          bytesToInt64(s.QueryHash),
			PlanHash:           bytesToInt64(s.QueryPlanHash),
			LastExecutionTime:  s.LastExecutionTime,
			IsUserWorkload:     1, // Default to user workload if no enrichment found
		}

		if e, ok := enrichMap[string(s.PlanHandle)]; ok {
			metric.LoginName = e.LoginName
			metric.ApplicationName = e.ApplicationName
			metric.IsUserWorkload = e.IsUserWorkload
			if metric.DatabaseName == "" {
				metric.DatabaseName = e.DatabaseName
			}
		}

		results = append(results, metric)
	}
	return results
}

func EnrichPG(snapshots []domain.PGQuerySnapshot, enrichments []domain.PGActivityEnrichment) []domain.PGCombinedMetric {
	enrichMap := make(map[int64]domain.PGActivityEnrichment)
	for _, e := range enrichments {
		if e.QueryID != 0 {
			enrichMap[e.QueryID] = e
		}
	}

	var results []domain.PGCombinedMetric
	for _, s := range snapshots {
		metric := domain.PGCombinedMetric{
			Timestamp:       s.CaptureTime,
			QueryID:         s.QueryID,
			Query:           s.Query,
			Calls:           s.Calls,
			TotalExecTime:   s.TotalExecTime,
			Rows:            s.Rows,
			SharedBlksHit:   s.SharedBlksHit,
			SharedBlksRead:  s.SharedBlksRead,
			TempBlksWritten: s.TempBlksWritten,
		}

		if e, ok := enrichMap[s.QueryID]; ok {
			metric.UserName = e.UserName
			metric.DatabaseName = e.DatabaseName
			metric.ApplicationName = e.ApplicationName
		}

		results = append(results, metric)
	}
	return results
}

func bytesToInt64(b []byte) int64 {
	if len(b) < 8 {
		return 0
	}
	return int64(binary.BigEndian.Uint64(b))
}
