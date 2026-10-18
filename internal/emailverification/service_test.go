package emailverification

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"testing"
	"time"
)

type fakeRepository struct {
	userID     string
	tokenHash  []byte
	expiresAt  time.Time
	issueNow   time.Time
	cooldown   time.Duration
	verifyHash []byte
	verifyNow  time.Time

	issueErr    error
	verifyErr   error
	issueCalls  int
	verifyCalls int
}

func (f *fakeRepository) Issue(
	_ context.Context,
	userID string,
	tokenHash []byte,
	expiresAt time.Time,
	now time.Time,
	resendCooldown time.Duration,
) error {
	f.issueCalls++
	f.userID = userID
	f.tokenHash = append([]byte(nil), tokenHash...)
	f.expiresAt = expiresAt
	f.issueNow = now
	f.cooldown = resendCooldown
	return f.issueErr
}

func (f *fakeRepository) Verify(
	_ context.Context,
	tokenHash []byte,
	now time.Time,
) error {
	f.verifyCalls++
	f.verifyHash = append([]byte(nil), tokenHash...)
	f.verifyNow = now
	return f.verifyErr
}

func TestServiceIssue(t *testing.T) {
	repo := &fakeRepository{}
	service, err := NewService(repo, time.Hour, time.Minute)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	fixedTime := time.Date(2026, time.October, 18, 1, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return fixedTime }

	token, err := service.Issue(context.Background(), "user-123")
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	if token == "" {
		t.Fatal("Issue() returned an empty token")
	}
	if repo.issueCalls != 1 {
		t.Fatalf("Issue calls = %d, want 1", repo.issueCalls)
	}
	if repo.userID != "user-123" {
		t.Errorf("userID = %q, want user-123", repo.userID)
	}
	if len(repo.tokenHash) != sha256.Size {
		t.Fatalf("stored hash length = %d, want %d", len(repo.tokenHash), sha256.Size)
	}

	expectedHash := sha256.Sum256([]byte(token))
	if !bytes.Equal(repo.tokenHash, expectedHash[:]) {
		t.Fatal("repository did not receive the token's SHA-256 hash")
	}
	if bytes.Equal(repo.tokenHash, []byte(token)) {
		t.Fatal("repository received the raw token instead of its hash")
	}
	if !repo.expiresAt.Equal(fixedTime.Add(time.Hour)) {
		t.Errorf("expiresAt = %v, want %v", repo.expiresAt, fixedTime.Add(time.Hour))
	}
	if !repo.issueNow.Equal(fixedTime) {
		t.Errorf("issue time = %v, want %v", repo.issueNow, fixedTime)
	}
	if repo.cooldown != time.Minute {
		t.Errorf("resend cooldown = %v, want %v", repo.cooldown, time.Minute)
	}
}

func TestServiceIssueRejectsEmptyUserID(t *testing.T) {
	repo := &fakeRepository{}
	service, err := NewService(repo, time.Hour, time.Minute)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = service.Issue(context.Background(), "")
	if !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("Issue() error = %v, want ErrUserNotFound", err)
	}
	if repo.issueCalls != 0 {
		t.Fatalf("Issue calls = %d, want 0", repo.issueCalls)
	}
}

func TestServiceIssueRepositoryError(t *testing.T) {
	repositoryErr := errors.New("database unavailable")
	repo := &fakeRepository{issueErr: repositoryErr}
	service, err := NewService(repo, time.Hour, time.Minute)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = service.Issue(context.Background(), "user-123")
	if !errors.Is(err, repositoryErr) {
		t.Fatalf("Issue() error = %v, want repository error", err)
	}
}

func TestServiceVerify(t *testing.T) {
	repo := &fakeRepository{}
	service, err := NewService(repo, time.Hour, time.Minute)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	fixedTime := time.Date(2026, time.October, 18, 1, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return fixedTime }

	token := "example-verification-token"
	err = service.Verify(context.Background(), token)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}

	expectedHash := sha256.Sum256([]byte(token))
	if !bytes.Equal(repo.verifyHash, expectedHash[:]) {
		t.Fatal("Verify() did not pass the token's SHA-256 hash to the repository")
	}
	if !repo.verifyNow.Equal(fixedTime) {
		t.Errorf("verification time = %v, want %v", repo.verifyNow, fixedTime)
	}
	if repo.verifyCalls != 1 {
		t.Fatalf("Verify calls = %d, want 1", repo.verifyCalls)
	}
}

func TestServiceVerifyRejectsEmptyToken(t *testing.T) {
	repo := &fakeRepository{}
	service, err := NewService(repo, time.Hour, time.Minute)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	err = service.Verify(context.Background(), "")
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("Verify() error = %v, want ErrInvalidToken", err)
	}
	if repo.verifyCalls != 0 {
		t.Fatalf("Verify calls = %d, want 0", repo.verifyCalls)
	}
}

func TestServiceVerifyRepositoryError(t *testing.T) {
	repositoryErr := ErrInvalidToken
	repo := &fakeRepository{verifyErr: repositoryErr}
	service, err := NewService(repo, time.Hour, time.Minute)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	err = service.Verify(context.Background(), "example-token")
	if !errors.Is(err, repositoryErr) {
		t.Fatalf("Verify() error = %v, want repository error", err)
	}
}
