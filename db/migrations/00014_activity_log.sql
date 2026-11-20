-- The Activity log: every change to Products, Orders, Users and Customers, written in the
-- same transaction as the change (activity-log R1-R4, business ADR 0016).

-- +goose Up
CREATE TABLE activity_log (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    at timestamptz NOT NULL,
    actor_kind text NOT NULL CHECK (actor_kind IN ('user', 'customer', 'system', 'installation')),
    -- No foreign key: a Customer is not a User, and the entry keeps the actor as they
    -- were (R2, R11). The system and the installation have no id.
    actor_id bigint,
    actor_name text NOT NULL,
    actor_email text NOT NULL,
    -- A User's Role as it was then; empty for the others.
    actor_role text NOT NULL,
    action text NOT NULL,
    subject_kind text NOT NULL CHECK (subject_kind IN ('product', 'order', 'user', 'customer')),
    subject_id bigint NOT NULL,
    -- The name, and the SKU, Order number or email, as they were (R2, R11).
    subject_name text NOT NULL,
    subject_ref text NOT NULL,
    -- For an edit: [{"field", "before", "after"}]. Never a secret (R6).
    changes jsonb NOT NULL DEFAULT '[]',
    ip text NOT NULL,
    user_agent text NOT NULL,
    CHECK ((actor_kind IN ('user', 'customer')) = (actor_id IS NOT NULL))
);

-- activity-log R8: as for the Login log, the database refuses its own role too.
-- +goose StatementBegin
CREATE FUNCTION activity_log_refuse_change() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'activity_log entries cannot be changed or removed' USING ERRCODE = 'restrict_violation';
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER activity_log_no_update_delete BEFORE UPDATE OR DELETE ON activity_log
    FOR EACH ROW EXECUTE FUNCTION activity_log_refuse_change();
CREATE TRIGGER activity_log_no_truncate BEFORE TRUNCATE ON activity_log
    FOR EACH STATEMENT EXECUTE FUNCTION activity_log_refuse_change();

-- A new table closes the Data API in the migration that creates it (ADR 0021).
ALTER TABLE activity_log ENABLE ROW LEVEL SECURITY;

-- +goose Down
DROP TABLE activity_log;
DROP FUNCTION activity_log_refuse_change();
