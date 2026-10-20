package audit

import (
	"context"
	"log/slog"
	"time"

	"moonflux/internal/platform/uuid"
)

// Appender is the subset of Repository used by Recorder.
// Declared as an interface so tests can substitute a fake.
type Appender interface {
	Append(ctx context.Context, e Event) error
}

// Recorder wraps an Appender with fire-and-forget semantics.
// A failed audit write never fails the calling request: it is logged
// and dropped. A nil or disabled Recorder is a safe no-op.
type Recorder struct {
	appender Appender
	log      *slog.Logger
	enabled  bool
	timeout  time.Duration
}

func NewRecorder(appender Appender, log *slog.Logger, enabled bool) *Recorder {
	if log == nil {
		log = slog.Default()
	}
	return &Recorder{
		appender: appender,
		log:      log,
		enabled:  enabled,
		timeout:  5 * time.Second,
	}
}

// Record schedules an audit event for asynchronous persistence.
// The Event.ID and Event.OccurredAt fields are filled in if empty.
// The request context is detached from cancellation: the caller may
// return before the goroutine finishes, but the write still completes.
func (r *Recorder) Record(ctx context.Context, e Event) {
	if !r.enabled || r.appender == nil {
		return
	}

	if e.ID == "" {
		id, err := uuid.NewV7()
		if err != nil {
			r.log.Error("audit event id generation failed",
				"event", "audit.id_failed",
				"event_type", e.EventType,
				"error", err,
			)
			return
		}
		e.ID = uuid.String(id)
	}
	if e.OccurredAt.IsZero() {
		e.OccurredAt = time.Now().UTC()
	}

	// Detach from request cancellation, but cap the write duration.
	detached := context.WithoutCancel(ctx)
	timeout := r.timeout

	go func() {
		writeCtx, cancel := context.WithTimeout(detached, timeout)
		defer cancel()
		if err := r.appender.Append(writeCtx, e); err != nil {
			r.log.ErrorContext(writeCtx, "audit append failed",
				"event", "audit.append_failed",
				"event_type", e.EventType,
				"outcome", string(e.Outcome),
				"error", err,
			)
		}
	}()
}
