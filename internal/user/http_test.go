package user

import (
	"context"
	"errors"
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
	handler := NewHTTPHandler(NewService(repo))

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
	handler := NewHTTPHandler(NewService(repo))

	req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(`{"nickname":`))
	rec := httptest.NewRecorder()
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
	handler := NewHTTPHandler(NewService(repo))

	body := `{
		"nickname": "moonflux",
		"first_name": "Nikolai",
		"last_name": "Example",
		"password": "some password",
		"role": 9
	}`
	req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(body))
	rec := httptest.NewRecorder()
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
	handler := NewHTTPHandler(NewService(repo))

	body := `{
		"nickname": "moonflux",
		"first_name": "Nikolai",
		"last_name": "Example",
		"password": "some password"
	}`
	req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusConflict, rec.Body.String())
	}
}

func TestRegisterHandlerHidesInternalErrors(t *testing.T) {
	repo := &httpTestCreator{err: errors.New("database credentials leaked")}
	handler := NewHTTPHandler(NewService(repo))

	body := `{
		"nickname": "moonflux",
		"first_name": "Nikolai",
		"last_name": "Example",
		"password": "some password"
	}`
	req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if strings.Contains(rec.Body.String(), "database credentials leaked") {
		t.Fatal("response exposes internal error")
	}
}
