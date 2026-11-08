-- A User's name and an Adjustment's note are blank in any script (users-and-access R14,
-- stock-movements R5), as a Product's name and unit are since migration 00008.

-- +goose Up
-- Blank is every character Go's unicode.IsSpace holds, which internal/api's patterns also
-- use; the list is the one migration 00008 explains.
ALTER TABLE users DROP CONSTRAINT users_name_check;
ALTER TABLE users ADD CONSTRAINT users_name_check CHECK (
    name ~ '[^\t-\r \u0085\u00a0\u1680\u2000-\u200a\u2028\u2029\u202f\u205f\u3000]'
); -- users-and-access R14
ALTER TABLE stock_movements DROP CONSTRAINT stock_movements_adjustment_note_check;
ALTER TABLE stock_movements ADD CONSTRAINT stock_movements_adjustment_note_check CHECK (
    kind <> 'adjustment' OR note ~ '[^\t-\r \u0085\u00a0\u1680\u2000-\u200a\u2028\u2029\u202f\u205f\u3000]'
); -- stock-movements R5

-- +goose Down
ALTER TABLE stock_movements DROP CONSTRAINT stock_movements_adjustment_note_check;
ALTER TABLE stock_movements ADD CONSTRAINT stock_movements_adjustment_note_check CHECK (kind <> 'adjustment' OR btrim(note) <> '');
ALTER TABLE users DROP CONSTRAINT users_name_check;
ALTER TABLE users ADD CONSTRAINT users_name_check CHECK (btrim(name) <> '');
