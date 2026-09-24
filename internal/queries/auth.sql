-- name: CreateSession :one
INSERT INTO sessions (user_id, token_hash, created_at, expires_at, user_agent, ip)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetSessionByTokenHash :one
SELECT * FROM sessions WHERE token_hash = ?;

-- name: DeleteSession :execrows
DELETE FROM sessions WHERE token_hash = ?;

-- name: DeleteSessionsForUser :execrows
DELETE FROM sessions WHERE user_id = ?;

-- name: DeleteOtherSessionsForUser :execrows
DELETE FROM sessions WHERE user_id = ? AND token_hash <> ?;

-- name: DeleteExpiredSessions :execrows
DELETE FROM sessions WHERE expires_at < ?;

-- name: CreateEmailToken :one
INSERT INTO email_tokens (user_id, kind, token_hash, expires_at, created_at)
VALUES (?, ?, ?, ?, ?)
RETURNING id;

-- name: GetEmailToken :one
SELECT * FROM email_tokens WHERE token_hash = ? AND kind = ?;

-- name: ConsumeEmailToken :execrows
UPDATE email_tokens SET consumed_at = ? WHERE id = ? AND consumed_at IS NULL;

-- name: InvalidateEmailTokens :execrows
UPDATE email_tokens SET consumed_at = ? WHERE user_id = ? AND kind = ? AND consumed_at IS NULL;

-- name: CreateIdentity :one
INSERT INTO auth_identities (user_id, provider, provider_user_id, email, created_at)
VALUES (?, ?, ?, ?, ?)
RETURNING *;

-- name: GetIdentity :one
SELECT * FROM auth_identities WHERE provider = ? AND provider_user_id = ?;

-- name: ListIdentitiesForUser :many
SELECT * FROM auth_identities WHERE user_id = ? ORDER BY provider;

-- name: CountIdentitiesForUser :one
SELECT COUNT(*) FROM auth_identities WHERE user_id = ?;

-- name: DeleteIdentity :execrows
DELETE FROM auth_identities WHERE user_id = ? AND provider = ?;

-- name: UpdateSessionExpiry :execrows
UPDATE sessions SET created_at = ?, expires_at = ? WHERE id = ?;
