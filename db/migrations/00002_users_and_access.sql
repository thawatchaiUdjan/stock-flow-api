-- Users, their sessions and their Setup links (users-and-access, ADR 0015).

-- +goose Up
CREATE TABLE users (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name text NOT NULL CHECK (btrim(name) <> ''), -- users-and-access R14
    email text NOT NULL,
    role text NOT NULL CHECK (role IN ('owner', 'admin', 'staff')), -- users-and-access R8
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive')),
    password_hash text -- NULL until the User sets one through a Setup link
);

-- users-and-access R3: an email belongs to one User, whatever its letter case.
CREATE UNIQUE INDEX users_email_key ON users (lower(email));

-- users-and-access R9: exactly one Owner, whatever the code above this does.
CREATE UNIQUE INDEX users_one_owner_key ON users (role) WHERE role = 'owner';

-- Only hashes of tokens are stored: a copy of this table logs nobody in.
CREATE TABLE sessions (
    token_hash bytea PRIMARY KEY,
    user_id bigint NOT NULL REFERENCES users (id),
    expires_at timestamptz NOT NULL
);

-- One row per User, so issuing a Setup link replaces the earlier one and only the newest
-- works (context ADR 0002). Using a link deletes its row.
CREATE TABLE setup_links (
    user_id bigint PRIMARY KEY REFERENCES users (id),
    token_hash bytea NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL
);

-- +goose Down
DROP TABLE setup_links;
DROP TABLE sessions;
DROP TABLE users;
