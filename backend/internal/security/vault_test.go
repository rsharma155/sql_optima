package security

import "testing"

func TestNormalizeVaultTransitMount(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"", "transit"},
		{"  transit  ", "transit"},
		{"/transit/", "transit"},
		{"team_transit", "team_transit"},
		{"bad/mount", "transit"},
		{"bad..x", "transit"},
		{"bad space", "transit"},
	}
	for _, tc := range tests {
		if got := NormalizeVaultTransitMount(tc.in); got != tc.want {
			t.Fatalf("NormalizeVaultTransitMount(%q) = %q want %q", tc.in, got, tc.want)
		}
	}
}
