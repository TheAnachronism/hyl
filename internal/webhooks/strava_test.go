package webhooks

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"github.com/markbeep/hyl/internal/config"
	"github.com/markbeep/hyl/internal/db"
	"github.com/markbeep/hyl/internal/secrets"
	syncpkg "github.com/markbeep/hyl/internal/sync"
)

const webhookAthleteID = "4242"

// newStravaHarness builds the handler over a temporary database holding one
// Strava connection, with Strava's token endpoint replaced by a fake.
func newStravaHarness(t *testing.T, tokenEndpoint http.HandlerFunc) (*Strava, *db.Queries, int64) {
	t.Helper()
	server := httptest.NewServer(tokenEndpoint)
	t.Cleanup(server.Close)

	pool, err := db.Open(config.Config{DBPath: filepath.Join(t.TempDir(), "hyl.db")})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = pool.Close() })
	if err := db.Migrate(pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	cfg := config.Config{
		SecretKey:                "0123456789abcdef0123456789abcdef",
		StravaClientID:           "client-id",
		StravaClientSecret:       "client-secret",
		StravaWebhookVerifyToken: "verify-token",
		StravaOAuthBase:          server.URL,
	}
	cipher, err := secrets.New(cfg.SecretKey)
	if err != nil {
		t.Fatal(err)
	}
	queries := db.New(pool)
	user, err := queries.CreateUser(context.Background(), db.CreateUserParams{
		Username: "athlete", Email: "athlete@example.com", DisplayName: "athlete", CreatedAt: 1, UpdatedAt: 1,
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	refresh, err := cipher.EncryptString("refresh-1")
	if err != nil {
		t.Fatal(err)
	}
	athleteID := webhookAthleteID
	if _, err := queries.UpsertConnection(context.Background(), db.UpsertConnectionParams{
		UserID: user.ID, Kind: syncpkg.KindStravaOAuth, ExternalAthleteID: &athleteID,
		RefreshTokenCipher: refresh, AutoExport: true, ExportMessage: "Imported from hyl",
		CreatedAt: 1, UpdatedAt: 1,
	}); err != nil {
		t.Fatalf("create connection: %v", err)
	}
	return NewStrava(pool, cfg, zap.NewNop(), cipher), queries, user.ID
}

// postDeauthorization delivers the event body Strava sends when an athlete
// revokes access.
func postDeauthorization(t *testing.T, handler *Strava) *httptest.ResponseRecorder {
	t.Helper()
	body := `{"object_type":"athlete","aspect_type":"update","object_id":` + webhookAthleteID +
		`,"owner_id":` + webhookAthleteID + `,"updates":{"authorized":"false"}}`
	req := httptest.NewRequest(http.MethodPost, "/webhooks/strava", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	if err := handler.Handle(echo.New().NewContext(req, rec)); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	return rec
}

func connectionCount(t *testing.T, queries *db.Queries, userID int64) int {
	t.Helper()
	connections, err := queries.ListConnectionsForUser(context.Background(), userID)
	if err != nil {
		t.Fatal(err)
	}
	return len(connections)
}

// TestForgedDeauthorizationIsIgnored is the regression test for the destructive
// webhook: Strava signs nothing, athlete ids are public, so an event on its own
// must not delete a connection. Strava's own token endpoint is the authority.
func TestForgedDeauthorizationIsIgnored(t *testing.T) {
	var mu sync.Mutex
	rotated := 0
	handler, queries, userID := newStravaHarness(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("token request: %v", err)
		}
		mu.Lock()
		rotated++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "access-2", "refresh_token": "refresh-2",
			"expires_at": time.Now().Add(6 * time.Hour).Unix(),
			"athlete":    map[string]any{"id": 4242},
		})
	})

	rec := postDeauthorization(t, handler)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 so Strava stops retrying", rec.Code)
	}
	if got := connectionCount(t, queries, userID); got != 1 {
		t.Fatalf("connections = %d, want the forged claim to leave the connection alone", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if rotated != 1 {
		t.Fatalf("token refreshes = %d, want the claim to be checked exactly once", rotated)
	}

	// The check rotated the refresh token, and Strava invalidates the previous
	// one, so the fresh pair has to be stored or the connection is bricked.
	conn, err := queries.GetConnection(context.Background(), userID, syncpkg.KindStravaOAuth)
	if err != nil {
		t.Fatal(err)
	}
	if len(conn.RefreshTokenCipher) == 0 {
		t.Fatal("the rotated refresh token was not stored")
	}
}

// TestGenuineDeauthorizationRemovesTheConnection covers the case the endpoint
// exists for: Strava rejects the stored refresh token, which is only true once
// the athlete has really revoked access.
func TestGenuineDeauthorizationRemovesTheConnection(t *testing.T) {
	handler, queries, userID := newStravaHarness(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"Bad Request","errors":[{"resource":"RefreshToken","field":"refresh_token","code":"invalid"}]}`))
	})

	rec := postDeauthorization(t, handler)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := connectionCount(t, queries, userID); got != 0 {
		t.Fatalf("connections = %d, want the revoked connection to be removed", got)
	}
}

// TestUnverifiedDeauthorizationIsNotRepeated pins the cooldown: a replayed event
// must not make hyl call Strava's token endpoint again.
func TestUnverifiedDeauthorizationIsNotRepeated(t *testing.T) {
	var mu sync.Mutex
	rotated := 0
	handler, queries, userID := newStravaHarness(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		rotated++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "access-2", "refresh_token": "refresh-2",
			"expires_at": time.Now().Add(6 * time.Hour).Unix(),
			"athlete":    map[string]any{"id": 4242},
		})
	})

	for range 3 {
		postDeauthorization(t, handler)
	}
	mu.Lock()
	defer mu.Unlock()
	if rotated != 1 {
		t.Fatalf("token refreshes = %d, want a replayed claim to be ignored", rotated)
	}
	if got := connectionCount(t, queries, userID); got != 1 {
		t.Fatalf("connections = %d, want 1", got)
	}
}
