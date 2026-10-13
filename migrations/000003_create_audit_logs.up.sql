CREATE TABLE audit_logs (
    id UUID PRIMARY KEY,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    event_type VARCHAR(100) NOT NULL,
    outcome VARCHAR(20) NOT NULL CHECK (outcome IN ('success', 'failure', 'denied', 'info')),
    actor_user_id UUID,
    target_user_id UUID,
    request_id UUID,
    client_ip INET,
    remote_ip INET,
    ip_source VARCHAR(30),
    user_agent TEXT,
    http_method VARCHAR(10),
    http_path TEXT,
    http_status SMALLINT CHECK (http_status IS NULL OR http_status BETWEEN 100 AND 599),
    reason_code VARCHAR(100),
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    expires_at TIMESTAMPTZ,
    archived_at TIMESTAMPTZ,
    CONSTRAINT audit_logs_event_type_not_blank CHECK (length(btrim(event_type)) > 0),
    CONSTRAINT audit_logs_no_self_fk_cascade CHECK (true)
);

CREATE INDEX audit_logs_occurred_at_idx ON audit_logs (occurred_at DESC);
CREATE INDEX audit_logs_event_time_idx ON audit_logs (event_type, occurred_at DESC);
CREATE INDEX audit_logs_actor_time_idx ON audit_logs (actor_user_id, occurred_at DESC) WHERE actor_user_id IS NOT NULL;
CREATE INDEX audit_logs_target_time_idx ON audit_logs (target_user_id, occurred_at DESC) WHERE target_user_id IS NOT NULL;
CREATE INDEX audit_logs_client_ip_time_idx ON audit_logs (client_ip, occurred_at DESC) WHERE client_ip IS NOT NULL;
CREATE INDEX audit_logs_request_id_idx ON audit_logs (request_id) WHERE request_id IS NOT NULL;

-- Intentionally no cascading foreign keys to users: audit history survives logical account deletion.
-- Application DB role should receive INSERT/SELECT only, not UPDATE/DELETE on this table.
