package user

import (
	"errors"
	netmail "net/mail"
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
	ID              string
	Nickname        string
	Email           *string
	EmailVerifiedAt *time.Time
	FirstName       string
	LastName        string
	Patronymic      *string
	Role            Role
	TokenVersion    int64
	BirthDate       *time.Time
	About           string
	PasswordHash    string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	DeletedAt       *time.Time
}

// Validate checks the fields required by the database schema.
func (u User) Validate() error {
	if err := u.ValidateFields(); err != nil {
		return err
	}

	if u.PasswordHash == "" {
		return ErrInvalidUser
	}

	return nil
}

// ValidateFields checks user fields independently of the password hash.
func (u User) ValidateFields() error {
	if strings.TrimSpace(u.Nickname) == "" ||
		len([]rune(u.Nickname)) > 40 {
		return ErrInvalidUser
	}

	if strings.TrimSpace(u.FirstName) == "" ||
		len([]rune(u.FirstName)) > 100 {
		return ErrInvalidUser
	}

	if strings.TrimSpace(u.LastName) == "" ||
		len([]rune(u.LastName)) > 100 {
		return ErrInvalidUser
	}

	if u.Email != nil {
		email := strings.TrimSpace(*u.Email)
		if len(email) > 254 {
			return ErrInvalidUser
		}
		parsed, err := netmail.ParseAddress(email)
		if err != nil || parsed.Address != email {
			return ErrInvalidUser
		}
	}

	if u.Patronymic != nil && len([]rune(*u.Patronymic)) > 100 {
		return ErrInvalidUser
	}

	if !validRole(u.Role) {
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
