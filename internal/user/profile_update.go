package user

import (
	"encoding/json"
	"strings"
	"time"
)

// Optional distinguishes an omitted JSON field from an explicit null.
type Optional[T any] struct {
	Present bool
	Null    bool
	Value   T
}

// UnmarshalJSON records whether a field was supplied and whether it was null.
func (o *Optional[T]) UnmarshalJSON(data []byte) error {
	o.Present = true

	if string(data) == "null" {
		o.Null = true
		var zero T
		o.Value = zero
		return nil
	}

	o.Null = false
	return json.Unmarshal(data, &o.Value)
}

// ProfileUpdateInput contains fields that a user may update in their profile.
type ProfileUpdateInput struct {
	Nickname   Optional[string]    `json:"nickname"`
	FirstName  Optional[string]    `json:"first_name"`
	LastName   Optional[string]    `json:"last_name"`
	Patronymic Optional[string]    `json:"patronymic"`
	BirthDate  Optional[time.Time] `json:"birth_date"`
	About      Optional[string]    `json:"about"`
}

// Validate checks the supplied profile fields.
func (in ProfileUpdateInput) Validate() error {
	if in.Nickname.Present {
		if in.Nickname.Null ||
			strings.TrimSpace(in.Nickname.Value) == "" ||
			len(in.Nickname.Value) > 40 {
			return ErrInvalidUser
		}
	}

	if in.FirstName.Present {
		if in.FirstName.Null ||
			strings.TrimSpace(in.FirstName.Value) == "" ||
			len(in.FirstName.Value) > 100 {
			return ErrInvalidUser
		}
	}

	if in.LastName.Present {
		if in.LastName.Null ||
			strings.TrimSpace(in.LastName.Value) == "" ||
			len(in.LastName.Value) > 100 {
			return ErrInvalidUser
		}
	}

	if in.Patronymic.Present && !in.Patronymic.Null &&
		len(in.Patronymic.Value) > 100 {
		return ErrInvalidUser
	}

	if in.About.Present &&
		(in.About.Null || len([]rune(in.About.Value)) > 2000) {
		return ErrInvalidUser
	}

	if in.BirthDate.Present && !in.BirthDate.Null &&
		in.BirthDate.Value.IsZero() {
		return ErrInvalidUser
	}

	return nil
}

// PasswordChangeInput contains the current and new passwords.
type PasswordChangeInput struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// Validate checks the password change input.
func (in PasswordChangeInput) Validate() error {
	if in.CurrentPassword == "" || in.NewPassword == "" {
		return ErrInvalidUser
	}

	if len([]rune(in.NewPassword)) < 8 {
		return ErrInvalidUser
	}

	return nil
}
