package db

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/markbeep/hyl/internal/config"
)

var schemaTables = []string{
	"activities", "activity_exports", "activity_points", "activity_tombstones",
	"api_keys", "auth_identities", "comment_mentions", "comments", "connections",
	"email_tokens", "follows", "goose_db_version", "import_rules", "likes",
	"media", "notifications", "privacy_zones", "sessions", "sync_runs", "users",
}

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	pool, err := Open(config.Config{DBPath: filepath.Join(t.TempDir(), "hyl.db")})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = pool.Close() })
	if err := Migrate(pool); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return pool
}

func tableNames(t *testing.T, pool *sql.DB) []string {
	t.Helper()
	rows, err := pool.Query(`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`)
	if err != nil {
		t.Fatalf("listing tables: %v", err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func gooseVersion(t *testing.T, pool *sql.DB) int64 {
	t.Helper()
	var v sql.NullInt64
	if err := pool.QueryRow(`SELECT MAX(version_id) FROM goose_db_version`).Scan(&v); err != nil {
		t.Fatalf("reading goose version: %v", err)
	}
	return v.Int64
}

// latestMigrationVersion is the highest migration number shipped. The migration
// test asserts "every shipped migration ran" against it instead of a literal,
// so adding a migration does not require editing the test.
func latestMigrationVersion(t *testing.T) int64 {
	t.Helper()
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		t.Fatalf("reading the embedded migrations: %v", err)
	}
	var latest int64
	for _, entry := range entries {
		prefix, _, ok := strings.Cut(entry.Name(), "_")
		if !ok {
			continue
		}
		version, err := strconv.ParseInt(prefix, 10, 64)
		if err != nil {
			t.Fatalf("migration %q has no numeric prefix: %v", entry.Name(), err)
		}
		if version > latest {
			latest = version
		}
	}
	if latest == 0 {
		t.Fatal("no migrations are embedded")
	}
	return latest
}

func TestMigrateUpIdempotentAndDown(t *testing.T) {
	pool := openTestDB(t)
	want := latestMigrationVersion(t)

	got := tableNames(t, pool)
	if len(got) != len(schemaTables) {
		t.Fatalf("table set mismatch:\n got %v\nwant %v", got, schemaTables)
	}
	for i, name := range schemaTables {
		if got[i] != name {
			t.Fatalf("table %d: got %q want %q", i, got[i], name)
		}
	}
	if v := gooseVersion(t, pool); v != want {
		t.Fatalf("goose version = %d, want %d", v, want)
	}

	// A second Migrate must be a no-op.
	if err := Migrate(pool); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	if v := gooseVersion(t, pool); v != want {
		t.Fatalf("goose version after re-run = %d, want %d", v, want)
	}

	if err := MigrateDown(pool); err != nil {
		t.Fatalf("MigrateDown: %v", err)
	}
	for _, name := range tableNames(t, pool) {
		if name == "users" || name == "activities" {
			t.Fatalf("table %q survived MigrateDown", name)
		}
	}

	// And the database is usable again after rolling forward.
	if err := Migrate(pool); err != nil {
		t.Fatalf("re-Migrate: %v", err)
	}
	if v := gooseVersion(t, pool); v != want {
		t.Fatalf("goose version after re-up = %d, want %d", v, want)
	}
}

// TestGeneratedCodeRoundTrip exercises the generated queries the way the
// handlers do: booleans, nullable columns, BLOBs and slices must all survive a
// write/read cycle through modernc's driver.
func TestGeneratedCodeRoundTrip(t *testing.T) {
	pool := openTestDB(t)
	ctx := context.Background()
	q := New(pool)

	hash := "phc-string"
	alice, err := q.CreateUser(ctx, CreateUserParams{
		Username: "alice", Email: "alice@example.com", DisplayName: "Alice",
		Bio: "", PasswordHash: &hash, EmailVerified: true,
		CreatedAt: 100, UpdatedAt: 100,
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if !alice.EmailVerified {
		t.Fatalf("booleans lost: %+v", alice)
	}
	bob, err := q.CreateUser(ctx, CreateUserParams{
		Username: "bob", Email: "bob@example.com", DisplayName: "Bob",
		Bio: "hi", PasswordHash: nil, EmailVerified: false,
		CreatedAt: 101, UpdatedAt: 101,
	})
	if err != nil {
		t.Fatalf("CreateUser bob: %v", err)
	}
	if bob.PasswordHash != nil {
		t.Fatalf("NULL password_hash came back as %v", *bob.PasswordHash)
	}

	// Sessions store a raw sha256.
	tokenHash := []byte{1, 2, 3, 4}
	if _, err := q.CreateSession(ctx, alice.ID, tokenHash, 1, 2, "ua", "127.0.0.1"); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	sess, err := q.GetSessionByTokenHash(ctx, tokenHash)
	if err != nil {
		t.Fatalf("GetSessionByTokenHash: %v", err)
	}
	if sess.UserID != alice.ID || len(sess.TokenHash) != 4 || sess.TokenHash[3] != 4 {
		t.Fatalf("session round trip: %+v", sess)
	}

	// Activities carry nullable metrics.
	act, err := q.CreateActivity(ctx, CreateActivityParams{
		UserID: bob.ID, Title: "Morning", Description: "", Sport: "ride",
		StartedAt: 1000, ElapsedTimeS: 3600, MovingTimeS: 3000,
		DistanceM: 40000, ElevationGainM: 120, ElevationLossM: 118,
		AvgSpeedMps: nil, MaxSpeedMps: nil, AvgHeartRate: nil, MaxHeartRate: nil,
		AvgCadence: nil, MaxCadence: nil, AvgPowerW: nil, MaxPowerW: nil,
		HasGps: true, RouteHidden: false, Visibility: "default",
		Source: "manual", SourceRef: nil, DedupeHash: "h1", CreatedAt: 1, UpdatedAt: 1,
	})
	if err != nil {
		t.Fatalf("CreateActivity: %v", err)
	}
	if act.AvgSpeedMps != nil || act.SourceRef != nil {
		t.Fatalf("nullable columns lost: %+v", act)
	}

	// The partial unique index on (user_id, source, source_ref) allows many NULLs.
	if _, err := q.CreateActivity(ctx, CreateActivityParams{
		UserID: bob.ID, Title: "Evening", Sport: "run", StartedAt: 2000,
		Source: "manual", DedupeHash: "h2", CreatedAt: 1, UpdatedAt: 1,
	}); err != nil {
		t.Fatalf("second NULL source_ref: %v", err)
	}

	// A duplicate dedupe hash is rejected.
	_, err = q.CreateActivity(ctx, CreateActivityParams{
		UserID: bob.ID, Title: "Dup", Sport: "ride", StartedAt: 1000,
		Source: "manual", DedupeHash: "h1", CreatedAt: 1, UpdatedAt: 1,
	})
	if err == nil {
		t.Fatal("duplicate dedupe_hash was accepted")
	}

	if err := q.CreateTombstone(ctx, bob.ID, "h1", 5); err != nil {
		t.Fatalf("CreateTombstone: %v", err)
	}
	if _, err := q.GetTombstone(ctx, bob.ID, "h1"); err != nil {
		t.Fatalf("GetTombstone: %v", err)
	}

	// Points load in one slice query, ordered per activity.
	for _, p := range []struct {
		seq int64
		lat float64
	}{{0, 51.5}, {1, 51.6}, {2, 51.7}} {
		if _, err := pool.ExecContext(ctx,
			`INSERT INTO activity_points (activity_id, seq, t, elapsed_s, lat, lon) VALUES (?, ?, ?, ?, ?, ?)`,
			act.ID, p.seq, 1000+p.seq, p.seq, p.lat, 4.9); err != nil {
			t.Fatalf("inserting point: %v", err)
		}
	}
	points, err := q.ListPointsForActivities(ctx, []int64{act.ID})
	if err != nil {
		t.Fatalf("ListPointsForActivities: %v", err)
	}
	if len(points) != 3 || points[0].Lat == nil || *points[0].Lat != 51.5 {
		t.Fatalf("points round trip: %+v", points)
	}
	// An empty slice must not produce an invalid query.
	if _, err := q.ListPointsForActivities(ctx, []int64{}); err != nil {
		t.Fatalf("ListPointsForActivities(empty): %v", err)
	}

	// The list query resolves visibility and aggregates likes.
	// feed_me = 1 restricts the page to the viewer's own activities.
	feed, err := q.ListActivities(ctx, ListActivitiesParams{
		ViewerID: bob.ID, FeedMe: 1, LimitCount: 10,
	})
	if err != nil {
		t.Fatalf("ListFeedActivities: %v", err)
	}
	if len(feed) != 2 {
		t.Fatalf("feed rows = %d, want 2", len(feed))
	}
	if feed[0].ID != act.ID+1 || feed[0].LikedByMe {
		t.Fatalf("feed ordering/aggregates: %+v", feed[0])
	}
	if feed[0].AvatarMediaID != 0 {
		t.Fatalf("avatar_media_id should default to 0, got %d", feed[0].AvatarMediaID)
	}
	if err := q.InsertLike(ctx, act.ID, bob.ID, 9); err != nil {
		t.Fatalf("InsertLike: %v", err)
	}
	if err := q.InsertLike(ctx, act.ID, bob.ID, 9); err != nil {
		t.Fatalf("InsertLike (idempotent): %v", err)
	}
	feed, err = q.ListActivities(ctx, ListActivitiesParams{
		ViewerID: bob.ID, FeedMe: 1, LimitCount: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !feed[1].LikedByMe || feed[1].LikeCount != 1 {
		t.Fatalf("like aggregation: %+v", feed[1])
	}

	// Media positions advance and the avatar subquery picks up the newest row.
	pos, err := q.NextMediaPosition(ctx, act.ID)
	if err != nil {
		t.Fatalf("NextMediaPosition: %v", err)
	}
	if pos != 0 {
		t.Fatalf("first media position = %d, want 0", pos)
	}
	if _, err := q.CreateMedia(ctx, CreateMediaParams{
		UserID: bob.ID, ActivityID: &act.ID, Kind: "activity", Position: pos,
		Width: 100, Height: 50, Bytes: 10, CreatedAt: 1,
	}); err != nil {
		t.Fatalf("CreateMedia: %v", err)
	}
	pos, err = q.NextMediaPosition(ctx, act.ID)
	if err != nil {
		t.Fatal(err)
	}
	if pos != 1 {
		t.Fatalf("second media position = %d, want 1", pos)
	}

	// Totals group per sport and honour the date range.
	totals, err := q.GetActivityTotalsBySport(ctx, bob.ID, 0, 0, bob.ID)
	if err != nil {
		t.Fatalf("GetActivityTotalsBySport: %v", err)
	}
	if len(totals) != 2 {
		t.Fatalf("sport totals = %+v, want 2 rows", totals)
	}
	rangeTotals, err := q.GetActivityTotalsBySport(ctx, bob.ID, 1500, 0, bob.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rangeTotals) != 1 || rangeTotals[0].Sport != "run" {
		t.Fatalf("date-filtered totals = %+v", rangeTotals)
	}

	// Username uniqueness is enforced case-insensitively.
	if _, err := q.CreateUser(ctx, CreateUserParams{
		Username: "ALICE", Email: "other@example.com", CreatedAt: 1, UpdatedAt: 1,
	}); err == nil {
		t.Fatal("case-variant username was accepted")
	}
	n, err := q.UsernameTaken(ctx, "Alice")
	if err != nil || n != 1 {
		t.Fatalf("UsernameTaken = %d, %v", n, err)
	}

	// Search matches prefixes of username or display name.
	found, err := q.SearchUsers(ctx, "ali", 8)
	if err != nil {
		t.Fatalf("SearchUsers: %v", err)
	}
	if len(found) != 1 || found[0].Username != "alice" {
		t.Fatalf("SearchUsers = %+v", found)
	}

	// Notification read marking is per user.
	if _, err := q.CreateNotification(ctx, alice.ID, bob.ID, "like", &act.ID, nil, 1); err != nil {
		t.Fatalf("CreateNotification: %v", err)
	}
	unread, err := q.CountUnreadNotifications(ctx, alice.ID)
	if err != nil || unread != 1 {
		t.Fatalf("CountUnreadNotifications = %d, %v", unread, err)
	}
	if _, err := q.MarkAllNotificationsRead(ctx, ptr(int64(10)), alice.ID); err != nil {
		t.Fatal(err)
	}
	unread, err = q.CountUnreadNotifications(ctx, alice.ID)
	if err != nil || unread != 0 {
		t.Fatalf("after mark-all unread = %d, %v", unread, err)
	}

	// Deleting an activity cascades to its children and is idempotent.
	if _, err := q.DeleteActivity(ctx, act.ID); err != nil {
		t.Fatalf("DeleteActivity: %v", err)
	}
	remaining, err := q.ListPointsForActivities(ctx, []int64{act.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 0 {
		t.Fatalf("points survived the cascade: %+v", remaining)
	}
	if _, err := q.GetActivity(ctx, act.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetActivity after delete: %v", err)
	}
}

func ptr[T any](v T) *T { return &v }
