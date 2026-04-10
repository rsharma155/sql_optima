package unit_test

import (
	"testing"

	"github.com/rsharma155/sql_optima/pkg/dashboard"
)

func TestCategorizeWaitType(t *testing.T) {
	tests := []struct {
		waitType string
		want     dashboard.WaitCategory
	}{
		{"SOS_SCHEDULER_YIELD", dashboard.WaitCPU},
		{"pageiolatch_sh", dashboard.WaitIO},
		{"WRITELOG", dashboard.WaitLog},
		{"LCK_M_S", dashboard.WaitLocking},
		{"RESOURCE_SEMAPHORE", dashboard.WaitMemory},
		{"ASYNC_NETWORK_IO", dashboard.WaitNetwork},
		{"SLEEP_TASK", dashboard.WaitOther},
	}

	for _, tt := range tests {
		got := dashboard.CategorizeWaitType(tt.waitType)
		if got != tt.want {
			t.Fatalf("waitType=%q got=%q want=%q", tt.waitType, got, tt.want)
		}
	}
}

func TestComputeWaitDeltas_ClampsNegative(t *testing.T) {
	prev := map[string]float64{"WRITELOG": 1000}
	curr := map[string]float64{"WRITELOG": 10} // reset scenario

	deltas, nextPrev := dashboard.ComputeWaitDeltas(prev, curr)
	if len(deltas) != 1 {
		t.Fatalf("len(deltas)=%d want=1", len(deltas))
	}
	if deltas[0].DeltaMs != 0 {
		t.Fatalf("delta=%v want=0", deltas[0].DeltaMs)
	}
	if nextPrev["WRITELOG"] != 10 {
		t.Fatalf("nextPrev=%v want=10", nextPrev["WRITELOG"])
	}
}

func TestCompilationRatioAndSeverity(t *testing.T) {
	r := dashboard.CompilationRatio(100, 5)
	if r != 0.05 {
		t.Fatalf("ratio=%v want=0.05", r)
	}
	if s := dashboard.CompilationSeverity(r); s != dashboard.SeverityOK {
		t.Fatalf("severity=%v want OK", s)
	}

	r2 := dashboard.CompilationRatio(100, 20)
	if s := dashboard.CompilationSeverity(r2); s != dashboard.SeverityWarning {
		t.Fatalf("severity=%v want WARNING", s)
	}
}

