-- +goose Up
CREATE TABLE activities (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id         INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title           TEXT NOT NULL DEFAULT '',
    description     TEXT NOT NULL DEFAULT '',
    sport           TEXT NOT NULL,                 -- run|ride|swim|hike|walk|ski|row|other
    started_at      INTEGER NOT NULL,
    elapsed_time_s  INTEGER NOT NULL,
    moving_time_s   INTEGER NOT NULL,
    distance_m      REAL NOT NULL DEFAULT 0,
    elevation_gain_m REAL NOT NULL DEFAULT 0,
    elevation_loss_m REAL NOT NULL DEFAULT 0,
    avg_speed_mps   REAL,
    max_speed_mps   REAL,
    avg_heart_rate  INTEGER,
    max_heart_rate  INTEGER,
    avg_cadence     REAL,
    max_cadence     INTEGER,
    avg_power_w     REAL,
    max_power_w     INTEGER,
    has_gps         BOOLEAN NOT NULL DEFAULT 0,
    route_hidden    BOOLEAN NOT NULL DEFAULT 0,
    visibility      TEXT NOT NULL DEFAULT 'default', -- default|everyone|followers|only_me
    source          TEXT NOT NULL,                 -- manual|api|intervals
    source_ref      TEXT,
    dedupe_hash     TEXT NOT NULL,
    created_at      INTEGER NOT NULL,
    updated_at      INTEGER NOT NULL,
    UNIQUE (user_id, dedupe_hash)
);
CREATE INDEX activities_user_started ON activities(user_id, started_at DESC, id DESC);
CREATE UNIQUE INDEX activities_source_ref ON activities(user_id, source, source_ref)
    WHERE source_ref IS NOT NULL;

CREATE TABLE activity_points (
    activity_id INTEGER NOT NULL REFERENCES activities(id) ON DELETE CASCADE,
    seq         INTEGER NOT NULL,
    t           INTEGER NOT NULL,          -- unix seconds UTC
    elapsed_s   INTEGER NOT NULL,          -- seconds since started_at (pauses included)
    lat         REAL,                      -- NULL unless kept by RDP
    lon         REAL,
    ele         REAL,                      -- metres
    hr          INTEGER,
    cad         INTEGER,
    pwr         INTEGER,
    spd         REAL,                      -- m/s
    dist_m      REAL,                      -- cumulative metres
    PRIMARY KEY (activity_id, seq)
) WITHOUT ROWID;

CREATE TABLE activity_tombstones (
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    dedupe_hash TEXT NOT NULL,
    deleted_at  INTEGER NOT NULL,
    PRIMARY KEY (user_id, dedupe_hash)
) WITHOUT ROWID;

-- +goose Down
DROP TABLE activity_tombstones; DROP TABLE activity_points; DROP TABLE activities;
