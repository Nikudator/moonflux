package user

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type passwordChangeRepositoryStub struct {
	user         User
	getErr       error
	changeErr    error
	changedID    string
	changedHash  string
	changeCalled bool
}

func (r *passwordChangeRepositoryStub) GetByID(
	_ context.Context,
	_ string,
) (User, error) {
	if r.getErr != nil {
		return User{}, r.getErr
	}

	return r.user, nil
}

func (r *passwordChangeRepositoryStub) ChangePassword(
	_ context.Context,
	id string,
	passwordHash string,
) error {
	r.changeCalled = true
	r.changedID = id
	r.changedHash = passwordHash

	return r.changeErr
}

func TestPasswordChangeServiceChange(t *testing.T) {
	const (
		userID          = "user-123"
		currentPassword = "Current-Password-123"
		newPassword     = "New-Password-456"
	)

	oldHash, err := HashPassword(currentPassword)
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}

	tests := []struct {
		name             string
		input            PasswordChangeInput
		getErr           error
		changeErr        error
		wantErr          error
		wantChangeCalled bool
	}{
		{
			name: "changes password with correct current password",
			input: PasswordChangeInput{
				CurrentPassword: currentPassword,
				NewPassword:     newPassword,
			},
			wantChangeCalled: true,
		},
		{
			name: "rejects incorrect current password",
			input: PasswordChangeInput{
				CurrentPassword: "Wrong-Password-123",
				NewPassword:     newPassword,
			},
			wantErr: ErrInvalidCurrentPassword,
		},
		{
			name: "returns user lookup error",
			input: PasswordChangeInput{
				CurrentPassword: currentPassword,
				NewPassword:     newPassword,
			},
			getErr:  ErrNotFound,
			wantErr: ErrNotFound,
		},
		{
			name: "returns password update error",
			input: PasswordChangeInput{
				CurrentPassword: currentPassword,
				NewPassword:     newPassword,
			},
			changeErr:        errors.New("database failure"),
			wantErr:          errors.New("database failure"),
			wantChangeCalled: true,
		},
		{
			name: "rejects short new password",
			input: PasswordChangeInput{
				CurrentPassword: currentPassword,
				NewPassword:     "1234567",
			},
			wantErr: ErrInvalidUser,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &passwordChangeRepositoryStub{
				user: User{
					ID:           userID,
					PasswordHash: oldHash,
				},
				getErr:    tt.getErr,
				changeErr: tt.changeErr,
			}

			service := NewPasswordChangeService(repo)
			err := service.Change(context.Background(), userID, tt.input)

			if tt.wantErr != nil {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr.Error()) {
					t.Fatalf("Change() error = %v, want error containing %q", err, tt.wantErr)
				}
			} else if err != nil {
				t.Fatalf("Change() error = %v", err)
			}

			if repo.changeCalled != tt.wantChangeCalled {
				t.Fatalf("ChangePassword() called = %v, want %v",
					repo.changeCalled, tt.wantChangeCalled)
			}

			if !repo.changeCalled {
				return
			}

			if repo.changedID != userID {
				t.Errorf("changed user ID = %q, want %q", repo.changedID, userID)
			}

			if repo.changedHash == "" {
				t.Fatal("new password hash is empty")
			}

			if repo.changedHash == oldHash {
				t.Fatal("new password hash equals old password hash")
			}

			valid, verifyErr := VerifyPassword(newPassword, repo.changedHash)
			if verifyErr != nil {
				t.Fatalf("VerifyPassword() error = %v", verifyErr)
			}
			if !valid {
				t.Error("stored hash does not match new password")
			}
		})
	}
}
