package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"moonflux/internal/user"
)

type fakeCurrentUserRepository struct {
	user user.User
	err  error
	id   string
}

func (f *fakeCurrentUserRepository) GetByID(_ context.Context, id string) (user.User, error) {
	f.id = id
	return f.user, f.err
}

func TestMeHandlerReturnsPublicUser(t *testing.T) {
	email := "test@example.com"
	repo := &fakeCurrentUserRepository{
		user: user.User{
			ID:           "test-user-id",
			Nickname:     "test",
			Email:        &email,
			FirstName:    "Test",
			LastName:     "User",
			Role:         user.RoleUser,
			PasswordHash: "must-not-be-exposed",
			CreatedAt:    time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			UpdatedAt:    time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		},
	}
	handler := NewMeHandler(repo)

	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	ctx := context.WithValue(req.Context(), claimsContextKey, Claims{
		Subject: "test-user-id",
	})
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	handler.Me(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s",
			rec.Code, http.StatusOK, rec.Body.String())
	}
	if repo.id != "test-user-id" {
		t.Errorf("GetByID id = %q", repo.id)
	}
	if strings.Contains(rec.Body.String(), "must-not-be-exposed") ||
		strings.Contains(rec.Body.String(), "password_hash") {
		t.Fatal("response exposed password hash")
	}

	var response user.PublicUser
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.ID != "test-user-id" || response.Nickname != "test" {
		t.Errorf("unexpected public user: %+v", response)
	}
}

func TestMeHandlerRequiresClaims(t *testing.T) {
	repo := &fakeCurrentUserRepository{}
	handler := NewMeHandler(repo)

	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	rec := httptest.NewRecorder()
	handler.Me(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if repo.id != "" {
		t.Errorf("repository called without claims, id = %q", repo.id)
	}
}

func TestMeHandlerMapsMissingUserToUnauthorized(t *testing.T) {
	repo := &fakeCurrentUserRepository{err: user.ErrNotFound}
	handler := NewMeHandler(repo)

	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	req = req.WithContext(context.WithValue(req.Context(), claimsContextKey, Claims{
		Subject: "deleted-user-id",
	}))
	rec := httptest.NewRecorder()
	handler.Me(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestMeHandlerMapsRepositoryErrorToInternalServerError(t *testing.T) {
	repo := &fakeCurrentUserRepository{err: errors.New("database unavailable")}
	handler := NewMeHandler(repo)

	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	req = req.WithContext(context.WithValue(req.Context(), claimsContextKey, Claims{
		Subject: "test-user-id",
	}))
	rec := httptest.NewRecorder()
	handler.Me(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if strings.Contains(rec.Body.String(), "database unavailable") {
		t.Fatal("response exposed internal error")
	}
}

func TestMeHandlerRejectsWrongMethod(t *testing.T) {
	handler := NewMeHandler(&fakeCurrentUserRepository{})
	req := httptest.NewRequest(http.MethodPost, "/auth/me", nil)
	rec := httptest.NewRecorder()
	handler.Me(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
	if got := rec.Header().Get("Allow"); got != http.MethodGet {
		t.Errorf("Allow = %q, want GET", got)
	}
}
