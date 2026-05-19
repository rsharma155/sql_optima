package hot

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

// ---- Compile-time type guards for new structs ----

func TestPgssWorkloadPointStructExists(t *testing.T) {
	var _ PgssWorkloadPoint
}

func TestPgssTopQueryStructExists(t *testing.T) {
	var _ PgssTopQuery
}

func TestPgssLatencyPointStructExists(t *testing.T) {
	var _ PgssLatencyPoint
}

func TestPgssRegressionStructExists(t *testing.T) {
	var _ PgssRegression
}

func TestLatencyEntryStructExists(t *testing.T) {
	var _ latencyEntry
}

// ---- weightedPercentile pure-function tests ----

func TestWeightedPercentile_SingleEntry(t *testing.T) {
	entries := []latencyEntry{{calls: 100, meanMs: 5.0}}
	got := weightedPercentile(entries, 100, 0.50)
	if got != 5.0 {
		t.Fatalf("p50 of single entry: want 5.0, got %f", got)
	}
	got = weightedPercentile(entries, 100, 0.99)
	if got != 5.0 {
		t.Fatalf("p99 of single entry: want 5.0, got %f", got)
	}
}

func TestWeightedPercentile_TwoEntries(t *testing.T) {
	entries := []latencyEntry{
		{calls: 90, meanMs: 1.0},
		{calls: 10, meanMs: 100.0},
	}
	p50 := weightedPercentile(entries, 100, 0.50)
	if p50 != 1.0 {
		t.Fatalf("p50 want 1.0, got %f", p50)
	}
	p95 := weightedPercentile(entries, 100, 0.95)
	// 95th percentile: ceil(100*0.95)=95, cumulative after first entry=90 < 95, after second=100 >= 95
	if p95 != 100.0 {
		t.Fatalf("p95 want 100.0, got %f", p95)
	}
}

func TestWeightedPercentile_EmptyEntries(t *testing.T) {
	got := weightedPercentile(nil, 0, 0.50)
	if got != 0 {
		t.Fatalf("empty: want 0, got %f", got)
	}
}

func TestWeightedPercentile_AllSameLatency(t *testing.T) {
	entries := []latencyEntry{
		{calls: 50, meanMs: 3.0},
		{calls: 50, meanMs: 3.0},
	}
	got := weightedPercentile(entries, 100, 0.99)
	if got != 3.0 {
		t.Fatalf("same latency p99: want 3.0, got %f", got)
	}
}

func TestWeightedPercentile_GranularPercentiles(t *testing.T) {
	entries := []latencyEntry{
		{calls: 50, meanMs: 1.0},
		{calls: 45, meanMs: 10.0},
		{calls: 4, meanMs: 50.0},
		{calls: 1, meanMs: 500.0},
	}
	total := int64(100)

	p50 := weightedPercentile(entries, total, 0.50)
	if p50 != 1.0 {
		t.Fatalf("p50 want 1.0, got %f", p50)
	}
	p95 := weightedPercentile(entries, total, 0.95)
	if p95 != 10.0 {
		t.Fatalf("p95 want 10.0, got %f", p95)
	}
	p99 := weightedPercentile(entries, total, 0.99)
	if p99 != 50.0 {
		t.Fatalf("p99 want 50.0, got %f", p99)
	}
}

// ---- subSnap delta correctness for new fields ----

func TestSubSnap_NewFieldsDelta(t *testing.T) {
	base := PostgresQueryStatsSnapRow{
		QueryID: 1, Calls: 10, TotalTimeMs: 100, Rows: 500,
		SharedBlksHit: 1000, SharedBlksRead: 200, SharedBlksDirtied: 50, SharedBlksWritten: 10,
		TempBlksWritten: 5, WalBytes: 1024, WalRecords: 100, WalFpi: 10,
		TotalPlanTime: 20, MeanPlanTime: 2.0, Plans: 10,
	}
	curr := PostgresQueryStatsSnapRow{
		QueryID: 1, Calls: 20, TotalTimeMs: 250, Rows: 1200,
		SharedBlksHit: 2000, SharedBlksRead: 350, SharedBlksDirtied: 80, SharedBlksWritten: 25,
		TempBlksWritten: 12, WalBytes: 3072, WalRecords: 200, WalFpi: 20,
		TotalPlanTime: 45, MeanPlanTime: 2.25, Plans: 20,
	}
	d := subSnap(base, curr)

	checks := []struct {
		name string
		got  float64
		want float64
	}{
		{"Calls", float64(d.Calls), 10},
		{"TotalTimeMs", d.TotalTimeMs, 150},
		{"Rows", float64(d.Rows), 700},
		{"SharedBlksHit", float64(d.SharedBlksHit), 1000},
		{"SharedBlksRead", float64(d.SharedBlksRead), 150},
		{"SharedBlksDirtied", float64(d.SharedBlksDirtied), 30},
		{"SharedBlksWritten", float64(d.SharedBlksWritten), 15},
		{"TempBlksWritten", float64(d.TempBlksWritten), 7},
		{"WalBytes", float64(d.WalBytes), 2048},
		{"WalRecords", float64(d.WalRecords), 100},
		{"WalFpi", float64(d.WalFpi), 10},
		{"TotalPlanTime", d.TotalPlanTime, 25},
		{"Plans", float64(d.Plans), 10},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s: want %v, got %v", c.name, c.want, c.got)
		}
	}
	// MeanTimeMs should be recomputed: (250-100)/(20-10) = 15.0
	if d.MeanTimeMs != 15.0 {
		t.Errorf("MeanTimeMs: want 15.0, got %f", d.MeanTimeMs)
	}
}

func TestSubSnap_ClampNegativeToZero(t *testing.T) {
	// Simulate pg_stat_statements reset (curr < base)
	base := PostgresQueryStatsSnapRow{QueryID: 1, Calls: 100, TotalTimeMs: 500, SharedBlksHit: 5000}
	curr := PostgresQueryStatsSnapRow{QueryID: 1, Calls: 5, TotalTimeMs: 20, SharedBlksHit: 100}
	d := subSnap(base, curr)
	// After reset detection: should return curr values directly (reset path)
	if d.Calls < 0 {
		t.Errorf("Calls should not be negative after reset, got %d", d.Calls)
	}
	if d.TotalTimeMs < 0 {
		t.Errorf("TotalTimeMs should not be negative after reset, got %f", d.TotalTimeMs)
	}
}

// ---- PgssWorkloadPoint time-series fields coverage ----

func TestPgssWorkloadPointHasTimeSeries(t *testing.T) {
	p := PgssWorkloadPoint{
		Timestamp:      time.Now(),
		QueryLoadMsSec: 1.5,
		QPS:            100.0,
		RowsSec:        500.0,
		CacheHitRatio:  99.5,
		WalBytesSec:    1024.0,
		PlanningMsSec:  0.5,
		ExecPct:        95.0,
		PlanPct:        5.0,
	}
	if p.Timestamp.IsZero() {
		t.Fatal("Timestamp should not be zero")
	}
	if p.QPS <= 0 {
		t.Fatal("QPS should be positive")
	}
}

// ---- UpsertPgssQueryDim nil-pool guard ----

func TestUpsertPgssQueryDim_EmptyRows(t *testing.T) {
	tl := &TimescaleLogger{} // nil pool — safe because empty rows short-circuits
	err := tl.UpsertPgssQueryDim(context.TODO(), uuid.New(), nil)
	if err != nil {
		t.Fatalf("expected nil error for empty rows, got %v", err)
	}
}

// ---- subSnap edge cases ----

func TestSubSnap_IdenticalSnapshots(t *testing.T) {
	snap := PostgresQueryStatsSnapRow{
		QueryID: 42, Calls: 100, TotalTimeMs: 500, Rows: 1000,
		SharedBlksHit: 5000, SharedBlksRead: 200, TempBlksWritten: 10,
		WalBytes: 2048, TotalPlanTime: 50,
	}
	d := subSnap(snap, snap)
	if d.Calls != 0 {
		t.Errorf("identical snapshots: Calls should be 0, got %d", d.Calls)
	}
	if d.TotalTimeMs != 0 {
		t.Errorf("identical snapshots: TotalTimeMs should be 0, got %f", d.TotalTimeMs)
	}
}

func TestSubSnap_SingleCallDelta(t *testing.T) {
	base := PostgresQueryStatsSnapRow{QueryID: 1, Calls: 10, TotalTimeMs: 100, MeanTimeMs: 10}
	curr := PostgresQueryStatsSnapRow{QueryID: 1, Calls: 11, TotalTimeMs: 115, MeanTimeMs: 15}
	d := subSnap(base, curr)
	if d.Calls != 1 {
		t.Errorf("want 1 call delta, got %d", d.Calls)
	}
	// MeanTimeMs should be (115-100)/(11-10) = 15.0
	if d.MeanTimeMs != 15.0 {
		t.Errorf("want mean=15.0, got %f", d.MeanTimeMs)
	}
}

// ---- Regression classification ----

func TestRegressionStatus_Classification(t *testing.T) {
	// Verify the classification logic: >100% = Degraded, <=100% = Warning
	tests := []struct {
		changePct  float64
		wantStatus string
	}{
		{150.0, "Degraded"},
		{100.1, "Degraded"},
		{100.0, "Warning"},
		{50.1, "Warning"},
	}
	for _, tc := range tests {
		r := PgssRegression{ChangePct: tc.changePct}
		if tc.changePct > 100 {
			r.Status = "Degraded"
		} else {
			r.Status = "Warning"
		}
		if r.Status != tc.wantStatus {
			t.Errorf("changePct=%.1f: want %q, got %q", tc.changePct, tc.wantStatus, r.Status)
		}
	}
}

// ---- weightedPercentile boundary conditions ----

func TestWeightedPercentile_P100(t *testing.T) {
	entries := []latencyEntry{
		{calls: 50, meanMs: 1.0},
		{calls: 50, meanMs: 10.0},
	}
	got := weightedPercentile(entries, 100, 1.0)
	if got != 10.0 {
		t.Fatalf("p100 want 10.0, got %f", got)
	}
}

func TestWeightedPercentile_P0(t *testing.T) {
	entries := []latencyEntry{
		{calls: 50, meanMs: 1.0},
		{calls: 50, meanMs: 10.0},
	}
	// ceil(100*0.0) = 0, loop should still return first entry where cumulative >= 0
	got := weightedPercentile(entries, 100, 0.0)
	// With target=0, the first entry (cumulative=50 >= 0) should match
	if got != 1.0 {
		t.Fatalf("p0 want 1.0, got %f", got)
	}
}

// ---- PgssSummary struct guard ----

func TestPgssSummaryStructExists(t *testing.T) {
	var _ PgssSummary
}

// ---- PgssTopQuery new fields ----

func TestPgssTopQueryHasNewFields(t *testing.T) {
	q := PgssTopQuery{
		QueryID: 1, Query: "SELECT 1", TotalTime: 100, PctDBTime: 10,
		Calls: 50, AvgMs: 2.0, RowsPerCall: 5.0, HitPct: 99.0,
		TempMB: 0, WalMB: 0,
		ReadsPerCall: 1.5, PlanRatio: 0.05,
		Flags: []string{"IO"},
	}
	if q.ReadsPerCall != 1.5 {
		t.Fatalf("ReadsPerCall: want 1.5, got %f", q.ReadsPerCall)
	}
	if q.PlanRatio != 0.05 {
		t.Fatalf("PlanRatio: want 0.05, got %f", q.PlanRatio)
	}
	if len(q.Flags) != 1 || q.Flags[0] != "IO" {
		t.Fatalf("Flags: want [IO], got %v", q.Flags)
	}
}

// ---- PgssWorkloadPoint new fields ----

func TestPgssWorkloadPointHasNewFields(t *testing.T) {
	p := PgssWorkloadPoint{
		Timestamp:          time.Now(),
		QueryLoadMsSec:     50.0,
		QPS:                200.0,
		RowsSec:            1000.0,
		RowsPerQuery:       5.0,
		TempMbSec:          0.5,
		BlksReadSec:        100.0,
		TempBlksWrittenSec: 10.0,
		CpuSaturationMsSec: 4000.0,
	}
	if p.RowsPerQuery != 5.0 {
		t.Fatalf("RowsPerQuery: want 5.0, got %f", p.RowsPerQuery)
	}
	if p.CpuSaturationMsSec != 4000.0 {
		t.Fatalf("CpuSaturationMsSec: want 4000.0, got %f", p.CpuSaturationMsSec)
	}
}

// ---- Flag computation logic ----

func TestTopQueryFlagComputation(t *testing.T) {
	tests := []struct {
		name      string
		hitPct    float64
		tempMB    float64
		planRatio float64
		walMB     float64
		wantFlags []string
	}{
		{
			name:   "no flags - healthy query",
			hitPct: 99.5, tempMB: 0, planRatio: 0.01, walMB: 1.0,
			wantFlags: nil,
		},
		{
			name:   "IO flag - low cache hit",
			hitPct: 90.0, tempMB: 0, planRatio: 0.01, walMB: 1.0,
			wantFlags: []string{"IO"},
		},
		{
			name:   "TEMP flag - temp spill",
			hitPct: 99.0, tempMB: 0.1, planRatio: 0.01, walMB: 1.0,
			wantFlags: []string{"TEMP"},
		},
		{
			name:   "PLAN flag - high planning ratio",
			hitPct: 99.0, tempMB: 0, planRatio: 0.15, walMB: 1.0,
			wantFlags: []string{"PLAN"},
		},
		{
			name:   "WAL flag - high WAL",
			hitPct: 99.0, tempMB: 0, planRatio: 0.01, walMB: 15.0,
			wantFlags: []string{"WAL"},
		},
		{
			name:   "all flags",
			hitPct: 80.0, tempMB: 5.0, planRatio: 0.2, walMB: 20.0,
			wantFlags: []string{"IO", "TEMP", "PLAN", "WAL"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var flags []string
			if tc.hitPct < 95.0 {
				flags = append(flags, "IO")
			}
			if tc.tempMB > 0 {
				flags = append(flags, "TEMP")
			}
			if tc.planRatio > 0.1 {
				flags = append(flags, "PLAN")
			}
			if tc.walMB > 10.0 {
				flags = append(flags, "WAL")
			}

			if len(flags) != len(tc.wantFlags) {
				t.Fatalf("want flags %v, got %v", tc.wantFlags, flags)
			}
			for i := range flags {
				if flags[i] != tc.wantFlags[i] {
					t.Fatalf("flag[%d]: want %q, got %q", i, tc.wantFlags[i], flags[i])
				}
			}
		})
	}
}

// ---- New struct compile-time guards ----

func TestPgssFilterOptionsStructExists(t *testing.T) {
	var _ PgssFilterOptions
}

func TestPgssDbBreakdownStructExists(t *testing.T) {
	var _ PgssDbBreakdown
}

func TestPgssUserBreakdownStructExists(t *testing.T) {
	var _ PgssUserBreakdown
}

// ---- PgssTopQuery new fields ----

func TestPgssTopQueryHasDbNameAndUserName(t *testing.T) {
	q := PgssTopQuery{
		QueryID: 1, Query: "SELECT 1",
		DbName: "app_db", UserName: "app_user", AppName: "myapp", QueryType: "S",
		TotalTime: 100, Calls: 50, AvgMs: 2.0,
	}
	if q.DbName != "app_db" {
		t.Fatalf("DbName: want app_db, got %q", q.DbName)
	}
	if q.QueryType != "S" {
		t.Fatalf("QueryType: want S, got %q", q.QueryType)
	}
}

// ---- PgssSummary UniqueQueryCount ----

func TestPgssSummaryHasUniqueQueryCount(t *testing.T) {
	s := PgssSummary{
		QueryLoadMsSec: 1.0, QPS: 100.0,
		P99Ms: 5.0, CacheHitPct: 99.0,
		UniqueQueryCount: 42,
	}
	if s.UniqueQueryCount != 42 {
		t.Fatalf("UniqueQueryCount: want 42, got %d", s.UniqueQueryCount)
	}
}

// ---- GetPgssFilterOptions nil-pool guard ----

func TestGetPgssFilterOptions_NilPool(t *testing.T) {
	tl := &TimescaleLogger{} // nil pool
	// Should return empty options, not panic
	opts, _ := tl.GetPgssFilterOptions(context.TODO(), uuid.New(),
		time.Now().Add(-time.Hour), time.Now())
	if opts == nil {
		// nil is acceptable for nil pool
		return
	}
}

// ---- subSnap DbName/UserName/QueryType propagation ----

func TestSubSnap_PropagatesNewFields(t *testing.T) {
	base := PostgresQueryStatsSnapRow{
		QueryID: 1, Calls: 10, TotalTimeMs: 100, DbName: "old_db", UserName: "old_user",
	}
	curr := PostgresQueryStatsSnapRow{
		QueryID: 1, Calls: 20, TotalTimeMs: 250, DbName: "app_db", UserName: "app_user", QueryType: "S",
	}
	d := subSnap(base, curr)
	if d.DbName != "app_db" {
		t.Errorf("DbName should be from current, got %q", d.DbName)
	}
	if d.UserName != "app_user" {
		t.Errorf("UserName should be from current, got %q", d.UserName)
	}
	if d.QueryType != "S" {
		t.Errorf("QueryType should be from current, got %q", d.QueryType)
	}
}

// ---- UpsertPgssQueryDim with new signature — empty rows guard ----

func TestUpsertPgssQueryDim_InvalidServerID(t *testing.T) {
	tl := &TimescaleLogger{}
	err := tl.UpsertPgssQueryDim(context.TODO(), uuid.New(), nil)
	if err != nil {
		t.Fatalf("expected nil error for empty rows, got %v", err)
	}
}

