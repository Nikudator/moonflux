package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"moonflux/internal/user"
)

type CurrentUserRepository interface {
	GetByID(ctx context.Context, id string) (user.User, error)
}

type MeHandler struct {
	users CurrentUserRepository
}

func NewMeHandler(users CurrentUserRepository) *MeHandler {
	return &MeHandler{users: users}
}

func (h *MeHandler) Me(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	u, err := h.users.GetByID(r.Context(), claims.Subject)
	if err != nil {
		if errors.Is(err, user.ErrNotFound) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		http.Error(w, "user lookup unavailable", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(u.ToPublic()); err != nil {
		return
	}
}
