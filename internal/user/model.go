package user

import (
	"errors"
	"strings"
	"time"
)

// Role represents a user's authorization level.
type Role int16

const (
	RoleGuest  Role = 0
	RoleUser   Role = 1
	RoleEditor Role = 5
	RoleAdmin  Role = 7
	RoleRoot   Role = 9
)

var (
	ErrInvalidUser = errors.New("invalid user")
	ErrNotFound    = errors.New("user not found")
	ErrConflict    = errors.New("user already exists")
)

// User represents an account stored in PostgreSQL.
// PasswordHash must never be included in public API responses.
type User struct {
	ID           string
	Nickname     string
	Email        *string
	FirstName    string
	LastName     string
	Patronymic   *string
	Role         Role
	TokenVersion int64
	BirthDate    *time.Time
	About        string
	PasswordHash string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DeletedAt    *time.Time
}

// Validate checks the fields required by the database schema.
func (u User) Validate() error {
	if strings.TrimSpace(u.Nickname) == "" ||
		len(u.Nickname) > 40 {
		return ErrInvalidUser
	}

	if strings.TrimSpace(u.FirstName) == "" ||
		len(u.FirstName) > 100 {
		return ErrInvalidUser
	}

	if strings.TrimSpace(u.LastName) == "" ||
		len(u.LastName) > 100 {
		return ErrInvalidUser
	}

	if u.Email != nil && len(*u.Email) > 254 {
		return ErrInvalidUser
	}

	if u.Patronymic != nil && len(*u.Patronymic) > 100 {
		return ErrInvalidUser
	}

	if !validRole(u.Role) {
		return ErrInvalidUser
	}

	if u.PasswordHash == "" {
		return ErrInvalidUser
	}

	return nil
}

func validRole(role Role) bool {
	switch role {
	case RoleGuest, RoleUser, RoleEditor, RoleAdmin, RoleRoot:
		return true
	default:
		return false
	}
}
