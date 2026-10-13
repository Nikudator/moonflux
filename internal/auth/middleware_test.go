package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestMiddlewareRejectsMissingToken(t *testing.T) {
	manager := newTestTokenManager(t, time.Now())
	handler := manager.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("protected handler must not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestMiddlewareRejectsInvalidToken(t *testing.T) {
	manager := newTestTokenManager(t, time.Now())
	handler := manager.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("protected handler must not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestMiddlewarePassesClaimsToContext(t *testing.T) {
	manager := newTestTokenManager(t, time.Now())
	token, err := manager.Issue("test-user-id", 1)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	handler := manager.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := ClaimsFromContext(r.Context())
		if !ok {
			t.Fatal("claims not found in request context")
		}
		if claims.Subject != "test-user-id" {
			t.Errorf("subject = %q, want test-user-id", claims.Subject)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}
func TestMiddlewareRejectsExpiredToken(t *testing.T) {
	now := time.Now().UTC()
	manager := newTestTokenManager(t, now)

	token, err := manager.Issue("test-user-id", 1)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	expiredManager, err := NewTokenManager(TokenConfig{
		Issuer: "moonflux-test",
		Secret: []byte("test-secret-at-least-32-bytes-long"),
		TTL:    15 * time.Minute,
		Now:    func() time.Time { return now.Add(2 * time.Hour) },
	})
	if err != nil {
		t.Fatalf("NewTokenManager() error = %v", err)
	}

	handler := expiredManager.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("protected handler must not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}
