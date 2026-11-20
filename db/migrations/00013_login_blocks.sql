-- Failed-login limits (users-and-access R55-R57, business ADR 0017): a blocked attempt is
-- a Login log entry, and the windows count recent failures by email and by IP address.

-- +goose Up
ALTER TABLE login_log DROP CONSTRAINT login_log_result_check;
ALTER TABLE login_log ADD CONSTRAINT login_log_result_check
    CHECK (result IN ('login', 'failed-login', 'logout', 'blocked'));

-- Partial: only failures are counted, newest first.
CREATE INDEX login_log_failed_by_email ON login_log (lower(email), at DESC) WHERE result = 'failed-login';
CREATE INDEX login_log_failed_by_ip ON login_log (ip, at DESC) WHERE result = 'failed-login';

-- +goose Down
DROP INDEX login_log_failed_by_ip;
DROP INDEX login_log_failed_by_email;
ALTER TABLE login_log DROP CONSTRAINT login_log_result_check;
-- Fails once a blocked entry exists: entries cannot be changed or removed (R39).
ALTER TABLE login_log ADD CONSTRAINT login_log_result_check
    CHECK (result IN ('login', 'failed-login', 'logout'));
