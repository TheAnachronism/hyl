-- +goose Up
CREATE TABLE follows (
    follower_id  INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    followee_id  INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status       TEXT NOT NULL,          -- accepted | pending
    created_at   INTEGER NOT NULL,
    responded_at INTEGER,
    PRIMARY KEY (follower_id, followee_id),
    CHECK (follower_id <> followee_id)
) WITHOUT ROWID;
CREATE INDEX follows_followee ON follows(followee_id, status);

CREATE TABLE likes (
    activity_id INTEGER NOT NULL REFERENCES activities(id) ON DELETE CASCADE,
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at  INTEGER NOT NULL,
    PRIMARY KEY (activity_id, user_id)
) WITHOUT ROWID;

CREATE TABLE comments (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    activity_id INTEGER NOT NULL REFERENCES activities(id) ON DELETE CASCADE,
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    body        TEXT NOT NULL,
    created_at  INTEGER NOT NULL,
    deleted_at  INTEGER
);
CREATE INDEX comments_activity ON comments(activity_id, id);

CREATE TABLE comment_mentions (
    comment_id         INTEGER NOT NULL REFERENCES comments(id) ON DELETE CASCADE,
    mentioned_user_id  INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    PRIMARY KEY (comment_id, mentioned_user_id)
) WITHOUT ROWID;

CREATE TABLE notifications (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,  -- recipient
    actor_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind        TEXT NOT NULL,     -- follow|follow_request|follow_accepted|like|comment|mention
    activity_id INTEGER REFERENCES activities(id) ON DELETE CASCADE,
    comment_id  INTEGER REFERENCES comments(id) ON DELETE CASCADE,
    created_at  INTEGER NOT NULL,
    read_at     INTEGER
);
CREATE INDEX notifications_user ON notifications(user_id, id DESC);
CREATE INDEX notifications_unread ON notifications(user_id) WHERE read_at IS NULL;

-- +goose Down
DROP TABLE notifications; DROP TABLE comment_mentions; DROP TABLE comments;
DROP TABLE likes; DROP TABLE follows;
