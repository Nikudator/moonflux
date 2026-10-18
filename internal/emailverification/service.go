package emailverification

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Service manages email verification tokens.
type Service struct {
	repo           Repository
	tokenTTL       time.Duration
	now            func() time.Time
	resendCooldown time.Duration
}

// NewService creates an email verification service.
func NewService(
	repo Repository,
	tokenTTL time.Duration,
	resendCooldown time.Duration,
) (*Service, error) {
	if repo == nil {
		return nil, fmt.Errorf("email verification repository is required")
	}
	if tokenTTL <= 0 {
		return nil, fmt.Errorf("email verification token TTL must be positive")
	}
	if resendCooldown < 0 {
		return nil, fmt.Errorf("email verification resend cooldown must not be negative")
	}

	return &Service{
		repo:           repo,
		tokenTTL:       tokenTTL,
		now:            time.Now,
		resendCooldown: resendCooldown,
	}, nil
}

// Issue creates a verification token and stores only its SHA-256 hash.
// The raw token must only be sent to the user's email address.
func (s *Service) Issue(
	ctx context.Context,
	userID string,
) (string, error) {
	if userID == "" {
		return "", ErrUserNotFound
	}

	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate email verification token: %w", err)
	}

	token := base64.RawURLEncoding.EncodeToString(raw[:])
	tokenHash := sha256.Sum256([]byte(token))
	now := s.now()
	expiresAt := now.Add(s.tokenTTL)

	if err := s.repo.Issue(
		ctx,
		userID,
		tokenHash[:],
		expiresAt,
		now,
		s.resendCooldown,
	); err != nil {
		return "", err
	}

	return token, nil
}

// Verify confirms an email using a raw token supplied by the user.
func (s *Service) Verify(
	ctx context.Context,
	token string,
) error {
	if token == "" {
		return ErrInvalidToken
	}

	tokenHash := sha256.Sum256([]byte(token))
	return s.repo.Verify(ctx, tokenHash[:], s.now())
}

// FindPendingUserByEmail finds an active user whose email is not verified.
func (s *Service) FindPendingUserByEmail(
	ctx context.Context,
	email string,
) (PendingUser, error) {
	email = strings.TrimSpace(email)
	if email == "" {
		return PendingUser{}, ErrUserNotFound
	}

	finder, ok := s.repo.(PendingUserFinder)
	if !ok {
		return PendingUser{}, errors.New("pending user lookup is not supported")
	}

	return finder.FindPendingUserByEmail(ctx, email)
}
