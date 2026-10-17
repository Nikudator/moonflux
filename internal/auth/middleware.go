package auth

import (
	"context"
	"errors"
	"moonflux/internal/user"
	"net/http"
	"strings"
)

type contextKey string

const claimsContextKey contextKey = "auth_claims"

type TokenVersionProvider interface {
	GetTokenVersion(ctx context.Context, userID string) (int64, error)
}

func ClaimsFromContext(ctx context.Context) (Claims, bool) {
	claims, ok := ctx.Value(claimsContextKey).(Claims)
	return claims, ok
}

func (m *TokenManager) Middleware(
	provider TokenVersionProvider,
	next http.Handler,
) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		const prefix = "Bearer "

		if !strings.HasPrefix(header, prefix) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		token := strings.TrimSpace(strings.TrimPrefix(header, prefix))
		if token == "" || strings.ContainsAny(token, " \t\r\n") {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		claims, err := m.Validate(token)
		if err != nil {
			if errors.Is(err, ErrInvalidToken) || errors.Is(err, ErrExpiredToken) {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			http.Error(w, "authentication unavailable", http.StatusInternalServerError)
			return
		}

		currentVersion, err := provider.GetTokenVersion(r.Context(), claims.Subject)
		if err != nil {
			if errors.Is(err, user.ErrNotFound) {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			http.Error(w, "authentication unavailable", http.StatusInternalServerError)
			return
		}

		if claims.TokenVersion != currentVersion {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if err != nil {
			if errors.Is(err, ErrInvalidToken) || errors.Is(err, ErrExpiredToken) {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			http.Error(w, "authentication unavailable", http.StatusInternalServerError)
			return
		}

		ctx := context.WithValue(r.Context(), claimsContextKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
