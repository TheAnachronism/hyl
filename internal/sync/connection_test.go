package sync

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"go.uber.org/zap"

	"github.com/markbeep/hyl/internal/config"
	"github.com/markbeep/hyl/internal/db"
	"github.com/markbeep/hyl/internal/secrets"
)

// disconnectHarness is an account with both providers, one import rule on each,
// and a Strava export in each status. The fake records the best-effort revoke.
type disconnectHarness struct {
	connections  *Connections
	queries      *db.Queries
	pool         *sql.DB
	user         db.User
	pending      db.Activity
	sent         db.Activity
	failed       db.Activity
	revokes      int
	revokeStatus int
}

func newDisconnectHarness(t *testing.T) *disconnectHarness {
	t.Helper()
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "hyl.db")
	pool, err := db.Open(config.Config{DBPath: dbPath})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = pool.Close() })
	if err := db.Migrate(pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	cfg := config.Config{
		BaseURL: "http://localhost:8080", Version: "test",
		SecretKey:          "0123456789abcdef0123456789abcdef",
		StravaClientID:     "client-id",
		StravaClientSecret: "client-secret",
	}
	cipher, err := secrets.New(cfg.SecretKey)
	if err != nil {
		t.Fatal(err)
	}
	queries := db.New(pool)

	user, err := queries.CreateUser(ctx, db.CreateUserParams{
		Username: "athlete", Email: "athlete@example.com", DisplayName: "Athlete", CreatedAt: 1, UpdatedAt: 1,
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	access, err := cipher.EncryptString("intervals-token")
	if err != nil {
		t.Fatal(err)
	}
	refresh, err := cipher.EncryptString("strava-refresh")
	if err != nil {
		t.Fatal(err)
	}
	athleteID := "42"
	for _, kind := range []string{KindIntervalsOAuth, KindIntervalsAPIKey, KindStravaOAuth} {
		token := access
		var rotated []byte
		if kind == KindStravaOAuth {
			token = refresh
			rotated = refresh
		}
		if _, err := queries.UpsertConnection(ctx, db.UpsertConnectionParams{
			UserID: user.ID, Kind: kind, ExternalAthleteID: &athleteID,
			AccessTokenCipher: token, RefreshTokenCipher: rotated,
			AutoExport: false, ExportMessage: "Imported from hyl", CreatedAt: 1, UpdatedAt: 1,
		}); err != nil {
			t.Fatalf("create %s: %v", kind, err)
		}
		if err := queries.UpsertImportRule(ctx, user.ID, kind, "ride", true, 1); err != nil {
			t.Fatalf("rule %s: %v", kind, err)
		}
	}

	pending := insertActivity(t, queries, user.ID, "pending-hash")
	sent := insertActivity(t, queries, user.ID, "sent-hash")
	failed := insertActivity(t, queries, user.ID, "error-hash")
	queueExport(t, queries, pending, exportStatusPending)
	queueExport(t, queries, sent, exportStatusSent)
	queueExport(t, queries, failed, exportStatusError)

	h := &disconnectHarness{
		queries: queries, pool: pool, user: user,
		pending: pending, sent: sent, failed: failed,
		revokeStatus: http.StatusNoContent,
	}
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.revokes++
		w.WriteHeader(h.revokeStatus)
	}))
	t.Cleanup(fake.Close)
	cfg.StravaOAuthBase = fake.URL
	h.connections = &Connections{
		Pool: pool, Q: queries, Cfg: cfg, Log: zap.NewNop(), Cipher: cipher,
		intervalsBase: fake.URL, httpClient: fake.Client(),
	}
	return h
}

func insertActivity(t *testing.T, queries *db.Queries, userID int64, hash string) db.Activity {
	t.Helper()
	row, err := queries.CreateActivity(context.Background(), db.CreateActivityParams{
		UserID: userID, Title: hash, Sport: "ride", StartedAt: 1,
		ElapsedTimeS: 60, MovingTimeS: 60, DistanceM: 100, HasGps: false,
		Visibility: "default", Source: "manual", DedupeHash: hash, CreatedAt: 1, UpdatedAt: 1,
	})
	if err != nil {
		t.Fatalf("create activity: %v", err)
	}
	return row
}

func queueExport(t *testing.T, queries *db.Queries, activity db.Activity, status string) {
	t.Helper()
	ctx := context.Background()
	if _, err := queries.CreateExport(ctx, activity.ID, activity.UserID, exportTargetStrava, activity.DedupeHash, 1, 1); err != nil {
		t.Fatalf("queue export: %v", err)
	}
	if status == exportStatusPending {
		return
	}
	row, err := queries.GetExport(ctx, activity.ID, exportTargetStrava)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := queries.UpdateExportStatus(ctx, status, nil, 1, nil, 1, row.ID); err != nil {
		t.Fatalf("set export status: %v", err)
	}
}

func TestDisconnectIntervalsOAuthLeavesPendingStravaExport(t *testing.T) {
	h := newDisconnectHarness(t)
	ctx := context.Background()

	if err := h.connections.Disconnect(ctx, h.user.ID, KindIntervalsOAuth); err != nil {
		t.Fatalf("disconnect: %v", err)
	}

	if _, err := h.queries.GetConnection(ctx, h.user.ID, KindIntervalsOAuth); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("intervals connection err = %v, want gone", err)
	}
	if _, err := h.queries.GetConnection(ctx, h.user.ID, KindStravaOAuth); err != nil {
		t.Fatalf("strava connection: %v", err)
	}
	rules, err := h.queries.ListImportRules(ctx, h.user.ID)
	if err != nil {
		t.Fatal(err)
	}
	var stravaRule bool
	for _, rule := range rules {
		if rule.ConnectionKind == KindIntervalsOAuth {
			t.Fatalf("intervals import rule remained: %+v", rule)
		}
		if rule.ConnectionKind == KindStravaOAuth && rule.Sport == "ride" {
			stravaRule = true
		}
	}
	if !stravaRule {
		t.Fatal("strava import rule was removed")
	}
	row, err := h.queries.GetExport(ctx, h.pending.ID, exportTargetStrava)
	if err != nil {
		t.Fatalf("pending export: %v", err)
	}
	if row.Status != exportStatusPending {
		t.Fatalf("pending export status = %q", row.Status)
	}
	if h.revokes != 1 {
		t.Fatalf("provider revoke calls = %d, want 1", h.revokes)
	}
}

func TestDisconnectIntervalsAPIKeyLeavesStravaExports(t *testing.T) {
	h := newDisconnectHarness(t)
	ctx := context.Background()

	if err := h.connections.Disconnect(ctx, h.user.ID, KindIntervalsAPIKey); err != nil {
		t.Fatalf("disconnect: %v", err)
	}

	if _, err := h.queries.GetConnection(ctx, h.user.ID, KindIntervalsAPIKey); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("api key connection err = %v, want gone", err)
	}
	if _, err := h.queries.GetConnection(ctx, h.user.ID, KindStravaOAuth); err != nil {
		t.Fatalf("strava connection: %v", err)
	}
	if _, err := h.queries.GetConnection(ctx, h.user.ID, KindIntervalsOAuth); err != nil {
		t.Fatalf("intervals oauth connection: %v", err)
	}
	rules, err := h.queries.ListImportRules(ctx, h.user.ID)
	if err != nil {
		t.Fatal(err)
	}
	var oauthRule, stravaRule bool
	for _, rule := range rules {
		if rule.ConnectionKind == KindIntervalsAPIKey {
			t.Fatalf("api key import rule remained: %+v", rule)
		}
		if rule.ConnectionKind == KindIntervalsOAuth {
			oauthRule = true
		}
		if rule.ConnectionKind == KindStravaOAuth {
			stravaRule = true
		}
	}
	if !oauthRule || !stravaRule {
		t.Fatalf("other import rules missing: %+v", rules)
	}
	row, err := h.queries.GetExport(ctx, h.pending.ID, exportTargetStrava)
	if err != nil {
		t.Fatalf("pending export: %v", err)
	}
	if row.Status != exportStatusPending {
		t.Fatalf("pending export status = %q", row.Status)
	}
	if h.revokes != 0 {
		t.Fatalf("api key disconnect called the provider %d times", h.revokes)
	}
}

func TestDisconnectStravaRemovesPendingExportsAndKeepsHistory(t *testing.T) {
	h := newDisconnectHarness(t)
	ctx := context.Background()

	if err := h.connections.Disconnect(ctx, h.user.ID, KindStravaOAuth); err != nil {
		t.Fatalf("disconnect: %v", err)
	}

	if _, err := h.queries.GetConnection(ctx, h.user.ID, KindStravaOAuth); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("strava connection err = %v, want gone", err)
	}
	if _, err := h.queries.GetConnection(ctx, h.user.ID, KindIntervalsOAuth); err != nil {
		t.Fatalf("intervals connection: %v", err)
	}
	rules, err := h.queries.ListImportRules(ctx, h.user.ID)
	if err != nil {
		t.Fatal(err)
	}
	var intervalsRule bool
	for _, rule := range rules {
		if rule.ConnectionKind == KindStravaOAuth {
			t.Fatalf("strava import rule remained: %+v", rule)
		}
		if rule.ConnectionKind == KindIntervalsOAuth {
			intervalsRule = true
		}
	}
	if !intervalsRule {
		t.Fatal("intervals import rule was removed")
	}
	if _, err := h.queries.GetExport(ctx, h.pending.ID, exportTargetStrava); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("pending export err = %v, want gone", err)
	}
	sent, err := h.queries.GetExport(ctx, h.sent.ID, exportTargetStrava)
	if err != nil {
		t.Fatalf("sent export: %v", err)
	}
	if sent.Status != exportStatusSent {
		t.Fatalf("sent status = %q", sent.Status)
	}
	failed, err := h.queries.GetExport(ctx, h.failed.ID, exportTargetStrava)
	if err != nil {
		t.Fatalf("errored export: %v", err)
	}
	if failed.Status != exportStatusError {
		t.Fatalf("errored status = %q", failed.Status)
	}
	if h.revokes != 1 {
		t.Fatalf("provider revoke calls = %d, want 1", h.revokes)
	}
}

func TestDisconnectKeepsLocalRemovalWhenRevokeFails(t *testing.T) {
	h := newDisconnectHarness(t)
	h.revokeStatus = http.StatusInternalServerError
	ctx := context.Background()

	if err := h.connections.Disconnect(ctx, h.user.ID, KindIntervalsOAuth); err != nil {
		t.Fatalf("disconnect: %v", err)
	}
	if _, err := h.queries.GetConnection(ctx, h.user.ID, KindIntervalsOAuth); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("intervals connection err = %v, want gone", err)
	}
	if h.revokes != 1 {
		t.Fatalf("provider revoke calls = %d, want 1", h.revokes)
	}
}

func TestDisconnectTransactionFailureRemovesNothing(t *testing.T) {
	h := newDisconnectHarness(t)
	ctx := context.Background()

	// Abort the connection delete. Earlier deletes in the same transaction must
	// roll back with it; a delete that already committed would remain.
	if _, err := h.pool.Exec(`CREATE TRIGGER fail_connection_delete
		BEFORE DELETE ON connections
		BEGIN
			SELECT RAISE(ROLLBACK, 'connection delete failed');
		END`); err != nil {
		t.Fatal(err)
	}

	if err := h.connections.Disconnect(ctx, h.user.ID, KindIntervalsAPIKey); err == nil {
		t.Fatal("disconnect succeeded after the connection delete failed")
	}

	if _, err := h.queries.GetConnection(ctx, h.user.ID, KindIntervalsAPIKey); err != nil {
		t.Fatalf("api key connection: %v", err)
	}
	rules, err := h.queries.ListImportRules(ctx, h.user.ID)
	if err != nil {
		t.Fatal(err)
	}
	var apiRule bool
	for _, rule := range rules {
		if rule.ConnectionKind == KindIntervalsAPIKey {
			apiRule = true
		}
	}
	if !apiRule {
		t.Fatal("api key import rule was removed")
	}
	row, err := h.queries.GetExport(ctx, h.pending.ID, exportTargetStrava)
	if err != nil {
		t.Fatalf("pending export: %v", err)
	}
	if row.Status != exportStatusPending {
		t.Fatalf("pending export status = %q", row.Status)
	}
}
