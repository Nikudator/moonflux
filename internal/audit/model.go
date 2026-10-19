package audit

import (
	"encoding/json"
	"net/netip"
	"time"
)

// Outcome enumerates allowed audit outcomes, mirroring the DB CHECK constraint.
type Outcome string

const (
	OutcomeSuccess Outcome = "success"
	OutcomeFailure Outcome = "failure"
	OutcomeDenied  Outcome = "denied"
	OutcomeInfo    Outcome = "info"
)

// valid reports whether the outcome is one of the four values accepted
// by the audit_logs.outcome CHECK constraint.
func (o Outcome) valid() bool {
	switch o {
	case OutcomeSuccess, OutcomeFailure, OutcomeDenied, OutcomeInfo:
		return true
	}
	return false
}

type Event struct {
	ID           string          `json:"id"`
	OccurredAt   time.Time       `json:"occurred_at"`
	EventType    string          `json:"event_type"`
	Outcome      Outcome         `json:"outcome"`
	ActorUserID  *string         `json:"actor_user_id,omitempty"`
	TargetUserID *string         `json:"target_user_id,omitempty"`
	RequestID    *string         `json:"request_id,omitempty"`
	ClientIP     *netip.Addr     `json:"client_ip,omitempty"`
	RemoteIP     *netip.Addr     `json:"remote_ip,omitempty"`
	IPSource     *string         `json:"ip_source,omitempty"`
	UserAgent    *string         `json:"user_agent,omitempty"`
	HTTPMethod   *string         `json:"http_method,omitempty"`
	HTTPPath     *string         `json:"http_path,omitempty"`
	HTTPStatus   *int            `json:"http_status,omitempty"`
	ReasonCode   *string         `json:"reason_code,omitempty"`
	Metadata     json.RawMessage `json:"metadata,omitempty"`
}
