package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type testTokenVersionProvider struct {
	version int64
	err     error
}

func (p testTokenVersionProvider) GetTokenVersion(_ context.Context, _ string) (int64, error) {
	return p.version, p.err
}
func TestMiddlewareRejectsMissingToken(t *testing.T) {
	manager := newTestTokenManager(t, time.Now())
	handler := manager.Middleware(
		testTokenVersionProvider{version: 1},
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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
	handler := manager.Middleware(
		testTokenVersionProvider{version: 1},
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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
func TestMiddlewareRejectsInvalidTokenBeforeVersionCheck(t *testing.T) {
	manager := newTestTokenManager(t, time.Now())

	handler := manager.Middleware(
		testTokenVersionProvider{
			err: errors.New("database unavailable"),
		},
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			t.Fatal("protected handler must not be called")
		}),
	)

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

	handler := manager.Middleware(
		testTokenVersionProvider{version: 1},
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

	handler := expiredManager.Middleware(
		testTokenVersionProvider{version: 1},
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			t.Fatal("protected handler must not be called")
		}),
	)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}
func TestMiddlewareRejectsMismatchedTokenVersion(t *testing.T) {
	manager := newTestTokenManager(t, time.Now())
	token, err := manager.IssueWithVersion("test-user-id", 1, 1)
	if err != nil {
		t.Fatalf("IssueWithVersion() error = %v", err)
	}

	handler := manager.Middleware(
		testTokenVersionProvider{version: 2},
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			t.Fatal("protected handler must not be called")
		}),
	)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestMiddlewareFailsClosedWhenVersionCheckFails(t *testing.T) {
	manager := newTestTokenManager(t, time.Now())
	token, err := manager.IssueWithVersion("test-user-id", 1, 1)
	if err != nil {
		t.Fatalf("IssueWithVersion() error = %v", err)
	}

	handler := manager.Middleware(
		testTokenVersionProvider{err: errors.New("database unavailable")},
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			t.Fatal("protected handler must not be called")
		}),
	)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}
