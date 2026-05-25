package hot

import "testing"

func TestNormalizeStatementTextForFingerprint_literalsAndNumbers(t *testing.T) {
	a := NormalizeStatementTextForFingerprint("SELECT * FROM t WHERE id = 1 AND name = 'alice'")
	b := NormalizeStatementTextForFingerprint("SELECT   *   FROM t WHERE id = 42 AND name = 'bob'")
	if a != b {
		t.Fatalf("expected same fingerprint input, got %q vs %q", a, b)
	}
	if a != "SELECT * FROM t WHERE id = ? AND name = '?'" {
		t.Fatalf("unexpected normalized: %q", a)
	}
}

func TestStatementFingerprintKey_planEvictedUsesHash(t *testing.T) {
	key := StatementFingerprintKey("Plan Evicted", "", 0xABCD)
	if key != "hash:43981" {
		t.Fatalf("got %q", key)
	}
}

func TestStatementFingerprintKey_samePatternSameKey(t *testing.T) {
	k1 := StatementFingerprintKey("SELECT id FROM users WHERE id = 1", "", 111)
	k2 := StatementFingerprintKey("SELECT id FROM users WHERE id = 99", "", 222)
	if k1 != k2 {
		t.Fatalf("expected same key, got %q vs %q", k1, k2)
	}
	if len(k1) != 32 {
		t.Fatalf("expected md5 hex, got %q", k1)
	}
}

func TestStatementFingerprintKey_emptyUsesHash(t *testing.T) {
	if StatementFingerprintKey("", "", 99) != "hash:99" {
		t.Fatal("expected hash fallback")
	}
}
