-- +goose Up
CREATE TABLE connections (
    id                 INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id            INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind               TEXT NOT NULL,   -- intervals_oauth|intervals_apikey|strava_oauth
    external_athlete_id TEXT,
    access_token_cipher  BLOB,
    refresh_token_cipher BLOB,
    token_expires_at     INTEGER,
    auto_export        BOOLEAN NOT NULL DEFAULT 0,
    export_message     TEXT NOT NULL DEFAULT 'Imported from hyl',
    synced_from        INTEGER,          -- sync window start (unix seconds)
    last_success_at    INTEGER,
    last_error         TEXT,
    next_attempt_at    INTEGER,
    created_at         INTEGER NOT NULL,
    updated_at         INTEGER NOT NULL,
    UNIQUE (user_id, kind)
);

CREATE TABLE import_rules (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    connection_kind TEXT NOT NULL,
    sport       TEXT NOT NULL,          -- 'all' or a sport key
    enabled     BOOLEAN NOT NULL,
    updated_at  INTEGER NOT NULL,
    UNIQUE (user_id, connection_kind, sport)
);

CREATE TABLE activity_exports (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    activity_id INTEGER NOT NULL REFERENCES activities(id) ON DELETE CASCADE,
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    target      TEXT NOT NULL,          -- strava
    status      TEXT NOT NULL,          -- pending|sent|error
    external_id TEXT NOT NULL,
    remote_id   TEXT,
    attempts    INTEGER NOT NULL DEFAULT 0,
    last_error  TEXT,
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL,
    UNIQUE (activity_id, target)
);

CREATE TABLE sync_runs (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id       INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    connection_kind TEXT NOT NULL,
    started_at    INTEGER NOT NULL,
    finished_at   INTEGER,
    imported      INTEGER NOT NULL DEFAULT 0,
    skipped       INTEGER NOT NULL DEFAULT 0,
    error         TEXT
);
CREATE INDEX sync_runs_recent ON sync_runs(user_id, id DESC);

-- +goose Down
DROP TABLE sync_runs; DROP TABLE activity_exports; DROP TABLE import_rules; DROP TABLE connections;
