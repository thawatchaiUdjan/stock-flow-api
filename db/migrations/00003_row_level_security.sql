-- Row Level Security with no policies shuts every role but the tables' owner out of these
-- tables, which closes Supabase's Data API. Authorization stays in Go (ADRs 0015, 0021).
-- Not FORCE: that would shut the owner out too, and the owner is the API.

-- +goose Up
ALTER TABLE users ENABLE ROW LEVEL SECURITY;
ALTER TABLE sessions ENABLE ROW LEVEL SECURITY;
ALTER TABLE setup_links ENABLE ROW LEVEL SECURITY;

-- +goose Down
ALTER TABLE setup_links DISABLE ROW LEVEL SECURITY;
ALTER TABLE sessions DISABLE ROW LEVEL SECURITY;
ALTER TABLE users DISABLE ROW LEVEL SECURITY;
