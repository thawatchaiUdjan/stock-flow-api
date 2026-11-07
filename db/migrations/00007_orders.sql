-- Orders and their lines (orders R1-R18, context ADR 0007).

-- +goose Up
-- orders R2: numbers count up from 1 and are never reused. A sequence never hands a number
-- out twice, even to an Order that was refused and rolled back; that gap is allowed.
CREATE SEQUENCE order_numbers;

CREATE TABLE orders (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    number bigint NOT NULL DEFAULT nextval('order_numbers') UNIQUE,
    -- orders R1: optional, up to 120 and 200 characters. Empty when there is none.
    customer text NOT NULL DEFAULT '' CHECK (char_length(customer) <= 120),
    note text NOT NULL DEFAULT '' CHECK (char_length(note) <= 200),
    -- orders R4: the sum of the lines' amounts, kept because neither ever changes (R14).
    total bigint NOT NULL CHECK (total >= 0),
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'completed', 'cancelled')), -- orders R10
    -- Users are never deleted (users-and-access R17), so these stay valid.
    created_by bigint NOT NULL REFERENCES users (id),
    created_at timestamptz NOT NULL,
    completed_by bigint REFERENCES users (id),
    completed_at timestamptz,
    cancelled_by bigint REFERENCES users (id),
    cancelled_at timestamptz,
    -- orders R11, R12: a finished Order records who finished it and when, and only that.
    CONSTRAINT orders_finished_check CHECK (
        (status = 'pending' AND completed_by IS NULL AND completed_at IS NULL AND cancelled_by IS NULL AND cancelled_at IS NULL)
        OR (status = 'completed' AND completed_by IS NOT NULL AND completed_at IS NOT NULL AND cancelled_by IS NULL AND cancelled_at IS NULL)
        OR (status = 'cancelled' AND cancelled_by IS NOT NULL AND cancelled_at IS NOT NULL AND completed_by IS NULL AND completed_at IS NULL)
    )
);

ALTER SEQUENCE order_numbers OWNED BY orders.number;

-- The Orders List, newest first (orders R15).
CREATE INDEX orders_status_number_idx ON orders (status, number);

CREATE TABLE order_lines (
    -- The order the lines were given in.
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    order_id bigint NOT NULL REFERENCES orders (id),
    -- Products are never deleted (products R11), so this stays valid.
    product_id bigint NOT NULL REFERENCES products (id),
    quantity integer NOT NULL CHECK (quantity >= 1), -- orders R3
    -- orders R3: the Product's price when the Order was created, in whole satang.
    price bigint NOT NULL CHECK (price BETWEEN 0 AND 999999999),
    -- orders R3: a Product is on one line of an Order at most.
    CONSTRAINT order_lines_order_product_key UNIQUE (order_id, product_id)
);

-- stock-movements R11: a movement made by an Order links to it; Receives and Adjustments
-- have none. Adding a column fires no row trigger, so stock_movements_no_update_delete
-- does not stand in the way.
ALTER TABLE stock_movements ADD COLUMN order_id bigint REFERENCES orders (id);
ALTER TABLE stock_movements ADD CONSTRAINT stock_movements_order_check
    CHECK ((kind IN ('order', 'cancellation')) = (order_id IS NOT NULL));

-- A new table closes the Data API in the migration that creates it (ADR 0021).
ALTER TABLE orders ENABLE ROW LEVEL SECURITY;
ALTER TABLE order_lines ENABLE ROW LEVEL SECURITY;

-- +goose Down
ALTER TABLE stock_movements DROP CONSTRAINT stock_movements_order_check;
ALTER TABLE stock_movements DROP COLUMN order_id;
DROP TABLE order_lines;
DROP TABLE orders;
