package unit_test

import "testing"

func TestPerfDebt_MissingIndexSeverityThresholds(t *testing.T) {
	// Mirrors the current collector thresholds:
	// CRITICAL >= 1,000,000
	// WARNING  >=   250,000
	// INFO     otherwise
	cases := []struct {
		score float64
		want  string
	}{
		{0, "INFO"},
		{249999, "INFO"},
		{250000, "WARNING"},
		{999999, "WARNING"},
		{1000000, "CRITICAL"},
	}

	for _, c := range cases {
		got := perfDebtMissingIndexSeverity(c.score)
		if got != c.want {
			t.Fatalf("score=%v got=%s want=%s", c.score, got, c.want)
		}
	}
}

func TestPerfDebt_IndexFragmentationSeverityThresholds(t *testing.T) {
	// WARNING >= 30, CRITICAL >= 60 (collector only emits rows >=30)
	cases := []struct {
		frag float64
		want string
	}{
		{30, "WARNING"},
		{59.9, "WARNING"},
		{60, "CRITICAL"},
		{99, "CRITICAL"},
	}
	for _, c := range cases {
		got := perfDebtFragSeverity(c.frag)
		if got != c.want {
			t.Fatalf("frag=%v got=%s want=%s", c.frag, got, c.want)
		}
	}
}

func TestPerfDebt_BackupAgeSeverityThresholds(t *testing.T) {
	// WARNING >= 48h, CRITICAL >= 168h
	cases := []struct {
		age float64
		want string
	}{
		{0, "INFO"},
		{47.9, "INFO"},
		{48, "WARNING"},
		{167.9, "WARNING"},
		{168, "CRITICAL"},
	}
	for _, c := range cases {
		got := perfDebtBackupSeverity(c.age)
		if got != c.want {
			t.Fatalf("age=%v got=%s want=%s", c.age, got, c.want)
		}
	}
}

func perfDebtMissingIndexSeverity(score float64) string {
	if score >= 1_000_000 {
		return "CRITICAL"
	}
	if score >= 250_000 {
		return "WARNING"
	}
	return "INFO"
}

func perfDebtFragSeverity(frag float64) string {
	if frag >= 60 {
		return "CRITICAL"
	}
	return "WARNING"
}

func perfDebtBackupSeverity(ageH float64) string {
	if ageH >= 168 {
		return "CRITICAL"
	}
	if ageH >= 48 {
		return "WARNING"
	}
	return "INFO"
}

