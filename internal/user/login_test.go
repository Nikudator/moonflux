package user

import (
	"context"
	"errors"
	"testing"
)

type fakeLoginRepository struct {
	user  User
	err   error
	login string
	calls int
}

func (f *fakeLoginRepository) GetByLogin(
	_ context.Context,
	login string,
) (User, error) {
	f.calls++
	f.login = login
	return f.user, f.err
}

func TestLoginServiceAuthenticate(t *testing.T) {
	hash, err := HashPassword("correct-password")
	if err != nil {
		t.Fatal(err)
	}

	repo := &fakeLoginRepository{
		user: User{
			ID:           "user-123",
			Nickname:     "moonflux",
			Role:         RoleUser,
			PasswordHash: hash,
		},
	}
	service := NewLoginService(repo, false)

	u, err := service.Authenticate(context.Background(), LoginInput{
		Login:    "  moonflux  ",
		Password: "correct-password",
	})
	if err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	if u.ID != "user-123" {
		t.Errorf("user ID = %q, want user-123", u.ID)
	}
	if repo.login != "moonflux" {
		t.Errorf("repository login = %q, want moonflux", repo.login)
	}
}

func TestLoginServiceRejectsWrongPassword(t *testing.T) {
	hash, err := HashPassword("correct-password")
	if err != nil {
		t.Fatal(err)
	}

	repo := &fakeLoginRepository{
		user: User{
			ID:           "user-123",
			PasswordHash: hash,
		},
	}
	service := NewLoginService(repo, false)

	_, err = service.Authenticate(context.Background(), LoginInput{
		Login:    "moonflux",
		Password: "wrong-password",
	})
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("Authenticate() error = %v, want ErrInvalidCredentials", err)
	}
}

func TestLoginServiceHidesUnknownUser(t *testing.T) {
	repo := &fakeLoginRepository{err: ErrNotFound}
	service := NewLoginService(repo, false)

	_, err := service.Authenticate(context.Background(), LoginInput{
		Login:    "unknown",
		Password: "some-password",
	})
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("Authenticate() error = %v, want ErrInvalidCredentials", err)
	}
}

func TestLoginServiceRejectsEmptyCredentials(t *testing.T) {
	repo := &fakeLoginRepository{}
	service := NewLoginService(repo, false)

	for _, input := range []LoginInput{
		{Login: "", Password: "password"},
		{Login: "moonflux", Password: ""},
		{Login: " ", Password: "password"},
	} {
		_, err := service.Authenticate(context.Background(), input)
		if !errors.Is(err, ErrInvalidCredentials) {
			t.Errorf("Authenticate(%+v) error = %v, want ErrInvalidCredentials", input, err)
		}
	}

	if repo.calls != 0 {
		t.Errorf("repository calls = %d, want 0", repo.calls)
	}
}

func TestLoginServicePropagatesRepositoryError(t *testing.T) {
	repositoryErr := errors.New("database unavailable")
	repo := &fakeLoginRepository{err: repositoryErr}
	service := NewLoginService(repo, false)

	_, err := service.Authenticate(context.Background(), LoginInput{
		Login:    "moonflux",
		Password: "password",
	})
	if !errors.Is(err, repositoryErr) {
		t.Fatalf("Authenticate() error = %v, want wrapped repository error", err)
	}
}

func TestLoginServiceRejectsUnverifiedEmail(t *testing.T) {
	hash, err := HashPassword("correct-password")
	if err != nil {
		t.Fatal(err)
	}

	repo := &fakeLoginRepository{
		user: User{
			ID:              "user-123",
			PasswordHash:    hash,
			EmailVerifiedAt: nil,
		},
	}
	service := NewLoginService(repo, true)

	_, err = service.Authenticate(context.Background(), LoginInput{
		Login:    "moonflux",
		Password: "correct-password",
	})
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("Authenticate() error = %v, want ErrInvalidCredentials", err)
	}
}

func TestLoginServiceAllowsUnverifiedEmailWhenDisabled(t *testing.T) {
	hash, err := HashPassword("correct-password")
	if err != nil {
		t.Fatal(err)
	}

	repo := &fakeLoginRepository{
		user: User{
			ID:              "user-123",
			PasswordHash:    hash,
			EmailVerifiedAt: nil,
		},
	}
	service := NewLoginService(repo, false)

	_, err = service.Authenticate(context.Background(), LoginInput{
		Login:    "moonflux",
		Password: "correct-password",
	})
	if err != nil {
		t.Fatalf("Authenticate() error = %v, want nil", err)
	}
}
