package audit

import (
	"context"
	"strings"
	"testing"
)

func TestAppendRejectsInvalidOutcome(t *testing.T) {
	repo := NewRepository(nil) // pool not needed: validation fails first.

	err := repo.Append(context.Background(), Event{
		ID:        "01900000-0000-7000-8000-000000000001",
		EventType: "auth.login.failed",
		Outcome:   Outcome("Sucess"), // typo
	})
	if err == nil {
		t.Fatal("Append() accepted invalid outcome")
	}
	if !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAppendRejectsEmptyOutcome(t *testing.T) {
	repo := NewRepository(nil)

	err := repo.Append(context.Background(), Event{
		ID:        "01900000-0000-7000-8000-000000000001",
		EventType: "auth.login.failed",
	})
	if err == nil {
		t.Fatal("Append() accepted empty outcome")
	}
	if !strings.Contains(err.Error(), "required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAppendRejectsInvalidRequestID(t *testing.T) {
	repo := NewRepository(nil)

	bad := "not-a-uuid"
	err := repo.Append(context.Background(), Event{
		ID:        "01900000-0000-7000-8000-000000000001",
		EventType: "auth.login.failed",
		Outcome:   OutcomeFailure,
		RequestID: &bad,
	})
	if err == nil {
		t.Fatal("Append() accepted non-UUID request_id")
	}
	if !strings.Contains(err.Error(), "request_id") {
		t.Fatalf("unexpected error: %v", err)
	}
}
