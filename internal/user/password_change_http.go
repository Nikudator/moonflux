package user

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

type PasswordChangeHTTPService interface {
	Change(ctx context.Context, id string, input PasswordChangeInput) error
}

type PasswordChangeHTTPHandler struct {
	service PasswordChangeHTTPService
	userID  func(*http.Request) (string, bool)
}

func NewPasswordChangeHTTPHandler(
	service PasswordChangeHTTPService,
	userID func(*http.Request) (string, bool),
) *PasswordChangeHTTPHandler {
	return &PasswordChangeHTTPHandler{
		service: service,
		userID:  userID,
	}
}

func (h *PasswordChangeHTTPHandler) ChangeMe(
	w http.ResponseWriter,
	r *http.Request,
) {
	if r.Method != http.MethodPatch {
		w.Header().Set("Allow", http.MethodPatch)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id, ok := h.userID(r)
	if !ok || strings.TrimSpace(id) == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	body = []byte(strings.TrimSpace(string(body)))
	if len(body) == 0 || body[0] != '{' {
		http.Error(w, "request body must be a JSON object", http.StatusBadRequest)
		return
	}

	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()

	var input PasswordChangeInput
	if err := decoder.Decode(&input); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if err := input.Validate(); err != nil {
		http.Error(w, "invalid password fields", http.StatusBadRequest)
		return
	}

	if err := h.service.Change(r.Context(), id, input); err != nil {
		switch {
		case errors.Is(err, ErrInvalidUser):
			http.Error(w, "invalid password fields", http.StatusBadRequest)
		case errors.Is(err, ErrInvalidCurrentPassword):
			http.Error(w, "invalid current password", http.StatusUnauthorized)
		case errors.Is(err, ErrNotFound):
			http.Error(w, "unauthorized", http.StatusUnauthorized)
		default:
			http.Error(w, "password change unavailable", http.StatusInternalServerError)
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
