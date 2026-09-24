package activity

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/markbeep/hyl/internal/config"
	"github.com/markbeep/hyl/internal/db"
)

// newStoreForTest opens a migrated temporary database with one user, which the
// ingest tests own without touching the filesystem beyond the temp dir.
func newStoreForTest(t *testing.T) (*Store, *db.Queries, int64) {
	t.Helper()
	pool, err := db.Open(config.Config{DBPath: filepath.Join(t.TempDir(), "hyl.db")})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = pool.Close() })
	if err := db.Migrate(pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	queries := db.New(pool)
	user, err := queries.CreateUser(context.Background(), db.CreateUserParams{
		Username: "ingestuser", Email: "ingest@example.com", DisplayName: "ingest",
		CreatedAt: 1, UpdatedAt: 1,
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	return NewStore(pool, zap.NewNop()), queries, user.ID
}

// ingestedFIT builds a payload our FIT reader accepts, so the test exercises the
// real parse-and-store path.
func ingestedFIT(t *testing.T, start time.Time, distance float64) []byte {
	t.Helper()
	stored := db.Activity{
		Sport: SportRide, StartedAt: start.Unix(), ElapsedTimeS: 600, MovingTimeS: 600, DistanceM: distance,
	}
	var points []Point
	for i := range 60 {
		lat := 52.0 + float64(i)*0.0002
		lon := 5.0
		dist := distance * float64(i) / 60
		speed := distance / 600
		points = append(points, Point{
			Seq: int64(i), T: start.Add(time.Duration(i*10) * time.Second).Unix(), ElapsedS: int64(i * 10),
			Lat: &lat, Lon: &lon, DistM: &dist, Spd: &speed,
		})
	}
	var buffer bytes.Buffer
	if err := WriteFIT(&buffer, stored, points); err != nil {
		t.Fatalf("WriteFIT: %v", err)
	}
	return buffer.Bytes()
}

// TestIngestCallsExportQueueOnce pins the contract every ingestion path relies
// on: the hook fires exactly once, for the stored activity, and not for a
// duplicate the guards refused.
func TestIngestCallsExportQueueOnce(t *testing.T) {
	store, _, userID := newStoreForTest(t)
	ctx := context.Background()
	start := time.Date(2026, 4, 1, 7, 30, 0, 0, time.UTC)
	payload := ingestedFIT(t, start, 30000)

	var calls []db.Activity
	store.ExportQueue = func(_ context.Context, a db.Activity) error {
		calls = append(calls, a)
		return nil
	}

	stored, err := store.Ingest(ctx, userID, bytes.NewReader(payload), IngestOptions{})
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("hook called %d times, want 1", len(calls))
	}
	if calls[0].ID != stored.ID || calls[0].UserID != userID {
		t.Fatalf("hook saw %+v, want activity %d of user %d", calls[0], stored.ID, userID)
	}

	// The second upload is a duplicate: it is refused before the insert, so the
	// hook must not fire a second time.
	if _, err := store.Ingest(ctx, userID, bytes.NewReader(payload), IngestOptions{}); err == nil {
		t.Fatal("a duplicate ingest succeeded")
	}
	if len(calls) != 1 {
		t.Fatalf("hook called %d times after a duplicate, want 1", len(calls))
	}
}

// TestIngestSurvivesExportQueueProblems proves an automatic export can never
// break an upload: no hook and a failing hook both still store the activity.
func TestIngestSurvivesExportQueueProblems(t *testing.T) {
	store, queries, userID := newStoreForTest(t)
	ctx := context.Background()
	start := time.Date(2026, 4, 2, 7, 30, 0, 0, time.UTC)

	if _, err := store.Ingest(ctx, userID, bytes.NewReader(ingestedFIT(t, start, 5000)), IngestOptions{}); err != nil {
		t.Fatalf("Ingest with a nil hook: %v", err)
	}

	store.ExportQueue = func(context.Context, db.Activity) error {
		return errors.New("strava is unreachable")
	}
	stored, err := store.Ingest(ctx, userID, bytes.NewReader(ingestedFIT(t, start.Add(time.Hour), 6000)), IngestOptions{})
	if err != nil {
		t.Fatalf("Ingest with a failing hook: %v", err)
	}
	if _, err := queries.GetActivity(ctx, stored.ID); err != nil {
		t.Fatalf("the activity was not stored: %v", err)
	}
}
