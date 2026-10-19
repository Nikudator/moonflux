package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"
	"time"

	"moonflux/internal/platform/uuid"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository appends audit events. It intentionally exposes no update/delete methods.
type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (r *Repository) Append(ctx context.Context, e Event) error {
	if e.ID == "" {
		return fmt.Errorf("audit event id is required")
	}
	if e.EventType == "" {
		return fmt.Errorf("audit event type is required")
	}
	if e.Outcome == "" {
		return fmt.Errorf("audit outcome is required")
	}
	if !e.Outcome.valid() {
		return fmt.Errorf("audit outcome %q is not allowed", e.Outcome)
	}
	if e.RequestID != nil && *e.RequestID != "" {
		if _, err := uuid.Parse(*e.RequestID); err != nil {
			return fmt.Errorf("audit request_id must be a UUID: %w", err)
		}
	}
	if e.OccurredAt.IsZero() {
		e.OccurredAt = time.Now().UTC()
	}
	metadata := e.Metadata
	if len(metadata) == 0 {
		metadata = json.RawMessage(`{}`)
	}
	if !json.Valid(metadata) {
		return fmt.Errorf("audit metadata must be valid JSON")
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO audit_logs (
			id, occurred_at, event_type, outcome, actor_user_id, target_user_id,
			request_id, client_ip, remote_ip, ip_source, user_agent, http_method,
			http_path, http_status, reason_code, metadata
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16
		)`, e.ID, e.OccurredAt, e.EventType, string(e.Outcome), e.ActorUserID, e.TargetUserID,
		nullableUUID(e.RequestID), nullableIP(e.ClientIP), nullableIP(e.RemoteIP), e.IPSource,
		e.UserAgent, e.HTTPMethod, e.HTTPPath, e.HTTPStatus, e.ReasonCode, []byte(metadata))
	if err != nil {
		return fmt.Errorf("append audit event %q: %w", e.EventType, err)
	}
	return nil
}

func nullableUUID(v *string) any {
	if v == nil || *v == "" {
		return nil
	}
	return *v
}
func nullableIP(v *netip.Addr) any {
	if v == nil || !v.IsValid() {
		return nil
	}
	return v.String()
}
