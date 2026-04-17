package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// dummy handler that records it was called
var ok = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
})

func TestCSRFProtect_SafeMethodsExempt(t *testing.T) {
	handler := CSRFProtect(ok)
	for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodOptions} {
		r := httptest.NewRequest(method, "/", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Errorf("%s: expected 200, got %d", method, w.Code)
		}
	}
}

func TestCSRFProtect_BearerTokenExempt(t *testing.T) {
	handler := CSRFProtect(ok)
	r := httptest.NewRequest(http.MethodPost, "/api/test", nil)
	r.Header.Set("Authorization", "Bearer some-jwt-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("Bearer request: expected 200, got %d", w.Code)
	}
}

func TestCSRFProtect_MissingCookieFails(t *testing.T) {
	handler := CSRFProtect(ok)
	r := httptest.NewRequest(http.MethodPost, "/api/test", nil)
	r.Header.Set(CSRFHeaderName, "some-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Errorf("missing cookie: expected 403, got %d", w.Code)
	}
}

func TestCSRFProtect_MissingHeaderFails(t *testing.T) {
	handler := CSRFProtect(ok)
	r := httptest.NewRequest(http.MethodPost, "/api/test", nil)
	r.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: "some-token"})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Errorf("missing header: expected 403, got %d", w.Code)
	}
}

func TestCSRFProtect_MismatchFails(t *testing.T) {
	handler := CSRFProtect(ok)
	r := httptest.NewRequest(http.MethodPost, "/api/test", nil)
	r.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: "token-a"})
	r.Header.Set(CSRFHeaderName, "token-b")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Errorf("mismatch: expected 403, got %d", w.Code)
	}
}

func TestCSRFProtect_MatchingTokenSucceeds(t *testing.T) {
	handler := CSRFProtect(ok)
	token := "valid-csrf-token"
	r := httptest.NewRequest(http.MethodPost, "/api/test", nil)
	r.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: token})
	r.Header.Set(CSRFHeaderName, token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("matching token: expected 200, got %d", w.Code)
	}
}

func TestCSRFProtect_AllMutatingMethods(t *testing.T) {
	handler := CSRFProtect(ok)
	token := "valid-csrf-token"
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		r := httptest.NewRequest(method, "/api/test", nil)
		r.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: token})
		r.Header.Set(CSRFHeaderName, token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Errorf("%s with valid token: expected 200, got %d", method, w.Code)
		}
	}
}

func TestGenerateCSRFToken(t *testing.T) {
	tok, err := GenerateCSRFToken()
	if err != nil {
		t.Fatalf("GenerateCSRFToken: %v", err)
	}
	if len(tok) < 40 {
		t.Errorf("token too short: %d chars", len(tok))
	}
}
