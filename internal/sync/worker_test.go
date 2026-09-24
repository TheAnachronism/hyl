package sync

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/markbeep/hyl/internal/activity"
	"github.com/markbeep/hyl/internal/config"
	"github.com/markbeep/hyl/internal/db"
	"github.com/markbeep/hyl/internal/secrets"
)

// fakeProvider stands in for intervals.icu: it serves pre-built FIT payloads
// and records how often it was asked for a file.
type fakeProvider struct {
	candidates []IntervalsActivity
	files      map[string][]byte
	downloads  int
	lists      int
	athleteIDs int
	listErr    error
	fileErr    error
	athleteID  string
	// athleteIDErr makes the resolution fail, the way a key without access
	// would.
	athleteIDErr error
}

func (f *fakeProvider) ListActivities(_ context.Context, _, oldest, newest string, _ int) ([]IntervalsActivity, error) {
	f.lists++
	if f.listErr != nil {
		return nil, f.listErr
	}
	// The real API only returns what falls inside the window, so the fake must
	// too; otherwise every 14-day chunk would re-offer the same activity.
	out := make([]IntervalsActivity, 0, len(f.candidates))
	for _, candidate := range f.candidates {
		day := candidate.StartDateLocal
		if day == "" || (day >= oldest && day <= newest) {
			out = append(out, candidate)
		}
	}
	return out, nil
}

// AthleteID stands in for intervals.icu's "who owns this credential" lookup.
func (f *fakeProvider) AthleteID(context.Context) (string, error) {
	f.athleteIDs++
	if f.athleteIDErr != nil {
		return "", f.athleteIDErr
	}
	return f.athleteID, nil
}

func (f *fakeProvider) DownloadFit(_ context.Context, activityID string) (io.ReadCloser, error) {
	f.downloads++
	if f.fileErr != nil {
		return nil, f.fileErr
	}
	payload, ok := f.files[activityID]
	if !ok {
		return nil, &ProviderError{StatusCode: 404, Message: "no file"}
	}
	return io.NopCloser(bytes.NewReader(payload)), nil
}

// harnessOptions selects the connections newHarness creates for its user. The
// intervals import connection is always present; the Strava connection is
// optional, because automatic export is governed by it alone.
type harnessOptions struct {
	strava           bool
	stravaAutoExport bool
	// onlyStrava drops the intervals connection, so a test can observe what the
	// worker does with an account that has nothing importable.
	onlyStrava bool
}

// newHarness wires a worker over a temporary database with a real activity
// store — including the automatic-export hook it calls after every ingest —
// and an injected provider.
func newHarness(t *testing.T, provider Provider, opts harnessOptions) (*Worker, *db.Queries, db.User) {
	t.Helper()
	pool, err := db.Open(config.Config{DBPath: filepath.Join(t.TempDir(), "hyl.db")})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = pool.Close() })
	if err := db.Migrate(pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	cfg := config.Config{
		BaseURL:      "http://localhost:8080",
		Version:      "test",
		SyncInterval: time.Minute,
		SecretKey:    "0123456789abcdef0123456789abcdef",
	}
	cipher, err := secrets.New(cfg.SecretKey)
	if err != nil {
		t.Fatal(err)
	}
	queries := db.New(pool)

	user, err := queries.CreateUser(context.Background(), db.CreateUserParams{
		Username: "syncuser", Email: "sync@example.com", DisplayName: "sync", CreatedAt: 1, UpdatedAt: 1,
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	athleteID := "0"
	if !opts.onlyStrava {
		sealed, err := cipher.EncryptString("api-key")
		if err != nil {
			t.Fatal(err)
		}
		// The intervals flag is always off: it has no bearing on exports, and
		// the tests below rely on that.
		if _, err := queries.UpsertConnection(context.Background(), db.UpsertConnectionParams{
			UserID: user.ID, Kind: KindIntervalsAPIKey, ExternalAthleteID: &athleteID,
			AccessTokenCipher: sealed, AutoExport: false, ExportMessage: "Imported from hyl",
			CreatedAt: 1, UpdatedAt: 1,
		}); err != nil {
			t.Fatalf("create connection: %v", err)
		}
	}

	if opts.strava {
		stravaToken, err := cipher.EncryptString("strava-token")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := queries.UpsertConnection(context.Background(), db.UpsertConnectionParams{
			UserID: user.ID, Kind: KindStravaOAuth, ExternalAthleteID: &athleteID,
			AccessTokenCipher: stravaToken, AutoExport: opts.stravaAutoExport,
			ExportMessage: "Imported from hyl", CreatedAt: 1, UpdatedAt: 1,
		}); err != nil {
			t.Fatalf("create strava connection: %v", err)
		}
	}

	store := activity.NewStore(pool, zap.NewNop())
	store.ExportQueue = NewExportQueuer(queries).Queue
	worker := NewWorker(pool, cfg, zap.NewNop(), cipher, store)
	worker.newProvider = func(db.Connection, string) Provider { return provider }
	return worker, queries, user
}

// TestStravaConnectionIsNeverImported is the regression test for the credential
// leak: the import worker only speaks the intervals.icu API, and a Strava row's
// stored credential is a Strava token, so the provider — which turns whatever
// secret it is handed into an intervals API key — must never see that row.
func TestStravaConnectionIsNeverImported(t *testing.T) {
	provider := &fakeProvider{}
	worker, queries, user := newHarness(t, provider, harnessOptions{strava: true, onlyStrava: true})

	worker.syncPass(context.Background(), 0)

	if provider.lists != 0 || provider.athleteIDs != 0 || provider.downloads != 0 {
		t.Fatalf("the intervals provider was called for a Strava connection: lists=%d athlete_ids=%d downloads=%d",
			provider.lists, provider.athleteIDs, provider.downloads)
	}
	runs, err := queries.ListRecentSyncRuns(context.Background(), user.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 0 {
		t.Fatalf("a Strava connection produced %d sync runs, want 0", len(runs))
	}
}

// TestStravaConnectionIsNotImportableInTheWindow guards the same rule one layer
// deeper, so a future caller that reaches importWindow directly still cannot
// leak the credential.
func TestStravaConnectionIsNotImportableInTheWindow(t *testing.T) {
	provider := &fakeProvider{}
	worker, queries, user := newHarness(t, provider, harnessOptions{strava: true, onlyStrava: true})

	conn, err := queries.GetConnection(context.Background(), user.ID, KindStravaOAuth)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := worker.importWindow(context.Background(), conn, time.Now()); err == nil {
		t.Fatal("importWindow accepted a Strava connection")
	}
	if provider.lists != 0 || provider.athleteIDs != 0 {
		t.Fatalf("the provider was called for a Strava connection: lists=%d athlete_ids=%d", provider.lists, provider.athleteIDs)
	}
}

// fitFixture builds a valid FIT payload with n points starting at a fixed time.
func fitFixture(t *testing.T, start time.Time, distance float64) []byte {
	t.Helper()
	stored := db.Activity{
		Sport: activity.SportRide, StartedAt: start.Unix(),
		ElapsedTimeS: 600, MovingTimeS: 600, DistanceM: distance,
	}
	var points []activity.Point
	for i := range 60 {
		lat := 52.0 + float64(i)*0.0002
		lon := 5.0
		dist := distance * float64(i) / 60
		speed := distance / 600
		timestamp := start.Add(time.Duration(i*10) * time.Second).Unix()
		points = append(points, activity.Point{
			Seq: int64(i), T: timestamp, ElapsedS: int64(i * 10),
			Lat: &lat, Lon: &lon, DistM: &dist, Spd: &speed,
		})
	}
	var buffer bytes.Buffer
	if err := activity.WriteFIT(&buffer, stored, points); err != nil {
		t.Fatalf("WriteFIT: %v", err)
	}
	return buffer.Bytes()
}

func TestWorkerImportsAndNeverDuplicates(t *testing.T) {
	start := time.Now().UTC().Add(-48 * time.Hour).Truncate(time.Hour)
	day := start.Format("2006-01-02")
	provider := &fakeProvider{
		candidates: []IntervalsActivity{
			{ID: "i100", Type: "Ride", Name: "First ride", Source: "UPLOAD", StartDateLocal: day},
			{ID: "i101", Type: "Run", Name: "Second run", Source: "UPLOAD", StartDateLocal: day},
		},
		files: map[string][]byte{
			"i100": fitFixture(t, start, 20000),
			"i101": fitFixture(t, start.Add(24*time.Hour), 8000),
		},
	}
	worker, queries, user := newHarness(t, provider, harnessOptions{})
	ctx := context.Background()

	worker.syncPass(ctx, user.ID)
	if provider.downloads != 2 {
		t.Fatalf("downloaded %d files, want 2", provider.downloads)
	}

	stored, err := queries.ListActivities(ctx, db.ListActivitiesParams{ViewerID: user.ID, FeedMe: 1, LimitCount: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 2 {
		t.Fatalf("imported %d activities, want 2", len(stored))
	}
	for _, row := range stored {
		full, err := queries.GetActivity(ctx, row.ID)
		if err != nil {
			t.Fatal(err)
		}
		if full.Source != KindIntervalsAPIKey || full.SourceRef == nil {
			t.Errorf("activity %d has source %q ref %v", row.ID, full.Source, full.SourceRef)
		}
	}

	conn, err := queries.GetConnection(ctx, user.ID, KindIntervalsAPIKey)
	if err != nil {
		t.Fatal(err)
	}
	if conn.SyncedFrom == nil {
		t.Fatal("the sync cursor was not advanced")
	}
	if conn.LastError != nil {
		t.Fatalf("connection has an error: %v", *conn.LastError)
	}

	// Second pass: the window has already been walked to "now", so nothing is
	// even listed and nothing is fetched again.
	before := provider.downloads
	worker.syncPass(ctx, user.ID)
	if provider.downloads != before {
		t.Fatalf("a second pass downloaded %d more files, want 0", provider.downloads-before)
	}
	runs, err := queries.ListRecentSyncRuns(ctx, user.ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	if runs[0].Imported != 0 {
		t.Fatalf("second run imported %d, want 0", runs[0].Imported)
	}

	// Rewinding the cursor forces the same window to be walked again: the
	// source_ref index must refuse every activity without downloading anything.
	if _, err := queries.UpdateConnectionSyncState(ctx, nil, nil, 2, connRecordID(t, queries, user.ID)); err != nil {
		t.Fatal(err)
	}
	before = provider.downloads
	worker.syncPass(ctx, user.ID)
	if provider.downloads != before {
		t.Fatalf("a re-walked window downloaded %d files, want 0", provider.downloads-before)
	}
	runs, err = queries.ListRecentSyncRuns(ctx, user.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if runs[0].Skipped != 2 || runs[0].Imported != 0 {
		t.Fatalf("re-walked run imported=%d skipped=%d, want 0/2", runs[0].Imported, runs[0].Skipped)
	}

	// Deleting an imported activity must keep it out: the tombstone refuses it.
	deleted, err := queries.GetActivity(ctx, stored[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := queries.DeleteActivity(ctx, deleted.ID); err != nil {
		t.Fatal(err)
	}
	if err := queries.CreateTombstone(ctx, user.ID, deleted.DedupeHash, time.Now().Unix()); err != nil {
		t.Fatal(err)
	}
	worker.syncPass(ctx, user.ID)
	remaining, err := queries.ListActivities(ctx, db.ListActivitiesParams{ViewerID: user.ID, FeedMe: 1, LimitCount: 10})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range remaining {
		if row.ID == stored[0].ID {
			t.Fatal("a deleted activity came back")
		}
	}
}

// connRecordID looks up the connection id the sync-state update needs.
func connRecordID(t *testing.T, queries *db.Queries, userID int64) int64 {
	t.Helper()
	conn, err := queries.GetConnection(context.Background(), userID, KindIntervalsAPIKey)
	if err != nil {
		t.Fatal(err)
	}
	return conn.ID
}

func TestWorkerSkipsStravaSourcedActivities(t *testing.T) {
	start := time.Now().UTC().Add(-24 * time.Hour).Truncate(time.Hour)
	day := start.Format("2006-01-02")
	provider := &fakeProvider{
		candidates: []IntervalsActivity{
			{ID: "s1", Type: "Ride", Source: "STRAVA", StartDateLocal: day},
			{ID: "s2", Type: "Ride", Source: "UPLOAD", StartDateLocal: day},
		},
		files: map[string][]byte{"s2": fitFixture(t, start, 15000)},
	}
	worker, queries, user := newHarness(t, provider, harnessOptions{})
	ctx := context.Background()

	worker.syncPass(ctx, user.ID)

	if provider.downloads != 1 {
		t.Fatalf("downloaded %d files, want only the non-Strava one", provider.downloads)
	}
	stored, err := queries.ListActivities(ctx, db.ListActivitiesParams{ViewerID: user.ID, FeedMe: 1, LimitCount: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 {
		t.Fatalf("imported %d activities, want 1", len(stored))
	}
}

func TestWorkerHonoursDisabledSportRule(t *testing.T) {
	start := time.Now().UTC().Add(-24 * time.Hour).Truncate(time.Hour)
	day := start.Format("2006-01-02")
	provider := &fakeProvider{
		candidates: []IntervalsActivity{
			{ID: "r1", Type: "Run", Source: "UPLOAD", StartDateLocal: day},
			{ID: "r2", Type: "Ride", Source: "UPLOAD", StartDateLocal: day},
		},
		files: map[string][]byte{
			"r1": fitFixture(t, start, 5000),
			"r2": fitFixture(t, start.Add(time.Hour), 20000),
		},
	}
	worker, queries, user := newHarness(t, provider, harnessOptions{})
	ctx := context.Background()

	if err := queries.UpsertImportRule(ctx, user.ID, KindIntervalsAPIKey, activity.SportRun, false, 1); err != nil {
		t.Fatal(err)
	}
	worker.syncPass(ctx, user.ID)

	if provider.downloads != 1 {
		t.Fatalf("downloaded %d files, want the ride only", provider.downloads)
	}
	stored, err := queries.ListActivities(ctx, db.ListActivitiesParams{ViewerID: user.ID, FeedMe: 1, LimitCount: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 || stored[0].Sport != activity.SportRide {
		t.Fatalf("imported %+v, want exactly the ride", stored)
	}
}

// TestWorkerEnqueuesExportsWhenAutoExportIsOn pins the gate: the Strava
// connection asks for automatic exports while the intervals connection does
// not, and the imported activity is still queued.
func TestWorkerEnqueuesExportsWhenAutoExportIsOn(t *testing.T) {
	start := time.Now().UTC().Add(-24 * time.Hour).Truncate(time.Hour)
	provider := &fakeProvider{
		candidates: []IntervalsActivity{{ID: "x1", Type: "Ride", Source: "UPLOAD", StartDateLocal: start.Format("2006-01-02")}},
		files:      map[string][]byte{"x1": fitFixture(t, start, 30000)},
	}
	worker, queries, user := newHarness(t, provider, harnessOptions{strava: true, stravaAutoExport: true})
	ctx := context.Background()

	worker.syncPass(ctx, user.ID)

	conn, err := queries.GetConnection(ctx, user.ID, KindIntervalsAPIKey)
	if err != nil {
		t.Fatal(err)
	}
	if conn.AutoExport {
		t.Fatal("the intervals connection must not drive exports")
	}

	pending, err := queries.ListPendingExports(ctx, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 {
		t.Fatalf("pending exports = %d, want 1", len(pending))
	}
	if pending[0].Target != "strava" || pending[0].ExternalID == "" {
		t.Fatalf("export row = %+v", pending[0])
	}
}

// TestWorkerDoesNotExportWhenStravaFlagIsOff proves the intervals connection's
// own flag is inert: a Strava connection that did not ask for exports queues
// nothing.
func TestWorkerDoesNotExportWhenStravaFlagIsOff(t *testing.T) {
	start := time.Now().UTC().Add(-24 * time.Hour).Truncate(time.Hour)
	provider := &fakeProvider{
		candidates: []IntervalsActivity{{ID: "x1", Type: "Ride", Source: "UPLOAD", StartDateLocal: start.Format("2006-01-02")}},
		files:      map[string][]byte{"x1": fitFixture(t, start, 30000)},
	}
	worker, queries, user := newHarness(t, provider, harnessOptions{strava: true})
	ctx := context.Background()

	worker.syncPass(ctx, user.ID)

	stored, err := queries.ListActivities(ctx, db.ListActivitiesParams{ViewerID: user.ID, FeedMe: 1, LimitCount: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 {
		t.Fatalf("imported %d activities, want 1", len(stored))
	}
	assertNoExports(t, queries)
}

// TestWorkerDoesNotExportWithoutStravaConnection covers the user who never
// connected Strava: ingesting intervals activities must not queue anything.
func TestWorkerDoesNotExportWithoutStravaConnection(t *testing.T) {
	start := time.Now().UTC().Add(-24 * time.Hour).Truncate(time.Hour)
	provider := &fakeProvider{
		candidates: []IntervalsActivity{{ID: "x1", Type: "Ride", Source: "UPLOAD", StartDateLocal: start.Format("2006-01-02")}},
		files:      map[string][]byte{"x1": fitFixture(t, start, 30000)},
	}
	worker, queries, user := newHarness(t, provider, harnessOptions{})
	ctx := context.Background()

	worker.syncPass(ctx, user.ID)

	stored, err := queries.ListActivities(ctx, db.ListActivitiesParams{ViewerID: user.ID, FeedMe: 1, LimitCount: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 {
		t.Fatalf("imported %d activities, want 1", len(stored))
	}
	assertNoExports(t, queries)
}

// assertNoExports fails when any export row exists for the harness database.
func assertNoExports(t *testing.T, queries *db.Queries) {
	t.Helper()
	pending, err := queries.ListPendingExports(context.Background(), 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending exports = %d, want 0", len(pending))
	}
}

func TestWorkerBacksOffAndMarksReauthorization(t *testing.T) {
	t.Run("rate limit honours Retry-After", func(t *testing.T) {
		provider := &fakeProvider{listErr: &ProviderError{
			StatusCode: 429, Message: "slow down", RetryAfter: 90 * time.Second,
		}}
		worker, queries, user := newHarness(t, provider, harnessOptions{})
		ctx := context.Background()

		worker.syncPass(ctx, user.ID)

		conn, err := queries.GetConnection(ctx, user.ID, KindIntervalsAPIKey)
		if err != nil {
			t.Fatal(err)
		}
		if conn.NextAttemptAt == nil {
			t.Fatal("no retry was scheduled")
		}
		delay := time.Until(time.Unix(*conn.NextAttemptAt, 0))
		if delay < 60*time.Second || delay > 120*time.Second {
			t.Fatalf("retry scheduled %s away, want about 90s", delay)
		}
		runs, err := queries.ListRecentSyncRuns(ctx, user.ID, 1)
		if err != nil {
			t.Fatal(err)
		}
		if runs[0].Error == nil || runs[0].FinishedAt == nil {
			t.Fatalf("failed run = %+v", runs[0])
		}
	})

	t.Run("401 on an oauth connection asks for reauthorization", func(t *testing.T) {
		provider := &fakeProvider{listErr: &ProviderError{StatusCode: 401, Message: "expired"}}
		worker, queries, user := newHarness(t, provider, harnessOptions{})
		ctx := context.Background()

		// Switch the connection to OAuth: that is the kind that cannot refresh.
		sealed, err := worker.cipher.EncryptString("oauth-token")
		if err != nil {
			t.Fatal(err)
		}
		athleteID := "i12345"
		if _, err := queries.UpsertConnection(ctx, db.UpsertConnectionParams{
			UserID: user.ID, Kind: KindIntervalsOAuth, ExternalAthleteID: &athleteID,
			AccessTokenCipher: sealed, ExportMessage: "Imported from hyl", CreatedAt: 1, UpdatedAt: 1,
		}); err != nil {
			t.Fatal(err)
		}

		worker.syncPass(ctx, user.ID)

		conn, err := queries.GetConnection(ctx, user.ID, KindIntervalsOAuth)
		if err != nil {
			t.Fatal(err)
		}
		if conn.LastError == nil || *conn.LastError != "reauthorize" {
			t.Fatalf("last error = %v, want reauthorize", conn.LastError)
		}
	})

	t.Run("a missing file is skipped rather than failing the pass", func(t *testing.T) {
		provider := &fakeProvider{
			candidates: []IntervalsActivity{{
				ID: "manual1", Type: "Ride", Source: "MANUAL",
				StartDateLocal: time.Now().UTC().Add(-24 * time.Hour).Format("2006-01-02"),
			}},
			files: map[string][]byte{},
		}
		worker, queries, user := newHarness(t, provider, harnessOptions{})
		ctx := context.Background()

		worker.syncPass(ctx, user.ID)

		conn, err := queries.GetConnection(ctx, user.ID, KindIntervalsAPIKey)
		if err != nil {
			t.Fatal(err)
		}
		if conn.LastError != nil {
			t.Fatalf("a manual entry failed the pass: %v", *conn.LastError)
		}
		runs, err := queries.ListRecentSyncRuns(ctx, user.ID, 1)
		if err != nil {
			t.Fatal(err)
		}
		if runs[0].Skipped != 1 || runs[0].Error != nil {
			t.Fatalf("run = %+v, want one skip and no error", runs[0])
		}
	})
}
