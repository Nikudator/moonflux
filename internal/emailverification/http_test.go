package emailverification

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

type fakeEmailVerifier struct {
	err   error
	token string
	calls int
}

func (f *fakeEmailVerifier) Verify(_ context.Context, token string) error {
	f.calls++
	f.token = token
	return f.err
}

type fakeVerificationResender struct {
	email string
	err   error
	calls int
}

func (f *fakeVerificationResender) Resend(
	_ context.Context,
	email string,
) error {
	f.calls++
	f.email = email
	return f.err
}
func TestHandleVerifySuccess(t *testing.T) {
	const token = "test-verification-token"

	service := &fakeEmailVerifier{}
	handler := NewHTTPHandler(service)

	request := httptest.NewRequest(
		http.MethodPost,
		"/auth/verify-email",
		strings.NewReader(`{"token":"`+token+`"}`),
	)
	recorder := httptest.NewRecorder()

	handler.HandleVerify(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if service.calls != 1 {
		t.Fatalf("expected Verify to be called once, got %d", service.calls)
	}
	if service.token != token {
		t.Fatal("handler passed an unexpected token to the service")
	}
	if strings.Contains(recorder.Body.String(), token) {
		t.Fatal("response must not contain the verification token")
	}
}

func TestHandleVerifyInvalidToken(t *testing.T) {
	service := &fakeEmailVerifier{err: ErrInvalidToken}
	handler := NewHTTPHandler(service)

	request := httptest.NewRequest(
		http.MethodPost,
		"/auth/verify-email",
		strings.NewReader(`{"token":"invalid-token"}`),
	)
	recorder := httptest.NewRecorder()

	handler.HandleVerify(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, recorder.Code)
	}
}

func TestHandleVerifyInternalError(t *testing.T) {
	service := &fakeEmailVerifier{err: errors.New("database failure")}
	handler := NewHTTPHandler(service)

	request := httptest.NewRequest(
		http.MethodPost,
		"/auth/verify-email",
		strings.NewReader(`{"token":"test-token"}`),
	)
	recorder := httptest.NewRecorder()

	handler.HandleVerify(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, recorder.Code)
	}
	if strings.Contains(recorder.Body.String(), "database failure") {
		t.Fatal("response must not expose internal errors")
	}
}

func TestHandleVerifyRejectsWrongMethod(t *testing.T) {
	service := &fakeEmailVerifier{}
	handler := NewHTTPHandler(service)

	request := httptest.NewRequest(http.MethodGet, "/auth/verify-email", nil)
	recorder := httptest.NewRecorder()

	handler.HandleVerify(recorder, request)

	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status %d, got %d", http.StatusMethodNotAllowed, recorder.Code)
	}
	if service.calls != 0 {
		t.Fatal("Verify must not be called for an unsupported method")
	}
}

func TestHandleResendSuccess(t *testing.T) {
	const email = "user@example.com"

	resender := &fakeVerificationResender{}
	handler := NewHTTPHandlerWithResender(
		&fakeEmailVerifier{},
		resender,
	)

	request := httptest.NewRequest(
		http.MethodPost,
		"/auth/resend-verification",
		strings.NewReader(`{"email":"`+email+`"}`),
	)
	recorder := httptest.NewRecorder()

	handler.HandleResend(recorder, request)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, recorder.Code)
	}
	if resender.calls != 1 {
		t.Fatalf("expected Resend to be called once, got %d", resender.calls)
	}
	if resender.email != email {
		t.Fatal("handler passed an unexpected email to the service")
	}
}

func TestHandleResendMasksIneligibleAddress(t *testing.T) {
	serviceErrors := []error{
		ErrUserNotFound,
		ErrEmailVerified,
		ErrResendCooldown,
	}

	var expectedBody string

	for i, serviceErr := range serviceErrors {
		resender := &fakeVerificationResender{err: serviceErr}
		handler := NewHTTPHandlerWithResender(
			&fakeEmailVerifier{},
			resender,
		)

		request := httptest.NewRequest(
			http.MethodPost,
			"/auth/resend-verification",
			strings.NewReader(`{"email":"user@example.com"}`),
		)
		recorder := httptest.NewRecorder()

		handler.HandleResend(recorder, request)

		if recorder.Code != http.StatusAccepted {
			t.Fatalf(
				"case %d: expected status %d, got %d",
				i,
				http.StatusAccepted,
				recorder.Code,
			)
		}

		body := recorder.Body.String()
		if i == 0 {
			expectedBody = body
		} else if body != expectedBody {
			t.Fatalf("case %d: response differs for an ineligible address", i)
		}
	}
}

func TestHandleResendSameResponseOnSuccessAndFailure(t *testing.T) {
	responses := []struct {
		name string
		err  error
	}{
		{name: "success"},
		{name: "SMTP failure", err: errors.New("SMTP failure")},
	}

	var expectedStatus int
	var expectedBody string

	for i, tt := range responses {
		t.Run(tt.name, func(t *testing.T) {
			resender := &fakeVerificationResender{err: tt.err}
			handler := NewHTTPHandlerWithResender(
				&fakeEmailVerifier{},
				resender,
			)

			request := httptest.NewRequest(
				http.MethodPost,
				"/auth/resend-verification",
				strings.NewReader(`{"email":"user@example.com"}`),
			)
			recorder := httptest.NewRecorder()

			handler.HandleResend(recorder, request)

			if resender.calls != 1 {
				t.Fatalf("expected Resend to be called once, got %d", resender.calls)
			}

			if i == 0 {
				expectedStatus = recorder.Code
				expectedBody = recorder.Body.String()
				return
			}

			if recorder.Code != expectedStatus {
				t.Fatalf(
					"status differs: expected %d, got %d",
					expectedStatus,
					recorder.Code,
				)
			}
			if recorder.Body.String() != expectedBody {
				t.Fatal("response body differs between success and SMTP failure")
			}
			if strings.Contains(recorder.Body.String(), "SMTP failure") {
				t.Fatal("response must not expose internal errors")
			}
		})
	}
}

func TestHandleResendInternalError(t *testing.T) {
	resender := &fakeVerificationResender{
		err: errors.New("SMTP credentials leaked"),
	}
	handler := NewHTTPHandlerWithResender(
		&fakeEmailVerifier{},
		resender,
	)

	request := httptest.NewRequest(
		http.MethodPost,
		"/auth/resend-verification",
		strings.NewReader(`{"email":"user@example.com"}`),
	)
	recorder := httptest.NewRecorder()

	handler.HandleResend(recorder, request)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf(
			"expected status %d, got %d",
			http.StatusAccepted,
			recorder.Code,
		)
	}
	if strings.Contains(recorder.Body.String(), "SMTP credentials leaked") {
		t.Fatal("response must not expose internal errors")
	}
}

func TestHandleResendRejectsInvalidRequests(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "invalid JSON", body: `{`},
		{name: "unknown field", body: `{"email":"user@example.com","token":"secret"}`},
		{name: "empty email", body: `{"email":"  "}`},
		{name: "trailing JSON", body: `{"email":"user@example.com"} {}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resender := &fakeVerificationResender{}
			handler := NewHTTPHandlerWithResender(
				&fakeEmailVerifier{},
				resender,
			)

			request := httptest.NewRequest(
				http.MethodPost,
				"/auth/resend-verification",
				strings.NewReader(tt.body),
			)
			recorder := httptest.NewRecorder()

			handler.HandleResend(recorder, request)

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf(
					"expected status %d, got %d",
					http.StatusBadRequest,
					recorder.Code,
				)
			}
			if resender.calls != 0 {
				t.Fatal("Resend must not be called for an invalid request")
			}
		})
	}
}

func TestHandleResendRejectsWrongMethod(t *testing.T) {
	resender := &fakeVerificationResender{}
	handler := NewHTTPHandlerWithResender(
		&fakeEmailVerifier{},
		resender,
	)

	request := httptest.NewRequest(
		http.MethodGet,
		"/auth/resend-verification",
		nil,
	)
	recorder := httptest.NewRecorder()

	handler.HandleResend(recorder, request)

	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf(
			"expected status %d, got %d",
			http.StatusMethodNotAllowed,
			recorder.Code,
		)
	}
	if resender.calls != 0 {
		t.Fatal("Resend must not be called for an unsupported method")
	}
}

func TestHandleResendLogsSafeError(t *testing.T) {
	const (
		email     = "private-user@example.com"
		requestID = "test-request-id"
		secret    = "SMTP credentials leaked"
	)

	var logOutput bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logOutput, nil))

	resender := &fakeVerificationResender{
		err: errors.New(secret),
	}
	handler := NewHTTPHandlerWithResenderAndLogger(
		&fakeEmailVerifier{},
		resender,
		logger,
	)

	request := httptest.NewRequest(
		http.MethodPost,
		"/auth/resend-verification",
		strings.NewReader(`{"email":"`+email+`"}`),
	)
	request.Header.Set("X-Request-ID", requestID)

	recorder := httptest.NewRecorder()
	handler.HandleResend(recorder, request)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf(
			"expected status %d, got %d",
			http.StatusAccepted,
			recorder.Code,
		)
	}

	logged := logOutput.String()

	for _, expected := range []string{
		"email.verification.resend_failed",
		requestID,
	} {
		if !strings.Contains(logged, expected) {
			t.Errorf("log does not contain expected safe field %q", expected)
		}
	}

	for _, forbidden := range []string{email, secret} {
		if strings.Contains(logged, forbidden) {
			t.Errorf("log contains sensitive value %q", forbidden)
		}
	}
}
