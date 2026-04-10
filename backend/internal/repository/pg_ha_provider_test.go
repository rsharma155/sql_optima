package repository

import "testing"

func TestDetectHaProviderAuto(t *testing.T) {
	got := DetectHaProviderAuto("my-cnpg-cluster", []string{"replica-1"}, true)
	if got.Provider != HaProviderCNPG {
		t.Fatalf("expected cnpg, got %s", got.Provider)
	}

	got2 := DetectHaProviderAuto("patroni-prod", []string{"replica"}, true)
	if got2.Provider != HaProviderPatroni {
		t.Fatalf("expected patroni, got %s", got2.Provider)
	}

	got3 := DetectHaProviderAuto("", []string{"walreceiver"}, true)
	if got3.Provider != HaProviderStreaming {
		t.Fatalf("expected streaming, got %s", got3.Provider)
	}

	got4 := DetectHaProviderAuto("", nil, false)
	if got4.Provider != HaProviderStandalone {
		t.Fatalf("expected standalone, got %s", got4.Provider)
	}
}

