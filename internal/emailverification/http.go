package emailverification

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
)

type EmailVerifier interface {
	Verify(ctx context.Context, token string) error
}

type VerificationResender interface {
	Resend(ctx context.Context, email string) error
}

// HTTPHandler handles email verification HTTP requests.
type HTTPHandler struct {
	service  EmailVerifier
	resender VerificationResender
	logger   *slog.Logger
}

// NewHTTPHandlerWithResender creates an HTTP handler with resend support.
func NewHTTPHandlerWithResender(
	service EmailVerifier,
	resender VerificationResender,
) *HTTPHandler {
	return &HTTPHandler{
		service:  service,
		resender: resender,
	}
}

// NewHTTPHandlerWithResenderAndLogger creates a handler with resend support and logging.
func NewHTTPHandlerWithResenderAndLogger(
	service EmailVerifier,
	resender VerificationResender,
	logger *slog.Logger,
) *HTTPHandler {
	return &HTTPHandler{
		service:  service,
		resender: resender,
		logger:   logger,
	}
}

// NewHTTPHandler creates an HTTP handler for email verification.
func NewHTTPHandler(service EmailVerifier) *HTTPHandler {
	return &HTTPHandler{service: service}
}

type verifyEmailRequest struct {
	Token string `json:"token"`
}

type verifyEmailResponse struct {
	Status string `json:"status"`
}

func (h *HTTPHandler) HandleVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	var request verifyEmailRequest
	if err := decoder.Decode(&request); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	if request.Token == "" {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	if err := h.service.Verify(r.Context(), request.Token); err != nil {
		if errors.Is(err, ErrInvalidToken) {
			http.Error(w, "invalid or expired verification token", http.StatusBadRequest)
			return
		}

		if errors.Is(err, ErrEmailVerified) {
			http.Error(w, "email already verified", http.StatusConflict)
			return
		}

		http.Error(w, "unable to verify email", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(verifyEmailResponse{Status: "verified"})
}

type resendVerificationRequest struct {
	Email string `json:"email"`
}

type resendVerificationResponse struct {
	Status string `json:"status"`
}

func (h *HTTPHandler) HandleResend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.resender == nil {
		http.Error(w, "resend is unavailable", http.StatusInternalServerError)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	var request resendVerificationRequest
	if err := decoder.Decode(&request); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(request.Email) == "" {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	if err := h.resender.Resend(r.Context(), request.Email); err != nil {
		if h.logger != nil {
			h.logger.ErrorContext(
				r.Context(),
				"email verification resend failed",
				"event", "email.verification.resend_failed",
				"request_id", r.Header.Get("X-Request-ID"),
			)
		}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(
		resendVerificationResponse{
			Status: "if the address is eligible, a verification email will be sent",
		},
	)
}
