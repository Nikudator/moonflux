package audit

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeAppender struct {
	mu     sync.Mutex
	events []Event
	err    error
	calls  atomic.Int32
}

func (f *fakeAppender) Append(_ context.Context, e Event) error {
	f.calls.Add(1)
	f.mu.Lock()
	f.events = append(f.events, e)
	f.mu.Unlock()
	return f.err
}

func (f *fakeAppender) captured() []Event {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]Event, len(f.events))
	copy(out, f.events)
	return out
}

// waitFor polls until appender has been called at least `want` times,
// or fails after a bounded wait. Avoids fixed sleeps in tests.
func waitFor(t *testing.T, f *fakeAppender, want int32) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if f.calls.Load() >= want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d appends, got %d", want, f.calls.Load())
}

func TestRecorderDisabledIsNoop(t *testing.T) {
	f := &fakeAppender{}
	r := NewRecorder(f, slog.Default(), false)

	r.Record(context.Background(), NewEvent(EventAuthLoginFailed, OutcomeFailure))

	time.Sleep(50 * time.Millisecond)
	if f.calls.Load() != 0 {
		t.Fatalf("disabled recorder called appender %d times", f.calls.Load())
	}
}

func TestRecorderNilAppenderIsNoop(t *testing.T) {
	r := NewRecorder(nil, slog.Default(), true)

	// Must not panic or block.
	r.Record(context.Background(), NewEvent(EventAuthLoginFailed, OutcomeFailure))
}

func TestRecorderFillsIDAndOccurredAt(t *testing.T) {
	f := &fakeAppender{}
	r := NewRecorder(f, slog.Default(), true)

	before := time.Now()
	r.Record(context.Background(), NewEvent(EventAuthLoginSucceeded, OutcomeSuccess))
	waitFor(t, f, 1)

	events := f.captured()
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	e := events[0]
	if e.ID == "" {
		t.Error("ID was not filled")
	}
	if e.OccurredAt.IsZero() {
		t.Error("OccurredAt was not filled")
	}
	if e.OccurredAt.Before(before.Add(-time.Second)) || e.OccurredAt.After(time.Now().Add(time.Second)) {
		t.Errorf("OccurredAt = %v is not near the call time", e.OccurredAt)
	}
}

func TestRecorderRespectsExplicitFields(t *testing.T) {
	f := &fakeAppender{}
	r := NewRecorder(f, slog.Default(), true)

	explicitTime := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	e := NewEvent(EventAuthLoginFailed, OutcomeFailure)
	e.ID = "01900000-0000-7000-8000-000000000001"
	e.OccurredAt = explicitTime

	r.Record(context.Background(), e)
	waitFor(t, f, 1)

	got := f.captured()[0]
	if got.ID != e.ID {
		t.Errorf("ID = %q, want %q", got.ID, e.ID)
	}
	if !got.OccurredAt.Equal(explicitTime) {
		t.Errorf("OccurredAt = %v, want %v", got.OccurredAt, explicitTime)
	}
}

func TestRecorderLogsAppendFailure(t *testing.T) {
	var logBuf strings.Builder
	logger := slog.New(slog.NewJSONHandler(&logBuf, nil))

	f := &fakeAppender{err: errors.New("db down")}
	r := NewRecorder(f, logger, true)

	r.Record(context.Background(), NewEvent(EventAuthLoginFailed, OutcomeFailure))
	waitFor(t, f, 1)

	// Give the goroutine a moment to write the log line.
	time.Sleep(20 * time.Millisecond)

	out := logBuf.String()
	if !strings.Contains(out, "audit.append_failed") {
		t.Errorf("expected log entry, got: %s", out)
	}
	if !strings.Contains(out, "db down") {
		t.Errorf("expected error text in log, got: %s", out)
	}
}

func TestRecorderDoesNotBlockCaller(t *testing.T) {
	block := make(chan struct{})
	f := &blockingAppender{release: block}
	r := NewRecorder(f, slog.Default(), true)

	start := time.Now()
	r.Record(context.Background(), NewEvent(EventAuthLoginSucceeded, OutcomeSuccess))
	elapsed := time.Since(start)

	close(block) // release the goroutine

	if elapsed > 100*time.Millisecond {
		t.Fatalf("Record() took %v, expected fire-and-forget", elapsed)
	}
}

type blockingAppender struct{ release chan struct{} }

func (b *blockingAppender) Append(_ context.Context, _ Event) error {
	<-b.release
	return nil
}

func TestEventBuilder(t *testing.T) {
	e := NewEvent(EventArticleCreated, OutcomeSuccess).
		WithActor("user-1").
		WithTarget("article-1").
		WithReason("").
		WithRequestID("req-1")

	if e.EventType != EventArticleCreated {
		t.Errorf("EventType = %q", e.EventType)
	}
	if e.ActorUserID == nil || *e.ActorUserID != "user-1" {
		t.Errorf("ActorUserID = %v", e.ActorUserID)
	}
	if e.TargetUserID == nil || *e.TargetUserID != "article-1" {
		t.Errorf("TargetUserID = %v", e.TargetUserID)
	}
	if e.ReasonCode != nil {
		t.Errorf("ReasonCode should be nil for empty input, got %v", *e.ReasonCode)
	}
	if e.RequestID == nil || *e.RequestID != "req-1" {
		t.Errorf("RequestID = %v", e.RequestID)
	}
}
