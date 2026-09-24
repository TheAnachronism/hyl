-- +goose Up
CREATE TABLE users (
    id                   INTEGER PRIMARY KEY AUTOINCREMENT,
    username             TEXT NOT NULL COLLATE NOCASE UNIQUE,
    email                TEXT NOT NULL COLLATE NOCASE UNIQUE,
    display_name         TEXT NOT NULL DEFAULT '',
    bio                  TEXT NOT NULL DEFAULT '',
    password_hash        TEXT,                      -- NULL for OAuth-only accounts
    email_verified       BOOLEAN NOT NULL DEFAULT 0,
    is_admin             BOOLEAN NOT NULL DEFAULT 0,
    profile_visibility   TEXT NOT NULL DEFAULT 'everyone',   -- everyone | followers
    activities_visibility TEXT NOT NULL DEFAULT 'followers', -- everyone | followers | only_me
    follow_policy        TEXT NOT NULL DEFAULT 'everyone',   -- everyone | on_request
    mention_policy       TEXT NOT NULL DEFAULT 'followers',  -- everyone | followers | nobody
    trim_scope           TEXT NOT NULL DEFAULT 'all',        -- all | zones
    trim_radius_m        INTEGER NOT NULL DEFAULT 200,
    created_at           INTEGER NOT NULL,
    updated_at           INTEGER NOT NULL
);

CREATE TABLE auth_identities (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id          INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider         TEXT NOT NULL,               -- google | github
    provider_user_id TEXT NOT NULL,
    email            TEXT,
    created_at       INTEGER NOT NULL,
    UNIQUE (provider, provider_user_id)
);
CREATE INDEX auth_identities_user ON auth_identities(user_id);

CREATE TABLE sessions (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash BLOB NOT NULL UNIQUE,
    created_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL,
    user_agent TEXT NOT NULL DEFAULT '',
    ip         TEXT NOT NULL DEFAULT ''
);
CREATE INDEX sessions_user ON sessions(user_id);

CREATE TABLE email_tokens (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind       TEXT NOT NULL,                     -- verify | reset
    token_hash BLOB NOT NULL UNIQUE,
    expires_at INTEGER NOT NULL,
    consumed_at INTEGER,
    created_at INTEGER NOT NULL
);
CREATE INDEX email_tokens_user ON email_tokens(user_id, kind);

-- +goose Down
DROP TABLE email_tokens; DROP TABLE sessions; DROP TABLE auth_identities; DROP TABLE users;
