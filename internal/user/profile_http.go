package user

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

type ProfileUpdateService interface {
	Update(ctx context.Context, id string, input ProfileUpdateInput) (User, error)
}

type ProfileHTTPHandler struct {
	service ProfileUpdateService
	userID  func(*http.Request) (string, bool)
}

func NewProfileHTTPHandler(
	service ProfileUpdateService,
	userID func(*http.Request) (string, bool),
) *ProfileHTTPHandler {
	return &ProfileHTTPHandler{
		service: service,
		userID:  userID,
	}
}

func (h *ProfileHTTPHandler) UpdateMe(w http.ResponseWriter, r *http.Request) {
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

	var input ProfileUpdateInput
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
		http.Error(w, "invalid profile fields", http.StatusBadRequest)
		return
	}

	u, err := h.service.Update(r.Context(), id, input)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidUser):
			http.Error(w, "invalid profile fields", http.StatusBadRequest)
		case errors.Is(err, ErrNotFound):
			http.Error(w, "user not found", http.StatusUnauthorized)
		case errors.Is(err, ErrConflict):
			http.Error(w, "nickname already exists", http.StatusConflict)
		default:
			http.Error(w, "profile update unavailable", http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(u.ToPublic()); err != nil {
		return
	}
}
