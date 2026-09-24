-- +goose Up
CREATE TABLE privacy_zones (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    label      TEXT NOT NULL,
    lat        REAL NOT NULL,
    lon        REAL NOT NULL,
    radius_m   INTEGER NOT NULL,
    created_at INTEGER NOT NULL
);
CREATE INDEX privacy_zones_user ON privacy_zones(user_id);

CREATE TABLE api_keys (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    key_hash     BLOB NOT NULL UNIQUE,
    prefix       TEXT NOT NULL,       -- first 8 chars, shown in the UI
    created_at   INTEGER NOT NULL,
    last_used_at INTEGER,
    revoked_at   INTEGER
);
CREATE INDEX api_keys_user ON api_keys(user_id);

-- +goose Down
DROP TABLE api_keys; DROP TABLE privacy_zones;
