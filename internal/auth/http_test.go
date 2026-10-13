package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"moonflux/internal/user"
)

type fakeUserAuthenticator struct {
	user user.User
	err  error
}

func (f fakeUserAuthenticator) Authenticate(
	_ context.Context,
	_ user.LoginInput,
) (user.User, error) {
	return f.user, f.err
}

func newTestHTTPHandler(t *testing.T, authErr error) *HTTPHandler {
	t.Helper()

	authenticator := fakeUserAuthenticator{
		user: user.User{
			ID:   "test-user-id",
			Role: user.RoleUser,
		},
		err: authErr,
	}

	tokens, err := NewTokenManager(TokenConfig{
		Issuer: "moonflux-test",
		Secret: []byte("test-secret-at-least-32-bytes-long"),
		TTL:    15 * 60 * 1e9,
	})
	if err != nil {
		t.Fatalf("NewTokenManager() error = %v", err)
	}

	loginService, err := NewLoginService(authenticator, tokens)
	if err != nil {
		t.Fatalf("NewLoginService() error = %v", err)
	}

	return NewHTTPHandler(loginService)
}

func TestHTTPHandlerLoginSuccess(t *testing.T) {
	handler := newTestHTTPHandler(t, nil)
	req := httptest.NewRequest(
		http.MethodPost,
		"/auth/login",
		strings.NewReader(`{"login":"test","password":"password"}`),
	)
	rec := httptest.NewRecorder()

	handler.Login(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s",
			rec.Code, http.StatusOK, rec.Body.String())
	}

	if got := rec.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q", got)
	}

	var result LoginResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if result.TokenType != "Bearer" {
		t.Errorf("token_type = %q, want Bearer", result.TokenType)
	}
	if result.AccessToken == "" {
		t.Error("access_token is empty")
	}
	if result.ExpiresIn <= 0 {
		t.Errorf("expires_in = %d, want positive value", result.ExpiresIn)
	}
}

func TestHTTPHandlerLoginInvalidCredentials(t *testing.T) {
	handler := newTestHTTPHandler(t, user.ErrInvalidCredentials)
	req := httptest.NewRequest(
		http.MethodPost,
		"/auth/login",
		strings.NewReader(`{"login":"test","password":"wrong"}`),
	)
	rec := httptest.NewRecorder()

	handler.Login(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if strings.TrimSpace(rec.Body.String()) != "invalid login or password" {
		t.Errorf("unexpected response body: %q", rec.Body.String())
	}
}

func TestHTTPHandlerLoginInternalError(t *testing.T) {
	handler := newTestHTTPHandler(t, errors.New("database unavailable"))
	req := httptest.NewRequest(
		http.MethodPost,
		"/auth/login",
		strings.NewReader(`{"login":"test","password":"password"}`),
	)
	rec := httptest.NewRecorder()

	handler.Login(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if strings.TrimSpace(rec.Body.String()) != "authentication unavailable" {
		t.Errorf("unexpected response body: %q", rec.Body.String())
	}
}

func TestHTTPHandlerLoginRejectsInvalidJSON(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "malformed JSON", body: `{"login":`},
		{name: "unknown field", body: `{"login":"test","password":"password","admin":true}`},
		{name: "trailing JSON", body: `{"login":"test","password":"password"} {}`},
		{name: "non-object", body: `null`},
	}

	handler := newTestHTTPHandler(t, nil)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(
				http.MethodPost,
				"/auth/login",
				strings.NewReader(tt.body),
			)
			rec := httptest.NewRecorder()

			handler.Login(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d; body = %s",
					rec.Code, http.StatusBadRequest, rec.Body.String())
			}
		})
	}
}

func TestHTTPHandlerLoginRejectsWrongMethod(t *testing.T) {
	handler := newTestHTTPHandler(t, nil)
	req := httptest.NewRequest(http.MethodGet, "/auth/login", nil)
	rec := httptest.NewRecorder()

	handler.Login(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
	if got := rec.Header().Get("Allow"); got != http.MethodPost {
		t.Errorf("Allow = %q, want POST", got)
	}
}
