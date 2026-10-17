package user

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fakeProfileUpdateService struct {
	gotID    string
	gotInput ProfileUpdateInput
	user     User
	err      error
	calls    int
}

func (f *fakeProfileUpdateService) Update(
	_ context.Context,
	id string,
	input ProfileUpdateInput,
) (User, error) {
	f.calls++
	f.gotID = id
	f.gotInput = input
	return f.user, f.err
}

func newProfileHandler(service ProfileUpdateService) *ProfileHTTPHandler {
	return NewProfileHTTPHandler(service, func(r *http.Request) (string, bool) {
		id := r.Header.Get("X-Test-User-ID")
		return id, id != ""
	})
}

func TestProfileHTTPHandlerUpdateMeSuccess(t *testing.T) {
	service := &fakeProfileUpdateService{
		user: User{
			ID:        "user-123",
			Nickname:  "new-name",
			FirstName: "Alice",
			LastName:  "Smith",
			Role:      RoleUser,
			About:     "Hello",
			CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			UpdatedAt: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
		},
	}
	handler := newProfileHandler(service)

	req := httptest.NewRequest(
		http.MethodPatch,
		"/users/me",
		strings.NewReader(`{"nickname":"new-name","about":"Hello"}`),
	)
	req.Header.Set("X-Test-User-ID", "user-123")
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	handler.UpdateMe(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if service.calls != 1 {
		t.Fatalf("service calls = %d, want 1", service.calls)
	}
	if service.gotID != "user-123" {
		t.Fatalf("user ID = %q, want %q", service.gotID, "user-123")
	}
	if !service.gotInput.Nickname.Present || service.gotInput.Nickname.Value != "new-name" {
		t.Fatalf("unexpected nickname input: %+v", service.gotInput.Nickname)
	}

	var response map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if _, exists := response["password_hash"]; exists {
		t.Fatal("response must not contain password_hash")
	}
	if response["nickname"] != "new-name" {
		t.Fatalf("response nickname = %v, want new-name", response["nickname"])
	}
}

func TestProfileHTTPHandlerUpdateMeRejectsInvalidBody(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "invalid JSON", body: `{"nickname":`},
		{name: "unknown protected field", body: `{"role":9}`},
		{name: "email field", body: `{"email":"other@example.com"}`},
		{name: "password hash field", body: `{"password_hash":"x"}`},
		{name: "trailing JSON", body: `{"nickname":"valid"} {}`},
		{name: "empty body", body: ``},
		{name: "null body", body: `null`},
		{name: "array body", body: `[]`},
		{name: "invalid nickname", body: `{"nickname":"   "}`},
		{name: "null about", body: `{"about":null}`},
		{
			name: "multiple JSON values",
			body: `{"nickname":"valid"} true`,
		},
		{
			name: "body larger than limit",
			body: `{"about":"` + strings.Repeat("a", (1<<20)+1) + `"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &fakeProfileUpdateService{}
			handler := newProfileHandler(service)

			req := httptest.NewRequest(http.MethodPatch, "/users/me", strings.NewReader(tt.body))
			req.Header.Set("X-Test-User-ID", "user-123")
			rec := httptest.NewRecorder()

			handler.UpdateMe(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusBadRequest, rec.Body.String())
			}
			if service.calls != 0 {
				t.Fatalf("service calls = %d, want 0", service.calls)
			}
		})
	}
}

func TestProfileHTTPHandlerUpdateMeRejectsMissingIdentity(t *testing.T) {
	service := &fakeProfileUpdateService{}
	handler := newProfileHandler(service)

	req := httptest.NewRequest(http.MethodPatch, "/users/me", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	handler.UpdateMe(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if service.calls != 0 {
		t.Fatalf("service calls = %d, want 0", service.calls)
	}
}

func TestProfileHTTPHandlerUpdateMeMapsServiceErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "invalid user", err: ErrInvalidUser, want: http.StatusBadRequest},
		{name: "not found", err: ErrNotFound, want: http.StatusUnauthorized},
		{name: "nickname conflict", err: ErrConflict, want: http.StatusConflict},
		{name: "internal error", err: errors.New("database unavailable"), want: http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &fakeProfileUpdateService{err: tt.err}
			handler := newProfileHandler(service)

			req := httptest.NewRequest(http.MethodPatch, "/users/me", strings.NewReader(`{}`))
			req.Header.Set("X-Test-User-ID", "user-123")
			rec := httptest.NewRecorder()

			handler.UpdateMe(rec, req)

			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d; body: %s", rec.Code, tt.want, rec.Body.String())
			}
		})
	}
}

func TestProfileHTTPHandlerUpdateMeRejectsWrongMethod(t *testing.T) {
	service := &fakeProfileUpdateService{}
	handler := newProfileHandler(service)

	req := httptest.NewRequest(http.MethodPost, "/users/me", strings.NewReader(`{}`))
	req.Header.Set("X-Test-User-ID", "user-123")
	rec := httptest.NewRecorder()
	handler.UpdateMe(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
	if got := rec.Header().Get("Allow"); got != http.MethodPatch {
		t.Fatalf("Allow = %q, want %q", got, http.MethodPatch)
	}
	if service.calls != 0 {
		t.Fatalf("service calls = %d, want 0", service.calls)
	}
}
