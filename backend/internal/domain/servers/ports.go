package servers

import (
	"context"
	"github.com/google/uuid"
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
	GetEncrypted(ctx context.Context, id uuid.UUID) (s Server, encryptedSecret, encryptedDEK []byte, err error)
	// Delete removes the server row from the registry (credentials and metadata).
	Delete(ctx context.Context, id uuid.UUID) error
	SetActive(ctx context.Context, id uuid.UUID, active bool) error
	UpdateMetadata(ctx context.Context, id uuid.UUID, name, host string, port int, username, sslMode string) error
	UpdateCredentials(ctx context.Context, id uuid.UUID, encryptedSecret, encryptedDEK []byte) error
	TouchLastTest(ctx context.Context, id uuid.UUID, at time.Time) error
	SetEngineEdition(ctx context.Context, id uuid.UUID, edition int) error
	CheckDuplicate(ctx context.Context, excludeID uuid.UUID, name, host string, port int) (string, error)
}

type AuditLogger interface {
	Log(ctx context.Context, eventType string, serverID uuid.UUID, actor string, ipAddress string, metadata map[string]interface{}) error
}
