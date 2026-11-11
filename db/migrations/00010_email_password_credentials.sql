-- +goose Up
-- One current credential per User; replacement and consumption are serialized on
-- the User (users-and-access R27, R31, R41, R42, R48).
CREATE TABLE email_credentials (
    user_id bigint PRIMARY KEY REFERENCES users (id),
    kind text NOT NULL CHECK (kind IN ('first-time-code', 'setup-grant', 'reset-link')),
    token_hash bytea NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    failed_attempts integer NOT NULL DEFAULT 0 CHECK (failed_attempts BETWEEN 0 AND 5),
    blocked_until timestamptz NOT NULL DEFAULT 'epoch'
);
ALTER TABLE email_credentials ENABLE ROW LEVEL SECURITY;

-- Hashed identities and a bounded rolling window, shared across API instances (R43).
CREATE TABLE email_request_events (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    email_hash bytea NOT NULL,
    ip_hash bytea NOT NULL,
    requested_at timestamptz NOT NULL
);
CREATE INDEX email_request_events_email_time ON email_request_events (email_hash, requested_at);
CREATE INDEX email_request_events_ip_time ON email_request_events (ip_hash, requested_at);
CREATE INDEX email_request_events_time ON email_request_events (requested_at);
ALTER TABLE email_request_events ENABLE ROW LEVEL SECURITY;

-- +goose Down
DROP TABLE email_request_events;
DROP TABLE email_credentials;
