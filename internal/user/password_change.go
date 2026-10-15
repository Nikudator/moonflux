package user

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var ErrInvalidCurrentPassword = errors.New("invalid current password")

// PasswordChanger describes the repository operation for changing a password.
type PasswordChanger interface {
	GetByID(ctx context.Context, id string) (User, error)
	ChangePassword(ctx context.Context, id, passwordHash string) error
}

// PasswordChangeService implements password change business rules.
type PasswordChangeService struct {
	repo PasswordChanger
}

// NewPasswordChangeService creates a password change service.
func NewPasswordChangeService(repo PasswordChanger) *PasswordChangeService {
	return &PasswordChangeService{repo: repo}
}

// Change verifies the current password and stores a hash of the new password.
func (s *PasswordChangeService) Change(
	ctx context.Context,
	id string,
	input PasswordChangeInput,
) error {
	if strings.TrimSpace(id) == "" {
		return ErrInvalidUser
	}

	if err := input.Validate(); err != nil {
		return err
	}

	u, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("get user for password change: %w", err)
	}

	valid, err := VerifyPassword(input.CurrentPassword, u.PasswordHash)
	if err != nil {
		return fmt.Errorf("verify current password: %w", err)
	}
	if !valid {
		return ErrInvalidCurrentPassword
	}

	passwordHash, err := HashPassword(input.NewPassword)
	if err != nil {
		return fmt.Errorf("hash new password: %w", err)
	}

	if err := s.repo.ChangePassword(ctx, id, passwordHash); err != nil {
		return fmt.Errorf("change user password: %w", err)
	}

	return nil
}
