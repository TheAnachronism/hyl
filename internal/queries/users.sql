-- name: GetUserByID :one
SELECT * FROM users WHERE id = ?;

-- name: GetUserByUsername :one
SELECT * FROM users WHERE username = ?;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = ?;

-- name: CreateUser :one
INSERT INTO users (
    username, email, display_name, bio, password_hash, email_verified, is_admin, created_at, updated_at
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?, ?
)
RETURNING *;

-- name: UpdateUserProfile :one
UPDATE users
SET display_name = ?, bio = ?, updated_at = ?
WHERE id = ?
RETURNING *;

-- name: UpdateUserPrivacy :one
UPDATE users
SET profile_visibility = ?, activities_visibility = ?, follow_policy = ?, mention_policy = ?,
    trim_scope = ?, trim_radius_m = ?, updated_at = ?
WHERE id = ?
RETURNING *;

-- name: UpdateUserPassword :execrows
UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?;

-- name: UpdateUserEmail :execrows
UPDATE users SET email = ?, email_verified = ?, updated_at = ? WHERE id = ?;

-- name: MarkEmailVerified :execrows
UPDATE users SET email_verified = 1, updated_at = ? WHERE id = ?;

-- name: UsernameTaken :one
SELECT COUNT(*) FROM users WHERE username = ?;

-- name: EmailTaken :one
SELECT COUNT(*) FROM users WHERE email = ?;

-- name: CountUsers :one
SELECT COUNT(*) FROM users;

-- name: SearchUsers :many
SELECT * FROM users
WHERE username LIKE CAST(sqlc.arg(query) AS TEXT) || '%' OR display_name LIKE CAST(sqlc.arg(query) AS TEXT) || '%'
ORDER BY username
LIMIT sqlc.arg(limit_count);
