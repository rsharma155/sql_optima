package repository

import (
	"testing"
)

func TestComputeDelta(t *testing.T) {
	prev := PgQueryStat{
		QueryID:   1,
		Calls:     100,
		TotalTime: 1000.0,
		Rows:      500,
	}

	t.Run("Normal Delta", func(t *testing.T) {
		curr := PgQueryStat{
			QueryID:   1,
			Calls:     110,
			TotalTime: 1100.0,
			Rows:      550,
		}

		delta, changed := ComputeDelta(curr, prev)
		if !changed {
			t.Error("Expected changed to be true")
		}
		if delta.Calls != 10 {
			t.Errorf("Expected delta calls 10, got %d", delta.Calls)
		}
		if delta.TotalTime != 100.0 {
			t.Errorf("Expected delta total time 100.0, got %f", delta.TotalTime)
		}
		if delta.Rows != 50 {
			t.Errorf("Expected delta rows 50, got %d", delta.Rows)
		}
		if delta.MeanTime != 10.0 {
			t.Errorf("Expected mean time 10.0, got %f", delta.MeanTime)
		}
	})

	t.Run("No Change", func(t *testing.T) {
		curr := prev
		_, changed := ComputeDelta(curr, prev)
		if changed {
			t.Error("Expected changed to be false")
		}
	})

	t.Run("Reset Detected", func(t *testing.T) {
		curr := PgQueryStat{
			QueryID:   1,
			Calls:     50, // decreased
			TotalTime: 500.0,
		}

		_, changed := ComputeDelta(curr, prev)
		if changed {
			t.Error("Expected changed to be false when reset detected")
		}
	})
}
