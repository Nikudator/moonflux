package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"moonflux/internal/user"
)

type fakeAuthenticator struct {
	user user.User
	err  error
}

func (f *fakeAuthenticator) Authenticate(
	_ context.Context,
	_ user.LoginInput,
) (user.User, error) {
	return f.user, f.err
}

func TestLoginServiceIssuesAccessToken(t *testing.T) {
	now := time.Date(2026, time.October, 14, 12, 0, 0, 0, time.UTC)
	manager, err := NewTokenManager(TokenConfig{
		Issuer: "moonflux",
		Secret: []byte("test-secret-that-is-at-least-32-bytes-long"),
		TTL:    15 * time.Minute,
		Now:    func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}

	authenticator := &fakeAuthenticator{
		user: user.User{
			ID:           "user-123",
			Role:         user.RoleEditor,
			TokenVersion: 1,
		},
	}
	service, err := NewLoginService(authenticator, manager)
	if err != nil {
		t.Fatal(err)
	}

	result, err := service.Login(context.Background(), user.LoginInput{
		Login:    "moonflux",
		Password: "correct-password",
	})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if result.AccessToken == "" {
		t.Fatal("access token is empty")
	}
	if result.TokenType != "Bearer" {
		t.Errorf("TokenType = %q, want Bearer", result.TokenType)
	}
	if result.ExpiresIn != 900 {
		t.Errorf("ExpiresIn = %d, want 900", result.ExpiresIn)
	}

	claims, err := manager.Validate(result.AccessToken)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if claims.Subject != "user-123" {
		t.Errorf("Subject = %q, want user-123", claims.Subject)
	}
	if claims.Role != int(user.RoleEditor) {
		t.Errorf("Role = %d, want %d", claims.Role, user.RoleEditor)
	}
}

func TestLoginServiceReturnsAuthenticationError(t *testing.T) {
	expectedErr := user.ErrInvalidCredentials
	authenticator := &fakeAuthenticator{err: expectedErr}

	manager, err := NewTokenManager(TokenConfig{
		Issuer: "moonflux",
		Secret: []byte("test-secret-that-is-at-least-32-bytes-long"),
		TTL:    time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}

	service, err := NewLoginService(authenticator, manager)
	if err != nil {
		t.Fatal(err)
	}

	_, err = service.Login(context.Background(), user.LoginInput{
		Login:    "moonflux",
		Password: "wrong-password",
	})
	if !errors.Is(err, expectedErr) {
		t.Fatalf("Login() error = %v, want ErrInvalidCredentials", err)
	}
}
