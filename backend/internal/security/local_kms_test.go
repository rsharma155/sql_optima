package security

import (
	"bytes"
	"context"
	"testing"
)

func TestLocalEnvelopeKMS_RoundTrip(t *testing.T) {
	kms, err := NewLocalEnvelopeKMS(bytes.Repeat([]byte("a"), 32))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	dek, enc, err := kms.GenerateDataKey(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(dek) != 32 {
		t.Fatalf("dek len %d", len(dek))
	}
	got, err := kms.DecryptDataKey(ctx, enc)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, dek) {
		t.Fatal("dek mismatch")
	}
}
