package user

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var ErrInvalidCredentials = errors.New("invalid credentials")

type LoginInput struct {
	Login    string
	Password string
}

type LoginRepository interface {
	GetByLogin(ctx context.Context, login string) (User, error)
}

type LoginService struct {
	repository LoginRepository
}

func NewLoginService(repository LoginRepository) *LoginService {
	return &LoginService{repository: repository}
}

// Authenticate validates the login and password.
// It returns the same public error for an unknown account and a wrong password.
func (s *LoginService) Authenticate(
	ctx context.Context,
	input LoginInput,
) (User, error) {
	if strings.TrimSpace(input.Login) == "" || input.Password == "" {
		return User{}, ErrInvalidCredentials
	}

	u, err := s.repository.GetByLogin(ctx, strings.TrimSpace(input.Login))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return User{}, ErrInvalidCredentials
		}
		return User{}, fmt.Errorf("find user for authentication: %w", err)
	}

	valid, err := VerifyPassword(input.Password, u.PasswordHash)
	if err != nil {
		return User{}, fmt.Errorf("verify stored password hash: %w", err)
	}
	if !valid {
		return User{}, ErrInvalidCredentials
	}

	return u, nil
}
