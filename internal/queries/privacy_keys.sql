-- name: ListPrivacyZones :many
SELECT * FROM privacy_zones WHERE user_id = ? ORDER BY id;

-- name: CreatePrivacyZone :one
INSERT INTO privacy_zones (user_id, label, lat, lon, radius_m, created_at)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: CountPrivacyZones :one
SELECT COUNT(*) FROM privacy_zones WHERE user_id = ?;

-- name: DeletePrivacyZone :execrows
DELETE FROM privacy_zones WHERE id = ? AND user_id = ?;

-- name: CreateAPIKey :one
INSERT INTO api_keys (user_id, name, key_hash, prefix, created_at)
VALUES (?, ?, ?, ?, ?)
RETURNING *;

-- name: ListAPIKeys :many
SELECT * FROM api_keys WHERE user_id = ? AND revoked_at IS NULL ORDER BY id DESC;

-- name: GetAPIKeyByHash :one
SELECT * FROM api_keys WHERE key_hash = ? AND revoked_at IS NULL;

-- name: TouchAPIKey :exec
UPDATE api_keys SET last_used_at = ? WHERE id = ?;

-- name: RevokeAPIKey :execrows
UPDATE api_keys SET revoked_at = ? WHERE id = ? AND user_id = ?;
