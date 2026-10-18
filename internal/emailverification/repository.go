package emailverification

import (
	"context"
	"errors"
	"time"
)

var (
	ErrInvalidToken   = errors.New("invalid or expired verification token")
	ErrEmailVerified  = errors.New("email is already verified")
	ErrUserNotFound   = errors.New("user not found")
	ErrResendCooldown = errors.New("email verification resend cooldown has not elapsed")
)

// PendingUser contains the minimal data needed to resend verification email.
type PendingUser struct {
	ID    string
	Email string
}

// Repository provides access to email verification tokens.
type Repository interface {
	// Issue replaces any previous active token for the user.
	Issue(
		ctx context.Context,
		userID string,
		tokenHash []byte,
		expiresAt time.Time,
		now time.Time,
		resendCooldown time.Duration,
	) error

	// Verify marks the token as used and verifies the user's email atomically.
	Verify(
		ctx context.Context,
		tokenHash []byte,
		now time.Time,
	) error
}

type PendingUserFinder interface {
	FindPendingUserByEmail(
		ctx context.Context,
		email string,
	) (PendingUser, error)
}
