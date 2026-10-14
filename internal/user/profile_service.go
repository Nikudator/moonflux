package user

import (
	"context"
	"fmt"
	"strings"
)

// ProfileUpdater describes the repository operation required by the profile service.
type ProfileUpdater interface {
	UpdateProfile(ctx context.Context, id string, input ProfileUpdateInput) (User, error)
}

// ProfileService implements profile update business rules.
type ProfileService struct {
	repo ProfileUpdater
}

// NewProfileService creates a profile service.
func NewProfileService(repo ProfileUpdater) *ProfileService {
	return &ProfileService{repo: repo}
}

// Update updates the editable fields of an existing user's profile.
func (s *ProfileService) Update(
	ctx context.Context,
	id string,
	input ProfileUpdateInput,
) (User, error) {
	if strings.TrimSpace(id) == "" {
		return User{}, ErrInvalidUser
	}

	if input.Nickname.Present && !input.Nickname.Null {
		input.Nickname.Value = strings.TrimSpace(input.Nickname.Value)
	}

	if input.FirstName.Present && !input.FirstName.Null {
		input.FirstName.Value = strings.TrimSpace(input.FirstName.Value)
	}

	if input.LastName.Present && !input.LastName.Null {
		input.LastName.Value = strings.TrimSpace(input.LastName.Value)
	}

	if input.Patronymic.Present && !input.Patronymic.Null {
		value := strings.TrimSpace(input.Patronymic.Value)
		if value == "" {
			input.Patronymic.Null = true
			input.Patronymic.Value = ""
		} else {
			input.Patronymic.Value = value
		}
	}

	if err := input.Validate(); err != nil {
		return User{}, err
	}

	u, err := s.repo.UpdateProfile(ctx, id, input)
	if err != nil {
		return User{}, fmt.Errorf("update user profile: %w", err)
	}

	return u, nil
}
