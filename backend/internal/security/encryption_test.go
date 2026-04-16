package security

import (
	"bytes"
	"testing"
)

func TestEnvelopeSecretBox_RoundTrip(t *testing.T) {
	sb := NewEnvelopeSecretBox()
	dek := bytes.Repeat([]byte{0x42}, 32)
	plain := []byte(`{"password":"secret","sslmode":"require","extra":{"x":1}}`)

	ct, err := sb.Encrypt(plain, dek)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if bytes.Contains(ct, []byte("secret")) {
		t.Fatalf("ciphertext should not contain plaintext")
	}
	got, err := sb.Decrypt(ct, dek)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("roundtrip mismatch: got=%s want=%s", string(got), string(plain))
	}
}

func TestEnvelopeSecretBox_RejectsWrongDEKSize(t *testing.T) {
	sb := NewEnvelopeSecretBox()
	_, err := sb.Encrypt([]byte(`{}`), []byte("short"))
	if err == nil {
		t.Fatalf("expected error")
	}
}
