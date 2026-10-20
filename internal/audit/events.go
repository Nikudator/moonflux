package audit

// Event type constants. Kept as strings so that future dynamic event
// types do not require interface changes.
const (
	EventAuthLoginSucceeded = "auth.login.succeeded"
	EventAuthLoginFailed    = "auth.login.failed"

	EventUserPasswordChanged = "user.password.changed"

	EventArticleCreated = "article.created"
	EventArticleUpdated = "article.updated"
	EventArticleDeleted = "article.deleted"

	EventArticlePreviewDenied = "article.preview.denied"
)

// Reason codes used in Event.ReasonCode.
const (
	ReasonInvalidCredentials = "invalid_credentials"
	ReasonUnverifiedEmail    = "unverified_email"
	ReasonForbidden          = "forbidden"
)

// NewEvent returns a zero-valued Event with the given type and outcome.
// ID and OccurredAt are filled in by Recorder.Record at dispatch time.
func NewEvent(eventType string, outcome Outcome) Event {
	return Event{
		EventType: eventType,
		Outcome:   outcome,
	}
}

// WithActor sets the acting user. Returns the same Event so calls chain.
func (e Event) WithActor(userID string) Event {
	if userID == "" {
		return e
	}
	id := userID
	e.ActorUserID = &id
	return e
}

// WithTarget sets the affected entity. For article events, target is
// the article ID. Stored in audit_logs.target_user_id, which is not
// a foreign key — the column name is historical.
func (e Event) WithTarget(targetID string) Event {
	if targetID == "" {
		return e
	}
	id := targetID
	e.TargetUserID = &id
	return e
}

// WithReason sets a short machine-readable reason code.
func (e Event) WithReason(code string) Event {
	if code == "" {
		return e
	}
	c := code
	e.ReasonCode = &c
	return e
}

// WithRequestID links the event to the current HTTP request.
func (e Event) WithRequestID(requestID string) Event {
	if requestID == "" {
		return e
	}
	r := requestID
	e.RequestID = &r
	return e
}
