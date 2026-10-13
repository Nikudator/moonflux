package auth

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func newTestTokenManager(t *testing.T, now time.Time) *TokenManager {
	t.Helper()

	manager, err := NewTokenManager(TokenConfig{
		Issuer: "moonflux",
		Secret: []byte("test-secret-that-is-at-least-32-bytes-long"),
		TTL:    15 * time.Minute,
		Now:    func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewTokenManager() error = %v", err)
	}
	return manager
}

func TestTokenIssueAndValidate(t *testing.T) {
	now := time.Date(2026, time.October, 14, 12, 0, 0, 0, time.UTC)
	manager := newTestTokenManager(t, now)

	token, err := manager.Issue("user-123", 1)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	claims, err := manager.Validate(token)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	if claims.Subject != "user-123" {
		t.Errorf("Subject = %q, want user-123", claims.Subject)
	}
	if claims.Issuer != "moonflux" {
		t.Errorf("Issuer = %q, want moonflux", claims.Issuer)
	}
	if claims.ExpiresAt-claims.IssuedAt != int64((15*time.Minute)/time.Second) {
		t.Errorf("unexpected token lifetime: %d seconds", claims.ExpiresAt-claims.IssuedAt)
	}
	if claims.Role != 1 {
		t.Errorf("Role = %d, want 1", claims.Role)
	}
}

func TestTokenRejectsTampering(t *testing.T) {
	now := time.Date(2026, time.October, 14, 12, 0, 0, 0, time.UTC)
	manager := newTestTokenManager(t, now)

	token, err := manager.Issue("user-123", 1)
	if err != nil {
		t.Fatal(err)
	}

	tampered := token[:len(token)-1]
	if token[len(token)-1:] != "A" {
		tampered += "A"
	} else {
		tampered += "B"
	}

	if _, err := manager.Validate(tampered); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("Validate() error = %v, want ErrInvalidToken", err)
	}
}

func TestTokenRejectsMalformedTokens(t *testing.T) {
	manager := newTestTokenManager(t, time.Now().UTC())

	for _, token := range []string{
		"",
		"one.two",
		"one.two.three",
		"e30.e30.",
		strings.Repeat("x", 100),
	} {
		t.Run(token, func(t *testing.T) {
			if _, err := manager.Validate(token); err == nil {
				t.Fatal("Validate() accepted malformed token")
			}
		})
	}
}

func TestTokenExpires(t *testing.T) {
	now := time.Date(2026, time.October, 14, 12, 0, 0, 0, time.UTC)
	manager := newTestTokenManager(t, now)

	token, err := manager.Issue("user-123", 1)
	if err != nil {
		t.Fatal(err)
	}

	manager.now = func() time.Time { return now.Add(15 * time.Minute) }

	if _, err := manager.Validate(token); !errors.Is(err, ErrExpiredToken) {
		t.Fatalf("Validate() error = %v, want ErrExpiredToken", err)
	}
}

func TestTokenRejectsEmptySubject(t *testing.T) {
	manager := newTestTokenManager(t, time.Now().UTC())

	if _, err := manager.Issue(" ", 1); err == nil {
		t.Fatal("Issue() accepted an empty subject")
	}
}

func TestTokenManagerRejectsWeakConfiguration(t *testing.T) {
	_, err := NewTokenManager(TokenConfig{
		Issuer: "moonflux",
		Secret: []byte("too-short"),
		TTL:    time.Minute,
	})
	if err == nil {
		t.Fatal("NewTokenManager() accepted a weak secret")
	}
}
func TestTokenRejectsNegativeRole(t *testing.T) {
	manager := newTestTokenManager(t, time.Now().UTC())

	if _, err := manager.Issue("user-123", -1); err == nil {
		t.Fatal("Issue() accepted a negative role")
	}
}
