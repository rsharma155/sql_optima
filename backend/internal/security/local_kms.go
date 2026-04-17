package security

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"io"
)

// LocalEnvelopeKMS implements KeyManagementService for single-node installs when HashiCorp
// Vault Transit is not configured. DEKs are encrypted at rest using a master key derived
// from JWT_SECRET (SHA-256). Production deployments should prefer Vault (VAULT_ADDR).
type LocalEnvelopeKMS struct {
	masterDEK []byte
}

// NewLocalEnvelopeKMS derives a 32-byte AES key from secret material (typically JWT_SECRET).
func NewLocalEnvelopeKMS(secretMaterial []byte) (*LocalEnvelopeKMS, error) {
	if len(secretMaterial) < 16 {
		return nil, errors.New("secret material too short for local KMS (use at least 16 bytes)")
	}
	sum := sha256.Sum256(secretMaterial)
	k := append([]byte(nil), sum[:]...)
	return &LocalEnvelopeKMS{masterDEK: k}, nil
}

func (k *LocalEnvelopeKMS) GenerateDataKey(ctx context.Context) ([]byte, []byte, error) {
	if k == nil || len(k.masterDEK) != 32 {
		return nil, nil, errors.New("local KMS not initialized")
	}
	dek := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, dek); err != nil {
		return nil, nil, err
	}
	box := NewEnvelopeSecretBox()
	enc, err := box.Encrypt(dek, k.masterDEK)
	if err != nil {
		for i := range dek {
			dek[i] = 0
		}
		return nil, nil, err
	}
	return dek, enc, nil
}

func (k *LocalEnvelopeKMS) DecryptDataKey(ctx context.Context, encryptedDEK []byte) ([]byte, error) {
	if k == nil || len(k.masterDEK) != 32 {
		return nil, errors.New("local KMS not initialized")
	}
	box := NewEnvelopeSecretBox()
	return box.Decrypt(encryptedDEK, k.masterDEK)
}
