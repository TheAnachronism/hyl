package social

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/markbeep/hyl/internal/config"
	"github.com/markbeep/hyl/internal/db"
)

// TestVisibilityAgreesWithSQL is the load-bearing check that the Go rule and
// the SQL predicate embedded in the list queries cannot drift apart. It builds
// a fixture matrix of owner account settings × activity overrides × viewer
// relationship and asserts both agree for every cell.
func TestVisibilityAgreesWithSQL(t *testing.T) {
	pool, err := db.Open(config.Config{DBPath: filepath.Join(t.TempDir(), "hyl.db")})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = pool.Close() }()
	if err := db.Migrate(pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	ctx := context.Background()
	queries := db.New(pool)

	owner := createUser(t, ctx, queries, "owner", "followers")
	follower := createUser(t, ctx, queries, "follower", "followers")
	stranger := createUser(t, ctx, queries, "stranger", "followers")

	// follower follows owner (accepted); stranger does not.
	if err := queries.UpsertFollow(ctx, follower.ID, owner.ID, "accepted", 1); err != nil {
		t.Fatalf("follow: %v", err)
	}

	overrides := []string{"default", "everyone", "followers", "only_me"}
	ownerDefaults := []string{"everyone", "followers", "only_me"}

	for _, ownerDefault := range ownerDefaults {
		if _, err := queries.UpdateUserPrivacy(ctx, db.UpdateUserPrivacyParams{
			ProfileVisibility: "everyone", ActivitiesVisibility: ownerDefault,
			FollowPolicy: "everyone", MentionPolicy: "followers",
			TrimScope: "all", TrimRadiusM: 200, UpdatedAt: 1, ID: owner.ID,
		}); err != nil {
			t.Fatalf("privacy: %v", err)
		}

		for _, override := range overrides {
			activity := createActivity(t, ctx, queries, owner.ID, override, ownerDefault+"/"+override)

			viewers := []struct {
				name string
				id   int64
			}{
				{"anonymous", 0},
				{"owner", owner.ID},
				{"follower", follower.ID},
				{"stranger", stranger.ID},
			}
			for _, viewer := range viewers {
				isFollower := viewer.id == follower.ID
				want := VisibilityAllows(viewer.id, owner.ID, isFollower, override, ownerDefault)

				rows, err := queries.ListActivities(ctx, db.ListActivitiesParams{
					ViewerID: viewer.id, OwnerID: owner.ID, LimitCount: 100,
				})
				if err != nil {
					t.Fatalf("list: %v", err)
				}
				got := false
				for _, row := range rows {
					if row.ID == activity.ID {
						got = true
						break
					}
				}
				if got != want {
					t.Errorf("owner=%s activity=%s viewer=%s: sql=%v go=%v",
						ownerDefault, override, viewer.name, got, want)
				}
			}
		}
	}
}

// TestVisibilityForAnonymousViewers covers the extra rule that an anonymous
// viewer sees only what the `everyone` visibility allows.
func TestVisibilityForAnonymousViewers(t *testing.T) {
	cases := []struct {
		override string
		owner    string
		want     bool
	}{
		{"default", "everyone", true},
		{"default", "followers", false},
		{"default", "only_me", false},
		{"everyone", "only_me", true},
		{"followers", "everyone", false},
		{"only_me", "everyone", false},
		{"unknown", "followers", false},
	}
	for _, tc := range cases {
		// An anonymous viewer can never be an accepted follower.
		got := VisibilityAllows(0, 42, false, tc.override, tc.owner)
		if got != tc.want {
			t.Errorf("anonymous override=%s owner=%s: got %v want %v", tc.override, tc.owner, got, tc.want)
		}
	}
}

func createUser(t *testing.T, ctx context.Context, queries *db.Queries, username, activitiesVisibility string) db.User {
	t.Helper()
	hash := "x"
	user, err := queries.CreateUser(ctx, db.CreateUserParams{
		Username: username, Email: username + "@example.com", DisplayName: username,
		PasswordHash: &hash, EmailVerified: true, IsAdmin: false, CreatedAt: 1, UpdatedAt: 1,
	})
	if err != nil {
		t.Fatalf("create user %s: %v", username, err)
	}
	if _, err := queries.UpdateUserPrivacy(ctx, db.UpdateUserPrivacyParams{
		ProfileVisibility: "everyone", ActivitiesVisibility: activitiesVisibility,
		FollowPolicy: "everyone", MentionPolicy: "followers",
		TrimScope: "all", TrimRadiusM: 200, UpdatedAt: 1, ID: user.ID,
	}); err != nil {
		t.Fatalf("privacy for %s: %v", username, err)
	}
	user, err = queries.GetUserByID(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	return user
}

func createActivity(t *testing.T, ctx context.Context, queries *db.Queries, ownerID int64, visibility, dedupe string) db.Activity {
	t.Helper()
	activity, err := queries.CreateActivity(ctx, db.CreateActivityParams{
		UserID: ownerID, Title: dedupe, Sport: "ride", StartedAt: 1000,
		ElapsedTimeS: 60, MovingTimeS: 60, DistanceM: 1000, HasGps: true,
		Visibility: visibility, Source: "manual", DedupeHash: dedupe, CreatedAt: 1, UpdatedAt: 1,
	})
	if err != nil {
		t.Fatalf("create activity %s: %v", dedupe, err)
	}
	return activity
}

var _ = sql.ErrNoRows
