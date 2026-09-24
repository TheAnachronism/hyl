-- name: CreateActivity :one
INSERT INTO activities (
    user_id, title, description, sport, external_sport, started_at, elapsed_time_s, moving_time_s,
    distance_m, elevation_gain_m, elevation_loss_m, avg_speed_mps, max_speed_mps,
    avg_heart_rate, max_heart_rate, avg_cadence, max_cadence, avg_power_w, max_power_w,
    has_gps, route_hidden, visibility, source, source_ref, dedupe_hash, created_at, updated_at
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
)
RETURNING *;

-- name: GetActivity :one
SELECT * FROM activities WHERE id = ?;

-- name: GetActivityBySourceRef :one
SELECT * FROM activities WHERE user_id = ? AND source = ? AND source_ref = ?;

-- name: GetActivityByDedupe :one
SELECT * FROM activities WHERE user_id = ? AND dedupe_hash = ?;

-- name: UpdateActivity :one
UPDATE activities
SET title = ?, description = ?, sport = ?, visibility = ?, route_hidden = ?, updated_at = ?
WHERE id = ?
RETURNING *;

-- name: DeleteActivity :execrows
DELETE FROM activities WHERE id = ?;

-- name: CountUserActivities :one
SELECT COUNT(*) FROM activities WHERE user_id = ?;

-- name: ListActivityPoints :many
SELECT * FROM activity_points WHERE activity_id = ? ORDER BY seq;

-- name: ListPointsForActivities :many
SELECT * FROM activity_points
WHERE activity_id IN (sqlc.slice('activity_ids'))
ORDER BY activity_id, seq;

-- name: GetTombstone :one
SELECT * FROM activity_tombstones WHERE user_id = ? AND dedupe_hash = ?;

-- name: CreateTombstone :exec
INSERT INTO activity_tombstones (user_id, dedupe_hash, deleted_at)
VALUES (?, ?, ?)
ON CONFLICT (user_id, dedupe_hash) DO UPDATE SET deleted_at = excluded.deleted_at;

-- The CASE expression below is the SQL twin of social.VisibilityAllows: it
-- resolves an activity's `default` visibility to its owner's
-- activities_visibility, then decides whether the viewer is allowed. The two
-- must keep agreeing, which TestVisibilityAgreesWithSQL asserts. CountActivities
-- repeats this predicate verbatim and must stay in step with it too, or a
-- page's total would disagree with the rows the list can return.
--
-- One query serves every activity list:
--   home feed        owner_id = 0, feed_me = 0, following_only = 1
--   feed=me          owner_id = 0, feed_me = 1, following_only = 0
--   a profile page   owner_id = <profile>, feed_me = 0, following_only = 0
--
-- Rows are ordered by when the activity happened (started_at DESC, id DESC for
-- the tiebreak), not by insertion order, so a late import (intervals sync) does
-- not jump to the top of the feed. Activity lists are page-numbered: the
-- handler turns ?page= into LIMIT/OFFSET and pairs it with the CountActivities
-- total, so page N always holds the same rows because the sort order is total.
--
-- search = '' disables the text filter; otherwise it matches title or
-- description with a case-insensitive LIKE. It is applied after the visibility
-- predicate and can never widen what the viewer may see.
--
-- name: ListActivities :many
SELECT a.id, a.user_id, u.username, u.display_name, a.title, a.description, a.sport, a.started_at,
       a.distance_m, a.moving_time_s, a.elapsed_time_s, a.elevation_gain_m,
       a.avg_speed_mps, a.max_speed_mps, a.avg_heart_rate, a.max_heart_rate,
       a.avg_cadence, a.avg_power_w, a.has_gps, a.route_hidden, a.visibility,
       u.trim_scope, u.trim_radius_m,
       CAST(COALESCE((SELECT m.id FROM media m WHERE m.user_id = u.id AND m.kind = 'avatar'
                 AND m.deleted_at IS NULL ORDER BY m.id DESC LIMIT 1), 0) AS INTEGER) AS avatar_media_id,
       (SELECT COUNT(*) FROM likes l WHERE l.activity_id = a.id) AS like_count,
       (SELECT COUNT(*) FROM comments c WHERE c.activity_id = a.id AND c.deleted_at IS NULL) AS comment_count,
       EXISTS (SELECT 1 FROM likes l2 WHERE l2.activity_id = a.id AND l2.user_id = sqlc.arg(viewer_id)) AS liked_by_me,
       (SELECT COUNT(*) FROM media m2 WHERE m2.activity_id = a.id AND m2.deleted_at IS NULL) AS photo_count
FROM activities a
JOIN users u ON u.id = a.user_id
WHERE (CAST(sqlc.arg(owner_id) AS INTEGER) = 0 OR a.user_id = CAST(sqlc.arg(owner_id) AS INTEGER))
  AND (CAST(sqlc.arg(feed_me) AS INTEGER) = 0 OR a.user_id = sqlc.arg(viewer_id))
  AND (CAST(sqlc.arg(following_only) AS INTEGER) = 0
       OR a.user_id = sqlc.arg(viewer_id)
       OR a.user_id IN (SELECT f.followee_id FROM follows f
                        WHERE f.follower_id = sqlc.arg(viewer_id) AND f.status = 'accepted'))
  AND (CAST(sqlc.arg(sport_filter) AS TEXT) = '' OR a.sport = CAST(sqlc.arg(sport_filter) AS TEXT))
  AND (CAST(sqlc.arg(search) AS TEXT) = ''
       OR a.title LIKE '%' || CAST(sqlc.arg(search) AS TEXT) || '%'
       OR a.description LIKE '%' || CAST(sqlc.arg(search) AS TEXT) || '%')
  AND (a.user_id = sqlc.arg(viewer_id)
       OR (CASE a.visibility WHEN 'everyone' THEN 'everyone' WHEN 'followers' THEN 'followers'
             WHEN 'only_me' THEN 'only_me' ELSE u.activities_visibility END) = 'everyone'
       OR ((CASE a.visibility WHEN 'everyone' THEN 'everyone' WHEN 'followers' THEN 'followers'
              WHEN 'only_me' THEN 'only_me' ELSE u.activities_visibility END) = 'followers'
           AND EXISTS (SELECT 1 FROM follows f2 WHERE f2.follower_id = sqlc.arg(viewer_id)
                       AND f2.followee_id = a.user_id AND f2.status = 'accepted')))
ORDER BY a.started_at DESC, a.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- CountActivities counts exactly the rows ListActivities can return for the same
-- arguments, with no LIMIT/OFFSET. Its WHERE clause is the visibility twin of
-- social.VisibilityAllows, identical to ListActivities above, and the two must
-- stay in step so a page's total matches its rows.
-- name: CountActivities :one
SELECT COUNT(*) AS total
FROM activities a
JOIN users u ON u.id = a.user_id
WHERE (CAST(sqlc.arg(owner_id) AS INTEGER) = 0 OR a.user_id = CAST(sqlc.arg(owner_id) AS INTEGER))
  AND (CAST(sqlc.arg(feed_me) AS INTEGER) = 0 OR a.user_id = sqlc.arg(viewer_id))
  AND (CAST(sqlc.arg(following_only) AS INTEGER) = 0
       OR a.user_id = sqlc.arg(viewer_id)
       OR a.user_id IN (SELECT f.followee_id FROM follows f
                        WHERE f.follower_id = sqlc.arg(viewer_id) AND f.status = 'accepted'))
  AND (CAST(sqlc.arg(sport_filter) AS TEXT) = '' OR a.sport = CAST(sqlc.arg(sport_filter) AS TEXT))
  AND (CAST(sqlc.arg(search) AS TEXT) = ''
       OR a.title LIKE '%' || CAST(sqlc.arg(search) AS TEXT) || '%'
       OR a.description LIKE '%' || CAST(sqlc.arg(search) AS TEXT) || '%')
  AND (a.user_id = sqlc.arg(viewer_id)
       OR (CASE a.visibility WHEN 'everyone' THEN 'everyone' WHEN 'followers' THEN 'followers'
             WHEN 'only_me' THEN 'only_me' ELSE u.activities_visibility END) = 'everyone'
       OR ((CASE a.visibility WHEN 'everyone' THEN 'everyone' WHEN 'followers' THEN 'followers'
              WHEN 'only_me' THEN 'only_me' ELSE u.activities_visibility END) = 'followers'
           AND EXISTS (SELECT 1 FROM follows f2 WHERE f2.follower_id = sqlc.arg(viewer_id)
                       AND f2.followee_id = a.user_id AND f2.status = 'accepted')));

-- Same visibility predicate as ListActivities above.
-- name: GetActivityTotalsBySport :many
SELECT a.sport,
       COUNT(*) AS activity_count,
       CAST(COALESCE(SUM(a.distance_m), 0) AS REAL) AS distance_m,
       CAST(COALESCE(SUM(a.elevation_gain_m), 0) AS REAL) AS elevation_gain_m,
       CAST(COALESCE(SUM(a.moving_time_s), 0) AS INTEGER) AS moving_time_s,
       CAST(COALESCE(SUM(a.elapsed_time_s), 0) AS INTEGER) AS elapsed_time_s
FROM activities a
JOIN users u ON u.id = a.user_id
WHERE a.user_id = sqlc.arg(owner_id)
  AND (CAST(sqlc.arg(from_ts) AS INTEGER) = 0 OR a.started_at >= CAST(sqlc.arg(from_ts) AS INTEGER))
  AND (CAST(sqlc.arg(to_ts) AS INTEGER) = 0 OR a.started_at < CAST(sqlc.arg(to_ts) AS INTEGER))
  AND (a.user_id = sqlc.arg(viewer_id)
       OR (CASE a.visibility WHEN 'everyone' THEN 'everyone' WHEN 'followers' THEN 'followers'
             WHEN 'only_me' THEN 'only_me' ELSE u.activities_visibility END) = 'everyone'
       OR ((CASE a.visibility WHEN 'everyone' THEN 'everyone' WHEN 'followers' THEN 'followers'
              WHEN 'only_me' THEN 'only_me' ELSE u.activities_visibility END) = 'followers'
           AND EXISTS (SELECT 1 FROM follows f2 WHERE f2.follower_id = sqlc.arg(viewer_id)
                       AND f2.followee_id = a.user_id AND f2.status = 'accepted')))
GROUP BY a.sport
ORDER BY a.sport;
