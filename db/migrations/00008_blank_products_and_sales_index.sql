-- A Product's name and unit are blank in any script (products R2, R4), and the Sales
-- queries are indexed (dashboard-and-reports R2, R7).

-- +goose Up
-- Blank is every character Go's unicode.IsSpace holds, which internal/api's pattern also
-- uses: U+0009-000D, U+0020, U+0085, U+00A0, U+1680, U+2000-200A, U+2028, U+2029, U+202F,
-- U+205F and U+3000. btrim and \s know only ASCII, and a class such as [[:space:]] depends
-- on the database's locale, so the characters are listed.
ALTER TABLE products DROP CONSTRAINT products_name_check;
ALTER TABLE products ADD CONSTRAINT products_name_check CHECK (
    name ~ '[^\t-\r \u0085   -     　]' AND char_length(name) <= 120
); -- products R2
ALTER TABLE products DROP CONSTRAINT products_unit_check;
ALTER TABLE products ADD CONSTRAINT products_unit_check CHECK (
    unit ~ '[^\t-\r \u0085   -     　]' AND char_length(unit) <= 20
); -- products R4

-- SalesByDay and TopSellingProducts read the Completed Orders whose completed_at is in a
-- range. Only completed Orders have one, so the index holds only them.
CREATE INDEX orders_completed_at_idx ON orders (completed_at) WHERE status = 'completed';

-- +goose Down
DROP INDEX orders_completed_at_idx;

ALTER TABLE products DROP CONSTRAINT products_unit_check;
ALTER TABLE products ADD CONSTRAINT products_unit_check CHECK (btrim(unit) <> '' AND char_length(unit) <= 20);
ALTER TABLE products DROP CONSTRAINT products_name_check;
ALTER TABLE products ADD CONSTRAINT products_name_check CHECK (btrim(name) <> '' AND char_length(name) <= 120);
