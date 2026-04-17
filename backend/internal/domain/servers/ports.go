package servers

import (
	"context"
	"time"
)

// KeyManagementService generates and decrypts data-encryption keys (DEKs).
// In production, this is backed by Vault Transit/KMS.
type KeyManagementService interface {
	GenerateDataKey(ctx context.Context) (plaintextDEK []byte, encryptedDEK []byte, err error)
	DecryptDataKey(ctx context.Context, encryptedDEK []byte) (plaintextDEK []byte, err error)
}

// SecretBox encrypts/decrypts credential JSON using a plaintext DEK (envelope encryption).
type SecretBox interface {
	Encrypt(plaintextJSON []byte, plaintextDEK []byte) (ciphertext []byte, err error)
	Decrypt(ciphertext []byte, plaintextDEK []byte) (plaintextJSON []byte, err error)
}

// ServerStore persists server metadata + encrypted credential blob and encrypted DEK.
type ServerStore interface {
	Create(ctx context.Context, s Server, encryptedSecret, encryptedDEK []byte) (Server, error)
	List(ctx context.Context, activeOnly bool) ([]Server, error)
	GetByName(ctx context.Context, name string) (Server, error)
	GetEncrypted(ctx context.Context, id string) (s Server, encryptedSecret, encryptedDEK []byte, err error)
	// Delete removes the server row from the registry (credentials and metadata).
	Delete(ctx context.Context, id string) error
	SetActive(ctx context.Context, id string, active bool) error
	UpdateMetadata(ctx context.Context, id string, name, host string, port int, username, sslMode string) error
	UpdateCredentials(ctx context.Context, id string, encryptedSecret, encryptedDEK []byte) error
	TouchLastTest(ctx context.Context, id string, at time.Time) error
}

type AuditLogger interface {
	Log(ctx context.Context, eventType string, serverID string, actor string, ipAddress string, metadata map[string]interface{}) error
}
