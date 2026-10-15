package user

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type passwordChangeHTTPServiceStub struct {
	err    error
	called bool
	userID string
	input  PasswordChangeInput
}

func (s *passwordChangeHTTPServiceStub) Change(
	_ context.Context,
	id string,
	input PasswordChangeInput,
) error {
	s.called = true
	s.userID = id
	s.input = input
	return s.err
}

func TestPasswordChangeHTTPHandlerChangeMe(t *testing.T) {
	tests := []struct {
		name          string
		method        string
		body          string
		serviceErr    error
		authenticated bool
		wantStatus    int
		wantCalled    bool
	}{
		{
			name:          "changes password",
			method:        http.MethodPatch,
			body:          `{"current_password":"Current-Password-123","new_password":"New-Password-456"}`,
			authenticated: true,
			wantStatus:    http.StatusNoContent,
			wantCalled:    true,
		},
		{
			name:          "rejects incorrect current password",
			method:        http.MethodPatch,
			body:          `{"current_password":"wrong-password","new_password":"New-Password-456"}`,
			serviceErr:    ErrInvalidCurrentPassword,
			authenticated: true,
			wantStatus:    http.StatusUnauthorized,
			wantCalled:    true,
		},
		{
			name:          "rejects missing password",
			method:        http.MethodPatch,
			body:          `{"current_password":"Current-Password-123"}`,
			authenticated: true,
			wantStatus:    http.StatusBadRequest,
		},
		{
			name:          "rejects unknown field",
			method:        http.MethodPatch,
			body:          `{"current_password":"Current-Password-123","new_password":"New-Password-456","role":9}`,
			authenticated: true,
			wantStatus:    http.StatusBadRequest,
		},
		{
			name:          "rejects malformed JSON",
			method:        http.MethodPatch,
			body:          `{"current_password":`,
			authenticated: true,
			wantStatus:    http.StatusBadRequest,
		},
		{
			name:       "rejects unauthenticated request",
			method:     http.MethodPatch,
			body:       `{"current_password":"Current-Password-123","new_password":"New-Password-456"}`,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:          "rejects other methods",
			method:        http.MethodPost,
			body:          `{}`,
			authenticated: true,
			wantStatus:    http.StatusMethodNotAllowed,
		},
		{
			name:          "hides internal errors",
			method:        http.MethodPatch,
			body:          `{"current_password":"Current-Password-123","new_password":"New-Password-456"}`,
			serviceErr:    errors.New("database failure"),
			authenticated: true,
			wantStatus:    http.StatusInternalServerError,
			wantCalled:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &passwordChangeHTTPServiceStub{}
			service.err = tt.serviceErr

			handler := NewPasswordChangeHTTPHandler(
				service,
				func(_ *http.Request) (string, bool) {
					return "user-123", tt.authenticated
				},
			)

			req := httptest.NewRequest(tt.method, "/users/me/password", strings.NewReader(tt.body))
			rec := httptest.NewRecorder()

			handler.ChangeMe(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body: %s",
					rec.Code, tt.wantStatus, rec.Body.String())
			}

			if service.called != tt.wantCalled {
				t.Fatalf("service called = %v, want %v", service.called, tt.wantCalled)
			}

			if !service.called {
				return
			}

			if service.userID != "user-123" {
				t.Errorf("user ID = %q, want %q", service.userID, "user-123")
			}

			if tt.name == "changes password" {
				if service.input.CurrentPassword != "Current-Password-123" {
					t.Error("current password was not passed correctly")
				}
				if service.input.NewPassword != "New-Password-456" {
					t.Error("new password was not passed correctly")
				}
			}
		})
	}
}
