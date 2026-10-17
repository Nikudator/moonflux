package user

import (
	"context"
	"fmt"
	"strings"
	"time"

	"moonflux/internal/platform/uuid"
)

// UserCreator describes the repository operation required by the service.
type UserCreator interface {
	Create(ctx context.Context, u User) error
}

// Service implements user-related business rules.
type Service struct {
	repo UserCreator
	now  func() time.Time
}

// NewService creates a user service.
func NewService(repo UserCreator) *Service {
	return &Service{
		repo: repo,
		now:  func() time.Time { return time.Now().UTC() },
	}
}

// RegisterInput contains fields accepted during public registration.
// Role and PasswordHash are intentionally absent.
type RegisterInput struct {
	Nickname   string
	Email      *string
	FirstName  string
	LastName   string
	Patronymic *string
	Password   string
}

// Register creates a regular user account.
func (s *Service) Register(ctx context.Context, input RegisterInput) (User, error) {
	input.Nickname = strings.TrimSpace(input.Nickname)
	input.FirstName = strings.TrimSpace(input.FirstName)
	input.LastName = strings.TrimSpace(input.LastName)

	if input.Email != nil {
		email := strings.TrimSpace(*input.Email)
		if email == "" {
			input.Email = nil
		} else {
			input.Email = &email
		}
	}

	if input.Patronymic != nil {
		patronymic := strings.TrimSpace(*input.Patronymic)
		if patronymic == "" {
			input.Patronymic = nil
		} else {
			input.Patronymic = &patronymic
		}
	}
	if err := (User{
		Nickname:   input.Nickname,
		Email:      input.Email,
		FirstName:  input.FirstName,
		LastName:   input.LastName,
		Patronymic: input.Patronymic,
		Role:       RoleUser,
	}).ValidateFields(); err != nil {
		return User{}, err
	}
	hash, err := HashPassword(input.Password)
	if err != nil {
		return User{}, fmt.Errorf("hash registration password: %w", err)
	}

	id, err := uuid.NewV7()
	if err != nil {
		return User{}, fmt.Errorf("generate user ID: %w", err)
	}

	now := s.now()
	u := User{
		ID:           uuid.String(id),
		Nickname:     input.Nickname,
		Email:        input.Email,
		FirstName:    input.FirstName,
		LastName:     input.LastName,
		Patronymic:   input.Patronymic,
		Role:         RoleUser,
		PasswordHash: hash,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := u.Validate(); err != nil {
		return User{}, err
	}

	if err := s.repo.Create(ctx, u); err != nil {
		return User{}, fmt.Errorf("register user: %w", err)
	}

	return u, nil
}
