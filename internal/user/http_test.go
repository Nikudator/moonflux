package user

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type httpTestCreator struct {
	created User
	err     error
	called  bool
}

func (f *httpTestCreator) Create(_ context.Context, u User) error {
	f.called = true
	f.created = u
	return f.err
}

func TestRegisterHandlerSuccess(t *testing.T) {
	repo := &httpTestCreator{}

	body := `{
		"nickname": "moonflux",
		"email": "moon@example.com",
		"first_name": "Nikolai",
		"last_name": "Example",
		"password": "correct horse battery staple"
	}`
	req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler := NewHTTPHandler(NewService(repo, false))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if !repo.called {
		t.Fatal("repository Create was not called")
	}
	if repo.created.PasswordHash == "" || repo.created.PasswordHash == "correct horse battery staple" {
		t.Fatal("password was not hashed")
	}

	response := rec.Body.String()
	if strings.Contains(response, "password_hash") {
		t.Fatal("response exposes password_hash")
	}
	if strings.Contains(response, "correct horse battery staple") {
		t.Fatal("response exposes the original password")
	}
	if !strings.Contains(response, `"nickname":"moonflux"`) {
		t.Fatalf("response missing nickname: %s", response)
	}
}

func TestRegisterHandlerRejectsInvalidJSON(t *testing.T) {
	repo := &httpTestCreator{}

	req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(`{"nickname":`))
	rec := httptest.NewRecorder()
	handler := NewHTTPHandler(NewService(repo, false))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if repo.called {
		t.Fatal("repository Create was called for invalid JSON")
	}
}

func TestRegisterHandlerRejectsUnknownFields(t *testing.T) {
	repo := &httpTestCreator{}

	body := `{
		"nickname": "moonflux",
		"first_name": "Nikolai",
		"last_name": "Example",
		"password": "some password",
		"role": 9
	}`
	req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler := NewHTTPHandler(NewService(repo, false))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if repo.called {
		t.Fatal("repository Create was called for unknown fields")
	}
}

func TestRegisterHandlerConflict(t *testing.T) {
	repo := &httpTestCreator{err: ErrConflict}

	body := `{
		"nickname": "moonflux",
		"first_name": "Nikolai",
		"last_name": "Example",
		"password": "some password"
	}`
	req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler := NewHTTPHandler(NewService(repo, false))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusConflict, rec.Body.String())
	}
}

func TestRegisterHandlerHidesInternalErrors(t *testing.T) {
	repo := &httpTestCreator{err: errors.New("database credentials leaked")}

	body := `{
		"nickname": "moonflux",
		"first_name": "Nikolai",
		"last_name": "Example",
		"password": "some password"
	}`
	req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler := NewHTTPHandler(NewService(repo, false))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if strings.Contains(rec.Body.String(), "database credentials leaked") {
		t.Fatal("response exposes internal error")
	}
}

func TestRegisterHandlerSendsVerification(t *testing.T) {
	repo := &httpTestCreator{}
	var senderCalled bool
	var senderUserID, senderEmail string

	sender := VerificationSender(func(
		_ context.Context,
		userID string,
		email string,
	) error {
		senderCalled = true
		senderUserID = userID
		senderEmail = email
		return nil
	})

	body := `{
		"nickname": "moonflux",
		"email": "moon@example.com",
		"first_name": "Nikolai",
		"last_name": "Example",
		"password": "correct horse battery staple"
	}`

	req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler := NewHTTPHandlerWithVerification(NewService(repo, false), sender)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if !senderCalled {
		t.Fatal("verification sender was not called")
	}
	if senderUserID == "" {
		t.Fatal("verification sender received an empty user ID")
	}
	if senderEmail != "moon@example.com" {
		t.Fatalf("sender email = %q, want %q", senderEmail, "moon@example.com")
	}
}

func TestRegisterHandlerVerificationFailureStillReturns201(t *testing.T) {
	repo := &httpTestCreator{}
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuf, nil))

	sender := VerificationSender(func(_ context.Context, _ string, _ string) error {
		return errors.New("SMTP credentials leaked")
	})

	body := `{
		"nickname": "moonflux",
		"email": "moon@example.com",
		"first_name": "Nikolai",
		"last_name": "Example",
		"password": "correct horse battery staple"
	}`
	req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler := NewHTTPHandlerWithVerificationAndLogger(
		NewService(repo, false), sender, logger)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", rec.Code, rec.Body.String())
	}
	if !repo.called {
		t.Fatal("user was not created")
	}
	if strings.Contains(rec.Body.String(), "SMTP credentials leaked") {
		t.Fatal("response exposes internal SMTP error")
	}
	out := logBuf.String()
	if !strings.Contains(out, "verification_send_failed") {
		t.Errorf("expected log entry, got: %s", out)
	}
	if strings.Contains(out, "SMTP credentials leaked") {
		t.Errorf("log leaks internal SMTP error: %s", out)
	}
}

func TestRegisterHandlerRequiresEmail(t *testing.T) {
	repo := &httpTestCreator{}
	body := `{
		"nickname": "moonflux",
		"first_name": "Nikolai",
		"last_name": "Example",
		"password": "strong-password-123"
	}`
	req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler := NewHTTPHandler(NewService(repo, true)) // requireEmail = true
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", rec.Code, rec.Body.String())
	}
	if repo.called {
		t.Fatal("repository must not be called without email")
	}
}
