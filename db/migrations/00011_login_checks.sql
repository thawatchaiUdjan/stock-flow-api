-- +goose Up
-- Hashed IP addresses and a bounded rolling window for Login's email step, shared
-- across API instances (users-and-access R53).
CREATE TABLE login_checks (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    ip_hash bytea NOT NULL,
    checked_at timestamptz NOT NULL
);
CREATE INDEX login_checks_ip_time ON login_checks (ip_hash, checked_at);
CREATE INDEX login_checks_time ON login_checks (checked_at);
ALTER TABLE login_checks ENABLE ROW LEVEL SECURITY;

-- +goose Down
DROP TABLE login_checks;
