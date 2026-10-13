package user

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type fakeUserCreator struct {
	created User
	err     error
	calls   int
}

func (f *fakeUserCreator) Create(_ context.Context, u User) error {
	f.calls++
	f.created = u
	return f.err
}

func TestServiceRegister(t *testing.T) {
	repo := &fakeUserCreator{}
	service := NewService(repo)

	input := RegisterInput{
		Nickname:  "  moonflux  ",
		FirstName: "  Иван ",
		LastName:  " Иванов  ",
		Password:  "a-strong-test-password",
	}

	email := "  user@example.com  "
	patronymic := "  Иванович  "
	input.Email = &email
	input.Patronymic = &patronymic

	u, err := service.Register(context.Background(), input)
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if repo.calls != 1 {
		t.Fatalf("repository Create calls = %d, want 1", repo.calls)
	}

	if u.ID == "" {
		t.Fatal("user ID is empty")
	}
	if u.Nickname != "moonflux" {
		t.Errorf("Nickname = %q, want moonflux", u.Nickname)
	}
	if u.FirstName != "Иван" {
		t.Errorf("FirstName = %q, want Иван", u.FirstName)
	}
	if u.LastName != "Иванов" {
		t.Errorf("LastName = %q, want Иванов", u.LastName)
	}
	if u.Email == nil || *u.Email != "user@example.com" {
		t.Errorf("Email = %v, want user@example.com", u.Email)
	}
	if u.Patronymic == nil || *u.Patronymic != "Иванович" {
		t.Errorf("Patronymic = %v, want Иванович", u.Patronymic)
	}
	if u.Role != RoleUser {
		t.Errorf("Role = %d, want RoleUser", u.Role)
	}
	if u.PasswordHash == input.Password {
		t.Fatal("password was stored without hashing")
	}
	if _, err := VerifyPassword(input.Password, u.PasswordHash); err != nil {
		t.Fatalf("stored password hash is invalid: %v", err)
	}
	if repo.created.ID != u.ID || repo.created.PasswordHash != u.PasswordHash {
		t.Fatal("repository did not receive the registered user")
	}
}

func TestServiceRegisterRejectsInvalidInput(t *testing.T) {
	repo := &fakeUserCreator{}
	service := NewService(repo)

	_, err := service.Register(context.Background(), RegisterInput{
		Nickname:  " ",
		FirstName: "Иван",
		LastName:  "Иванов",
		Password:  "password",
	})
	if !errors.Is(err, ErrInvalidUser) {
		t.Fatalf("Register() error = %v, want ErrInvalidUser", err)
	}

	if repo.calls != 0 {
		t.Fatalf("repository Create calls = %d, want 0", repo.calls)
	}
}

func TestServiceRegisterRepositoryError(t *testing.T) {
	repositoryErr := errors.New("database unavailable")
	repo := &fakeUserCreator{err: repositoryErr}
	service := NewService(repo)

	_, err := service.Register(context.Background(), RegisterInput{
		Nickname:  "moonflux",
		FirstName: "Иван",
		LastName:  "Иванов",
		Password:  "a-strong-test-password",
	})
	if !errors.Is(err, repositoryErr) {
		t.Fatalf("Register() error = %v, want wrapped repository error", err)
	}
}

func TestServiceRegisterEmptyOptionalFields(t *testing.T) {
	repo := &fakeUserCreator{}
	service := NewService(repo)

	email := "  "
	patronymic := " "
	u, err := service.Register(context.Background(), RegisterInput{
		Nickname:   "moonflux",
		FirstName:  "Иван",
		LastName:   "Иванов",
		Email:      &email,
		Patronymic: &patronymic,
		Password:   "a-strong-test-password",
	})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if u.Email != nil {
		t.Errorf("Email = %v, want nil", u.Email)
	}
	if u.Patronymic != nil {
		t.Errorf("Patronymic = %v, want nil", u.Patronymic)
	}
}

func TestServiceRegisterTimestamps(t *testing.T) {
	repo := &fakeUserCreator{}
	service := NewService(repo)

	// Keep this test focused on the timestamps supplied by the service.
	fixedTime := time.Date(2026, time.October, 14, 0, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return fixedTime }

	u, err := service.Register(context.Background(), RegisterInput{
		Nickname:  "moonflux",
		FirstName: "Иван",
		LastName:  "Иванов",
		Password:  "a-strong-test-password",
	})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if !u.CreatedAt.Equal(fixedTime) || !u.UpdatedAt.Equal(fixedTime) {
		t.Errorf("unexpected timestamps: created=%v updated=%v", u.CreatedAt, u.UpdatedAt)
	}
}

func TestServiceRegisterDoesNotTrimPassword(t *testing.T) {
	repo := &fakeUserCreator{}
	service := NewService(repo)

	password := "  password with spaces  "
	u, err := service.Register(context.Background(), RegisterInput{
		Nickname:  "moonflux",
		FirstName: "Иван",
		LastName:  "Иванов",
		Password:  password,
	})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if strings.TrimSpace(password) == password {
		t.Fatal("test password should contain surrounding spaces")
	}

	ok, err := VerifyPassword(password, u.PasswordHash)
	if err != nil {
		t.Fatalf("VerifyPassword() error = %v", err)
	}
	if !ok {
		t.Fatal("password whitespace was not preserved")
	}
}
