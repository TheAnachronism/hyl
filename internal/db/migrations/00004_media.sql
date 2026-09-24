-- +goose Up
CREATE TABLE media (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    activity_id INTEGER REFERENCES activities(id) ON DELETE CASCADE,
    kind        TEXT NOT NULL,          -- avatar | activity
    position    INTEGER NOT NULL DEFAULT 0,
    width       INTEGER NOT NULL,
    height      INTEGER NOT NULL,
    bytes       INTEGER NOT NULL,
    created_at  INTEGER NOT NULL,
    deleted_at  INTEGER
);
CREATE INDEX media_activity ON media(activity_id, position) WHERE deleted_at IS NULL;
CREATE INDEX media_avatar ON media(user_id) WHERE kind = 'avatar' AND deleted_at IS NULL;

-- +goose Down
DROP TABLE media;
