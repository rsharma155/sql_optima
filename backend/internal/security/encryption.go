package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"io"
)

// EnvelopeSecretBox encrypts secrets using AES-256-GCM with a per-secret nonce.
// Ciphertext format: nonce(12) || gcm(ciphertext+tag).
type EnvelopeSecretBox struct{}

func NewEnvelopeSecretBox() *EnvelopeSecretBox { return &EnvelopeSecretBox{} }

func (b *EnvelopeSecretBox) Encrypt(plaintextJSON []byte, plaintextDEK []byte) ([]byte, error) {
	if len(plaintextDEK) != 32 {
		return nil, errors.New("dek must be 32 bytes (AES-256)")
	}
	block, err := aes.NewCipher(plaintextDEK)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	// No AAD; callers can add it later if we bind ciphertext to server_id.
	ct := gcm.Seal(nil, nonce, plaintextJSON, nil)
	out := make([]byte, 0, len(nonce)+len(ct))
	out = append(out, nonce...)
	out = append(out, ct...)
	return out, nil
}

func (b *EnvelopeSecretBox) Decrypt(ciphertext []byte, plaintextDEK []byte) ([]byte, error) {
	if len(plaintextDEK) != 32 {
		return nil, errors.New("dek must be 32 bytes (AES-256)")
	}
	block, err := aes.NewCipher(plaintextDEK)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	ns := gcm.NonceSize()
	if len(ciphertext) < ns+16 {
		return nil, errors.New("ciphertext too short")
	}
	nonce := ciphertext[:ns]
	body := ciphertext[ns:]
	pt, err := gcm.Open(nil, nonce, body, nil)
	if err != nil {
		return nil, err
	}
	return pt, nil
}
