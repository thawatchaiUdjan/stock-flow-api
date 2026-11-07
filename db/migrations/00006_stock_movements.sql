-- Stock movements: every change to a Product's stock (stock-movements R1-R12, context ADR 0006).

-- +goose Up
CREATE TABLE stock_movements (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    kind text NOT NULL CHECK (kind IN ('receive', 'adjustment', 'order', 'cancellation')), -- stock-movements R3
    -- Products are never deleted (products R11), so this stays valid.
    product_id bigint NOT NULL REFERENCES products (id),
    change integer NOT NULL CHECK (change <> 0),
    -- stock-movements R6: no movement takes stock below 0.
    stock_after integer NOT NULL CHECK (stock_after >= 0),
    at timestamptz NOT NULL,
    -- Users are never deleted (users-and-access R17), so this stays valid.
    user_id bigint NOT NULL REFERENCES users (id),
    -- stock-movements R2: optional, up to 200 characters. Empty when there is none.
    note text NOT NULL DEFAULT '' CHECK (char_length(note) <= 200),
    -- stock-movements R4, orders R7, R12: Receive and Cancellation add stock, Order takes it.
    CONSTRAINT stock_movements_direction_check CHECK (
        (kind IN ('receive', 'cancellation') AND change > 0) OR kind = 'adjustment' OR (kind = 'order' AND change < 0)
    ),
    -- stock-movements R5: an Adjustment says why.
    CONSTRAINT stock_movements_adjustment_note_check CHECK (kind <> 'adjustment' OR btrim(note) <> '')
);

-- A Product's history, newest first (stock-movements R10).
CREATE INDEX stock_movements_product_id_idx ON stock_movements (product_id, id);

-- stock-movements R8: a movement is never edited or deleted. The API has no operation
-- that does, and this refuses the database's own role too.
-- +goose StatementBegin
CREATE FUNCTION stock_movements_refuse_change() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'stock movements cannot be changed or removed' USING ERRCODE = 'restrict_violation';
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER stock_movements_no_update_delete BEFORE UPDATE OR DELETE ON stock_movements
    FOR EACH ROW EXECUTE FUNCTION stock_movements_refuse_change();
CREATE TRIGGER stock_movements_no_truncate BEFORE TRUNCATE ON stock_movements
    FOR EACH STATEMENT EXECUTE FUNCTION stock_movements_refuse_change();

-- A new table closes the Data API in the migration that creates it (ADR 0021).
ALTER TABLE stock_movements ENABLE ROW LEVEL SECURITY;

-- +goose Down
DROP TABLE stock_movements;
DROP FUNCTION stock_movements_refuse_change();
