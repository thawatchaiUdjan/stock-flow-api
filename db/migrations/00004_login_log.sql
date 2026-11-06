-- The Login log: every login, failed login and logout (users-and-access R36).

-- +goose Up
CREATE TABLE login_log (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    at timestamptz NOT NULL,
    -- As the person typed it, so a guess at an email nobody has is kept (R36).
    email text NOT NULL,
    -- NULL when the email matches no User. Users are never deleted (R17), so it stays valid.
    user_id bigint REFERENCES users (id),
    result text NOT NULL CHECK (result IN ('login', 'failed-login', 'logout')),
    ip text NOT NULL,
    user_agent text NOT NULL
    -- There is no column for a password: it never reaches an entry (R37).
);

-- users-and-access R39: entries are never edited, and never deleted, by anyone. The API
-- has no operation that does, and this refuses the database's own role too.
-- +goose StatementBegin
CREATE FUNCTION login_log_refuse_change() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'login_log entries cannot be changed or removed' USING ERRCODE = 'restrict_violation';
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER login_log_no_update_delete BEFORE UPDATE OR DELETE ON login_log
    FOR EACH ROW EXECUTE FUNCTION login_log_refuse_change();
CREATE TRIGGER login_log_no_truncate BEFORE TRUNCATE ON login_log
    FOR EACH STATEMENT EXECUTE FUNCTION login_log_refuse_change();

-- A new table closes the Data API in the migration that creates it (ADR 0021).
ALTER TABLE login_log ENABLE ROW LEVEL SECURITY;

-- +goose Down
DROP TABLE login_log;
DROP FUNCTION login_log_refuse_change();
