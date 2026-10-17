package user

import (
	"errors"
	"testing"
)

func validTestUser() User {
	return User{
		ID:           "01900000-0000-7000-8000-000000000001",
		Nickname:     "moonflux",
		FirstName:    "Иван",
		LastName:     "Иванов",
		Role:         RoleUser,
		PasswordHash: "test-password-hash",
	}
}

func TestUserValidate(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*User)
		wantErr bool
	}{
		{
			name: "valid user",
		},
		{
			name: "empty nickname",
			modify: func(u *User) {
				u.Nickname = ""
			},
			wantErr: true,
		},
		{
			name: "nickname contains only spaces",
			modify: func(u *User) {
				u.Nickname = "   "
			},
			wantErr: true,
		},
		{
			name: "nickname too long",
			modify: func(u *User) {
				u.Nickname = string(make([]byte, 41))
			},
			wantErr: true,
		},
		{
			name: "empty first name",
			modify: func(u *User) {
				u.FirstName = " "
			},
			wantErr: true,
		},
		{
			name: "empty last name",
			modify: func(u *User) {
				u.LastName = ""
			},
			wantErr: true,
		},
		{
			name: "email too long",
			modify: func(u *User) {
				email := string(make([]byte, 255))
				u.Email = &email
			},
			wantErr: true,
		},
		{
			name: "patronymic too long",
			modify: func(u *User) {
				patronymic := string(make([]byte, 101))
				u.Patronymic = &patronymic
			},
			wantErr: true,
		},
		{
			name: "unsupported role",
			modify: func(u *User) {
				u.Role = 3
			},
			wantErr: true,
		},
		{
			name: "empty password hash",
			modify: func(u *User) {
				u.PasswordHash = ""
			},
			wantErr: true,
		},
		{
			name: "optional fields absent",
		},
		{
			name: "each supported role",
			modify: func(u *User) {
				u.Role = RoleRoot
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := validTestUser()
			if tt.modify != nil {
				tt.modify(&u)
			}

			err := u.Validate()
			if tt.wantErr && !errors.Is(err, ErrInvalidUser) {
				t.Fatalf("Validate() error = %v, want %v", err, ErrInvalidUser)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("Validate() error = %v, want nil", err)
			}
		})
	}
}

func TestValidRole(t *testing.T) {
	tests := []struct {
		role Role
		want bool
	}{
		{RoleGuest, true},
		{RoleUser, true},
		{RoleEditor, true},
		{RoleAdmin, true},
		{RoleRoot, true},
		{Role(2), false},
		{Role(-1), false},
	}

	for _, tt := range tests {
		t.Run(string(rune('0'+tt.role)), func(t *testing.T) {
			if got := validRole(tt.role); got != tt.want {
				t.Errorf("validRole(%d) = %v, want %v", tt.role, got, tt.want)
			}
		})
	}
}
func TestUserValidateFieldsDoesNotRequirePasswordHash(t *testing.T) {
	u := validTestUser()
	u.PasswordHash = ""

	if err := u.ValidateFields(); err != nil {
		t.Fatalf("ValidateFields() error = %v, want nil", err)
	}

	if err := u.Validate(); !errors.Is(err, ErrInvalidUser) {
		t.Fatalf("Validate() error = %v, want %v", err, ErrInvalidUser)
	}
}
