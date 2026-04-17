package middleware

import (
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func setupTestJWT(t *testing.T) string {
	t.Helper()
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		t.Fatal(err)
	}
	SetJWTSecret(secret)
	tok, err := GenerateToken(1, "testuser", "admin")
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

var okHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
})

func TestRequireAuth_BearerSuccess(t *testing.T) {
	tok := setupTestJWT(t)
	handler := RequireAuth("")(okHandler)

	r := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	r.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestRequireAuth_CookieSuccess(t *testing.T) {
	tok := setupTestJWT(t)
	handler := RequireAuth("")(okHandler)

	r := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	r.AddCookie(&http.Cookie{Name: AuthCookieName, Value: tok})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestRequireAuth_BearerPrecedenceOverCookie(t *testing.T) {
	tok := setupTestJWT(t)
	handler := RequireAuth("")(okHandler)

	r := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	r.Header.Set("Authorization", "Bearer "+tok)
	r.AddCookie(&http.Cookie{Name: AuthCookieName, Value: "invalid-cookie-token"})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 (Bearer should take precedence), got %d", w.Code)
	}
}

func TestRequireAuth_MissingTokenFails(t *testing.T) {
	_ = setupTestJWT(t)
	handler := RequireAuth("")(okHandler)

	r := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestRequireAuth_InvalidCookieFails(t *testing.T) {
	_ = setupTestJWT(t)
	handler := RequireAuth("")(okHandler)

	r := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	r.AddCookie(&http.Cookie{Name: AuthCookieName, Value: "bad-token"})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestRequireAuth_RoleCheckForbidden(t *testing.T) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		t.Fatal(err)
	}
	SetJWTSecret(secret)
	tok, err := GenerateToken(2, "viewer", "viewer")
	if err != nil {
		t.Fatal(err)
	}
	handler := RequireAuth("admin")(okHandler)

	r := httptest.NewRequest(http.MethodGet, "/api/admin", nil)
	r.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestValidateLocalJWT_EmptySecretFails(t *testing.T) {
	orig := JWTSecret
	JWTSecret = nil
	defer func() { JWTSecret = orig }()

	_, err := validateLocalJWT("some-token")
	if err == nil {
		t.Error("expected error when JWTSecret is nil")
	}
}

func TestGenerateToken_EmptySecretFails(t *testing.T) {
	orig := JWTSecret
	JWTSecret = nil
	defer func() { JWTSecret = orig }()

	_, err := GenerateToken(1, "test", "admin")
	if err == nil {
		t.Error("expected error when JWTSecret is nil")
	}
}

func TestRequireAuth_ResponseIsJSON(t *testing.T) {
	_ = setupTestJWT(t)
	handler := RequireAuth("")(okHandler)

	r := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if _, ok := body["error"]; !ok {
		t.Error("expected 'error' key in JSON response")
	}
}
