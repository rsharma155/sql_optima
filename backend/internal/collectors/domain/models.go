package domain

import (
	"time"

	"github.com/google/uuid"
)

// MSSQLQuerySnapshot represents a row from sys.dm_exec_query_stats
type MSSQLQuerySnapshot struct {
	DBID               int32
	DatabaseName       string
	TotalExecutions    int64
	LastExecutionTime  time.Time
	TotalCPUMs         int64
	TotalElapsedMs     int64
	TotalLogicalReads  int64
	TotalLogicalWrites int64
	TotalPhysicalReads int64
	TotalRows          int64
	TotalGrantKB       int64
	MaxWorkerTime      int64
	MaxLogicalReads    int64
	MaxDOP             int32
	MaxGrantKB         int64
	MaxRows            int64
	StatementText      string
	QueryTextRaw       string
	QueryPlanHash      []byte
	QueryHash          []byte
	PlanHandle         []byte
}

// MSSQLSessionEnrichment represents session metadata for joining
type MSSQLSessionEnrichment struct {
	PlanHandle      []byte
	LoginName       string
	ApplicationName string
	DatabaseName    string
	IsUserWorkload  int
}

// PGQuerySnapshot represents a row from pg_stat_statements
type PGQuerySnapshot struct {
	CaptureTime     time.Time
	UserID          uint32
	DBID            uint32
	QueryID         int64
	Query           string
	Calls           int64
	TotalExecTime   float64
	Rows            int64
	SharedBlksHit   int64
	SharedBlksRead  int64
	TempBlksWritten int64
}

// PGActivityEnrichment represents data from pg_stat_activity
type PGActivityEnrichment struct {
	CaptureTime     time.Time
	PID             int32
	QueryID         int64
	UserName        string
	DatabaseName    string
	ApplicationName string
	ClientAddr      string
	State           string
}

// MSSQLCombinedMetric is the enriched and ready-to-store metric
type MSSQLCombinedMetric struct {
	Timestamp          time.Time
	ServerID           uuid.UUID
	DatabaseName       string
	LoginName          string
	ApplicationName    string
	QueryHash          int64
	PlanHash           int64
	TotalExecutions    int64
	TotalCPUMs         int64
	TotalElapsedMs     int64
	TotalLogicalReads  int64
	TotalPhysicalReads int64
	TotalRows          int64
	StatementText      string
	QueryTextRaw       string
	LastExecutionTime  time.Time
	IsUserWorkload     int
}

// PGCombinedMetric is the enriched and ready-to-store metric
type PGCombinedMetric struct {
	Timestamp       time.Time
	ServerID        uuid.UUID
	DatabaseName    string
	UserName        string
	ApplicationName string
	QueryID         int64
	Query           string
	Calls           int64
	TotalExecTime   float64
	Rows            int64
	SharedBlksHit   int64
	SharedBlksRead  int64
	TempBlksWritten int64
}
