-- name: UpsertFollow :exec
INSERT INTO follows (follower_id, followee_id, status, created_at)
VALUES (?, ?, ?, ?)
ON CONFLICT (follower_id, followee_id) DO UPDATE
    SET status = excluded.status, created_at = excluded.created_at, responded_at = NULL
    WHERE follows.status <> 'accepted';

-- name: GetFollow :one
SELECT * FROM follows WHERE follower_id = ? AND followee_id = ?;

-- name: DeleteFollow :execrows
DELETE FROM follows WHERE follower_id = ? AND followee_id = ?;

-- name: AcceptFollow :execrows
UPDATE follows SET status = 'accepted', responded_at = ?
WHERE follower_id = ? AND followee_id = ? AND status = 'pending';

-- name: CountFollowers :one
SELECT COUNT(*) FROM follows WHERE followee_id = ? AND status = 'accepted';

-- name: CountFollowing :one
SELECT COUNT(*) FROM follows WHERE follower_id = ? AND status = 'accepted';

-- name: InsertLike :exec
INSERT INTO likes (activity_id, user_id, created_at)
VALUES (?, ?, ?)
ON CONFLICT (activity_id, user_id) DO NOTHING;

-- name: CountLikes :one
SELECT COUNT(*) FROM likes WHERE activity_id = ?;

-- name: GetLike :one
SELECT * FROM likes WHERE activity_id = ? AND user_id = ?;

-- name: CreateComment :one
INSERT INTO comments (activity_id, user_id, body, created_at)
VALUES (?, ?, ?, ?)
RETURNING *;

-- name: GetComment :one
SELECT * FROM comments WHERE id = ?;

-- name: ListComments :many
SELECT c.id, c.activity_id, c.user_id, c.body, c.created_at,
       u.username, u.display_name,
       CAST(COALESCE((SELECT m.id FROM media m WHERE m.user_id = u.id AND m.kind = 'avatar'
                 AND m.deleted_at IS NULL ORDER BY m.id DESC LIMIT 1), 0) AS INTEGER) AS avatar_media_id
FROM comments c
JOIN users u ON u.id = c.user_id
WHERE c.activity_id = sqlc.arg(activity_id) AND c.deleted_at IS NULL
  AND (CAST(sqlc.arg(before_id) AS INTEGER) = 0 OR c.id < CAST(sqlc.arg(before_id) AS INTEGER))
ORDER BY c.id DESC
LIMIT sqlc.arg(limit_count);

-- name: SoftDeleteComment :execrows
UPDATE comments SET deleted_at = ? WHERE id = ? AND deleted_at IS NULL;

-- name: CreateMention :exec
INSERT INTO comment_mentions (comment_id, mentioned_user_id)
VALUES (?, ?)
ON CONFLICT (comment_id, mentioned_user_id) DO NOTHING;

-- name: ListMentionsForComments :many
SELECT cm.comment_id, u.username
FROM comment_mentions cm
JOIN users u ON u.id = cm.mentioned_user_id
WHERE cm.comment_id IN (sqlc.slice('comment_ids'))
ORDER BY cm.comment_id, u.username;

-- name: CreateNotification :one
INSERT INTO notifications (user_id, actor_id, kind, activity_id, comment_id, created_at)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: ListNotifications :many
SELECT n.id, n.kind, n.activity_id, n.comment_id, n.created_at, n.read_at,
       a.username AS actor_username, a.display_name AS actor_display_name,
       CAST(COALESCE((SELECT m.id FROM media m WHERE m.user_id = a.id AND m.kind = 'avatar'
                 AND m.deleted_at IS NULL ORDER BY m.id DESC LIMIT 1), 0) AS INTEGER) AS actor_avatar_media_id
FROM notifications n
JOIN users a ON a.id = n.actor_id
WHERE n.user_id = sqlc.arg(user_id)
  AND (CAST(sqlc.arg(before_id) AS INTEGER) = 0 OR n.id < CAST(sqlc.arg(before_id) AS INTEGER))
ORDER BY n.id DESC
LIMIT sqlc.arg(limit_count);

-- name: CountUnreadNotifications :one
SELECT COUNT(*) FROM notifications WHERE user_id = ? AND read_at IS NULL;

-- name: MarkNotificationRead :execrows
UPDATE notifications SET read_at = ? WHERE id = ? AND user_id = ? AND read_at IS NULL;

-- name: MarkAllNotificationsRead :execrows
UPDATE notifications SET read_at = ? WHERE user_id = ? AND read_at IS NULL;

-- name: CountComments :one
SELECT COUNT(*) FROM comments WHERE activity_id = ? AND deleted_at IS NULL;
