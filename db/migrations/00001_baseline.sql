-- Baseline: no schema yet. goose refuses an empty migrations directory, and the deploy
-- workflow and the test database both run `goose up`, so this no-op lets them run
-- before the first feature adds a table. Its only effect is goose's version table.

-- +goose Up
SELECT 1;

-- +goose Down
SELECT 1;
