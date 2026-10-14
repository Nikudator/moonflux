package user

import (
	"context"
	"errors"
	"testing"
)

type fakeProfileUpdater struct {
	gotID    string
	gotInput ProfileUpdateInput
	user     User
	err      error
	calls    int
}

func (f *fakeProfileUpdater) UpdateProfile(
	_ context.Context,
	id string,
	input ProfileUpdateInput,
) (User, error) {
	f.calls++
	f.gotID = id
	f.gotInput = input
	return f.user, f.err
}

func TestProfileServiceUpdateTrimsText(t *testing.T) {
	repo := &fakeProfileUpdater{
		user: User{ID: "user-123", Nickname: "new-name"},
	}
	service := NewProfileService(repo)

	input := ProfileUpdateInput{
		Nickname:   Optional[string]{Present: true, Value: " new-name "},
		FirstName:  Optional[string]{Present: true, Value: " Alice "},
		Patronymic: Optional[string]{Present: true, Value: "   "},
	}

	got, err := service.Update(context.Background(), "user-123", input)
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	if got.ID != "user-123" {
		t.Fatalf("unexpected user ID: %q", got.ID)
	}
	if repo.calls != 1 {
		t.Fatalf("repository calls = %d, want 1", repo.calls)
	}
	if repo.gotID != "user-123" {
		t.Fatalf("repository ID = %q", repo.gotID)
	}
	if repo.gotInput.Nickname.Value != "new-name" {
		t.Fatalf("nickname was not trimmed: %q", repo.gotInput.Nickname.Value)
	}
	if repo.gotInput.FirstName.Value != "Alice" {
		t.Fatalf("first name was not trimmed: %q", repo.gotInput.FirstName.Value)
	}
	if !repo.gotInput.Patronymic.Null {
		t.Fatal("blank patronymic should be converted to null")
	}
}

func TestProfileServiceUpdateRejectsInvalidInput(t *testing.T) {
	repo := &fakeProfileUpdater{}
	service := NewProfileService(repo)

	_, err := service.Update(context.Background(), "user-123", ProfileUpdateInput{
		Nickname: Optional[string]{Present: true, Value: "  "},
	})
	if !errors.Is(err, ErrInvalidUser) {
		t.Fatalf("Update() error = %v, want ErrInvalidUser", err)
	}
	if repo.calls != 0 {
		t.Fatalf("repository calls = %d, want 0", repo.calls)
	}
}

func TestProfileServiceUpdateRejectsEmptyID(t *testing.T) {
	repo := &fakeProfileUpdater{}
	service := NewProfileService(repo)

	_, err := service.Update(context.Background(), " ", ProfileUpdateInput{})
	if !errors.Is(err, ErrInvalidUser) {
		t.Fatalf("Update() error = %v, want ErrInvalidUser", err)
	}
	if repo.calls != 0 {
		t.Fatalf("repository calls = %d, want 0", repo.calls)
	}
}

func TestProfileServiceUpdatePropagatesRepositoryError(t *testing.T) {
	repo := &fakeProfileUpdater{err: ErrNotFound}
	service := NewProfileService(repo)

	_, err := service.Update(context.Background(), "user-123", ProfileUpdateInput{})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Update() error = %v, want ErrNotFound", err)
	}
}
