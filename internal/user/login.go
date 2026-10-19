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
	repository           LoginRepository
	requireVerifiedEmail bool
	verifyPassword       func(password, hash string) (bool, error)
	dummyPasswordHash    string
}

// dummyPasswordHash is computed once at package init. It has the same
// Argon2id parameters as real hashes, so verifying a password against it
// costs the same amount of CPU as verifying against a real user's hash.
var dummyPasswordHash = func() string {
	h, err := HashPassword("moonflux-dummy-password-for-timing-equalization")
	if err != nil {
		panic("failed to precompute dummy password hash: " + err.Error())
	}
	return h
}()

func NewLoginService(
	repository LoginRepository,
	requireVerifiedEmail bool,
) *LoginService {
	return &LoginService{
		repository:           repository,
		requireVerifiedEmail: requireVerifiedEmail,
		verifyPassword:       VerifyPassword,
		dummyPasswordHash:    dummyPasswordHash,
	}
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
			// Equalize timing between existing and non-existing users:
			// run the same Argon2id verification against a dummy hash.
			_, _ = s.verifyPassword(input.Password, s.dummyPasswordHash)
			return User{}, ErrInvalidCredentials
		}
		return User{}, fmt.Errorf("find user for authentication: %w", err)
	}

	valid, err := s.verifyPassword(input.Password, u.PasswordHash)
	if err != nil {
		return User{}, fmt.Errorf("verify stored password hash: %w", err)
	}
	if !valid {
		return User{}, ErrInvalidCredentials
	}
	if s.requireVerifiedEmail && u.EmailVerifiedAt == nil {
		return User{}, ErrInvalidCredentials
	}

	return u, nil
}
