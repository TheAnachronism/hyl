-- name: CreateMedia :one
INSERT INTO media (user_id, activity_id, kind, position, width, height, bytes, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetMedia :one
SELECT * FROM media WHERE id = ?;

-- name: ListActivityMedia :many
SELECT * FROM media
WHERE activity_id = CAST(sqlc.arg(activity_id) AS INTEGER) AND deleted_at IS NULL
ORDER BY position, id;

-- name: CountActivityMedia :one
SELECT COUNT(*) FROM media WHERE activity_id = CAST(sqlc.arg(activity_id) AS INTEGER) AND deleted_at IS NULL;

-- name: NextMediaPosition :one
SELECT CAST(COALESCE(MAX(position), -1) + 1 AS INTEGER) AS next_position
FROM media WHERE activity_id = CAST(sqlc.arg(activity_id) AS INTEGER);

-- name: SoftDeleteMedia :execrows
UPDATE media SET deleted_at = ? WHERE id = ? AND deleted_at IS NULL;

-- name: GetActiveAvatar :one
SELECT * FROM media WHERE user_id = ? AND kind = 'avatar' AND deleted_at IS NULL;

-- name: UpdateMediaSize :execrows
UPDATE media SET width = ?, height = ?, bytes = ? WHERE id = ?;
