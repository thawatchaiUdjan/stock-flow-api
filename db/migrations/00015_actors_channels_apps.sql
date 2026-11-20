-- Who acts, how an Order was placed, and which app a login was for (business ADR 0014):
-- orders R1, R12, R21; stock-movements R2; users-and-access R36.
--
-- Additive: every existing row stays valid (a User's In-shop order, a User's movement, a
-- StockFlow login). Customer actor columns and their foreign keys arrive with the Customers
-- and Online order tickets, which extend the CHECKs below to name a Customer as one more
-- actor; until then an actor is a User or the system.

-- +goose Up

-- orders R21: an In-shop order is created by a User in StockFlow; an Online order is
-- placed by a Customer, who has no column yet, so it has no User.
ALTER TABLE orders ADD COLUMN channel text NOT NULL DEFAULT 'in-shop' CHECK (channel IN ('in-shop', 'online'));
ALTER TABLE orders ALTER COLUMN created_by DROP NOT NULL;
ALTER TABLE orders ADD CONSTRAINT orders_creator_check CHECK (channel = 'online' OR created_by IS NOT NULL);

-- orders R12: a cancellation is by a User or by the system, exactly one.
ALTER TABLE orders ADD COLUMN cancelled_by_system boolean NOT NULL DEFAULT false;
ALTER TABLE orders DROP CONSTRAINT orders_finished_check;
ALTER TABLE orders ADD CONSTRAINT orders_finished_check CHECK (
    (status = 'pending' AND completed_by IS NULL AND completed_at IS NULL AND cancelled_by IS NULL AND cancelled_at IS NULL AND NOT cancelled_by_system)
    OR (status = 'completed' AND completed_by IS NOT NULL AND completed_at IS NOT NULL AND cancelled_by IS NULL AND cancelled_at IS NULL AND NOT cancelled_by_system)
    OR (status = 'cancelled' AND (cancelled_by IS NOT NULL) <> cancelled_by_system AND cancelled_at IS NOT NULL AND completed_by IS NULL AND completed_at IS NULL)
);

-- stock-movements R2: made by a User or by the system, exactly one. Adding a column and
-- dropping NOT NULL fire no row trigger, so stock_movements_no_update_delete stays out of
-- the way.
ALTER TABLE stock_movements ALTER COLUMN user_id DROP NOT NULL;
ALTER TABLE stock_movements ADD COLUMN system boolean NOT NULL DEFAULT false;
ALTER TABLE stock_movements ADD CONSTRAINT stock_movements_actor_check CHECK ((user_id IS NOT NULL) <> system);

-- users-and-access R36: which app the login was for, and the Customer it matches, if any.
-- The Customer has no table yet, so no foreign key yet.
ALTER TABLE login_log ADD COLUMN app text NOT NULL DEFAULT 'stockflow' CHECK (app IN ('stockflow', 'storefront'));
ALTER TABLE login_log ADD COLUMN customer_id bigint;
ALTER TABLE login_log ADD CONSTRAINT login_log_customer_check CHECK (customer_id IS NULL OR (app = 'storefront' AND user_id IS NULL));

-- The failed-login windows count per app (users-and-access R57).
DROP INDEX login_log_failed_by_email;
DROP INDEX login_log_failed_by_ip;
CREATE INDEX login_log_failed_by_email ON login_log (app, lower(email), at DESC) WHERE result = 'failed-login';
CREATE INDEX login_log_failed_by_ip ON login_log (app, ip, at DESC) WHERE result = 'failed-login';

-- +goose Down
-- Fails once an Online order, a system actor or a Storefront entry exists: they cannot be
-- expressed in the old schema, and nothing is deleted to make them fit.
DROP INDEX login_log_failed_by_ip;
DROP INDEX login_log_failed_by_email;
CREATE INDEX login_log_failed_by_email ON login_log (lower(email), at DESC) WHERE result = 'failed-login';
CREATE INDEX login_log_failed_by_ip ON login_log (ip, at DESC) WHERE result = 'failed-login';
ALTER TABLE login_log DROP CONSTRAINT login_log_customer_check;
ALTER TABLE login_log DROP COLUMN customer_id;
ALTER TABLE login_log DROP COLUMN app;
ALTER TABLE stock_movements DROP CONSTRAINT stock_movements_actor_check;
ALTER TABLE stock_movements DROP COLUMN system;
ALTER TABLE stock_movements ALTER COLUMN user_id SET NOT NULL;
ALTER TABLE orders DROP CONSTRAINT orders_finished_check;
ALTER TABLE orders ADD CONSTRAINT orders_finished_check CHECK (
    (status = 'pending' AND completed_by IS NULL AND completed_at IS NULL AND cancelled_by IS NULL AND cancelled_at IS NULL)
    OR (status = 'completed' AND completed_by IS NOT NULL AND completed_at IS NOT NULL AND cancelled_by IS NULL AND cancelled_at IS NULL)
    OR (status = 'cancelled' AND cancelled_by IS NOT NULL AND cancelled_at IS NOT NULL AND completed_by IS NULL AND completed_at IS NULL)
);
ALTER TABLE orders DROP COLUMN cancelled_by_system;
ALTER TABLE orders DROP CONSTRAINT orders_creator_check;
ALTER TABLE orders ALTER COLUMN created_by SET NOT NULL;
ALTER TABLE orders DROP COLUMN channel;
