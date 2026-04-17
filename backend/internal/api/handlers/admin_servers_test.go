package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/rsharma155/sql_optima/internal/domain/servers"
	"github.com/rsharma155/sql_optima/internal/middleware"
	"github.com/rsharma155/sql_optima/internal/security"
	"github.com/rsharma155/sql_optima/internal/service"
)

var errNotFound = errors.New("not found")

type fakeKMS struct {
	dekPlain []byte
	dekEnc   []byte
}

func (f fakeKMS) GenerateDataKey(ctx context.Context) ([]byte, []byte, error) {
	return append([]byte(nil), f.dekPlain...), append([]byte(nil), f.dekEnc...), nil
}
func (f fakeKMS) DecryptDataKey(ctx context.Context, enc []byte) ([]byte, error) {
	return append([]byte(nil), f.dekPlain...), nil
}

type memServerStore struct {
	created []servers.Server
	encByID map[string]struct {
		s   servers.Server
		sec []byte
		dek []byte
	}
}

func (m *memServerStore) Create(ctx context.Context, s servers.Server, encryptedSecret, encryptedDEK []byte) (servers.Server, error) {
	s.ID = "srv_1"
	m.created = append(m.created, s)
	return s, nil
}
func (m *memServerStore) List(ctx context.Context, activeOnly bool) ([]servers.Server, error) {
	return append([]servers.Server(nil), m.created...), nil
}
func (m *memServerStore) GetByName(ctx context.Context, name string) (servers.Server, error) {
	for _, s := range m.created {
		if s.Name == name {
			return s, nil
		}
	}
	return servers.Server{}, errNotFound
}
func (m *memServerStore) GetEncrypted(ctx context.Context, id string) (servers.Server, []byte, []byte, error) {
	if m.encByID == nil {
		return servers.Server{}, nil, nil, errNotFound
	}
	v, ok := m.encByID[id]
	if !ok {
		return servers.Server{}, nil, nil, errNotFound
	}
	return v.s, v.sec, v.dek, nil
}
func (m *memServerStore) Delete(ctx context.Context, id string) error { return nil }
func (m *memServerStore) SetActive(ctx context.Context, id string, active bool) error {
	return nil
}
func (m *memServerStore) UpdateMetadata(ctx context.Context, id string, name, host string, port int, username, sslMode string) error {
	return nil
}
func (m *memServerStore) UpdateCredentials(ctx context.Context, id string, encryptedSecret, encryptedDEK []byte) error {
	return nil
}
func (m *memServerStore) TouchLastTest(ctx context.Context, id string, at time.Time) error { return nil }

func TestAdminServers_AddServer_ValidatesAndDoesNotEchoPassword(t *testing.T) {
	store := &memServerStore{}
	kms := fakeKMS{dekPlain: bytes.Repeat([]byte{0x11}, 32), dekEnc: []byte("enc")}
	sb := security.NewEnvelopeSecretBox()
	ms := &service.MetricsService{ServerRepo: store, ServerKMS: kms, ServerSecretBox: sb}
	h := NewAdminServerHandlers(ms)

	middleware.SetJWTSecret(bytes.Repeat([]byte("k"), 32))
	tok, err := middleware.GenerateToken(1, "admin", "admin")
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	body := map[string]any{
		"name":     "Prod PG",
		"db_type":  "postgres",
		"host":     "10.0.0.5",
		"port":     5432,
		"username": "monitor",
		"password": "supersecret",
		"ssl_mode": "require",
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/servers", bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()

	// Wrap with auth middleware to populate context claims.
	middleware.RequireAuth("admin")(http.HandlerFunc(h.AddServer)).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if len(store.created) != 1 {
		t.Fatalf("expected 1 create, got %d", len(store.created))
	}
	if bytes.Contains(rr.Body.Bytes(), []byte("supersecret")) {
		t.Fatalf("response should not contain password")
	}
}

func TestAdminServers_ListServers_DoesNotReturnSecrets(t *testing.T) {
	store := &memServerStore{}
	kms := fakeKMS{dekPlain: bytes.Repeat([]byte{0x11}, 32), dekEnc: []byte("enc")}
	sb := security.NewEnvelopeSecretBox()
	ms := &service.MetricsService{ServerRepo: store, ServerKMS: kms, ServerSecretBox: sb}
	h := NewAdminServerHandlers(ms)

	middleware.SetJWTSecret(bytes.Repeat([]byte("k"), 32))
	tok, err := middleware.GenerateToken(1, "admin", "admin")
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	// seed
	_, _ = store.Create(context.Background(), servers.Server{Name: "x", DBType: servers.DBPostgres, Host: "h", Port: 1, Username: "u", IsActive: true}, []byte("sec"), []byte("dek"))

	req := httptest.NewRequest(http.MethodGet, "/api/admin/servers", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	middleware.RequireAuth("admin")(http.HandlerFunc(h.ListServers)).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if bytes.Contains(rr.Body.Bytes(), []byte("encrypted_secret")) || bytes.Contains(rr.Body.Bytes(), []byte("password")) {
		t.Fatalf("list response leaked secret fields: %s", rr.Body.String())
	}
}

type fakeTester struct{ err error }

func (t fakeTester) Test(ctx context.Context, s servers.Server, cred servers.CredentialPayload) error {
	return t.err
}

func TestAdminServers_TestServer_DecryptsAndInvokesTester(t *testing.T) {
	store := &memServerStore{encByID: map[string]struct {
		s   servers.Server
		sec []byte
		dek []byte
	}{}}
	kms := fakeKMS{dekPlain: bytes.Repeat([]byte{0x11}, 32), dekEnc: []byte("enc")}
	sb := security.NewEnvelopeSecretBox()
	ms := &service.MetricsService{ServerRepo: store, ServerKMS: kms, ServerSecretBox: sb}
	h := NewAdminServerHandlers(ms)
	h.tester = fakeTester{err: nil}

	middleware.SetJWTSecret(bytes.Repeat([]byte("k"), 32))
	tok, _ := middleware.GenerateToken(1, "admin", "admin")

	plain := []byte(`{"password":"p","sslmode":"require","extra":{}}`)
	sec, err := sb.Encrypt(plain, bytes.Repeat([]byte{0x11}, 32))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	store.encByID["00000000-0000-0000-0000-000000000000"] = struct {
		s   servers.Server
		sec []byte
		dek []byte
	}{
		s:   servers.Server{ID: "00000000-0000-0000-0000-000000000000", DBType: servers.DBPostgres, Host: "h", Port: 1, Username: "u", SSLMode: "require"},
		sec: sec,
		dek: []byte("enc"),
	}

	req := httptest.NewRequest(http.MethodPost, "/api/admin/servers/00000000-0000-0000-0000-000000000000/test", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "00000000-0000-0000-0000-000000000000"})
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	middleware.RequireAuth("admin")(http.HandlerFunc(h.TestServer)).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}
