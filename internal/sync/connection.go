package sync

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	"go.uber.org/zap"

	"github.com/markbeep/hyl/internal/apperr"
	"github.com/markbeep/hyl/internal/config"
	"github.com/markbeep/hyl/internal/db"
	"github.com/markbeep/hyl/internal/secrets"
)

// Connections disconnects one provider for an account. The settings route calls
// it; it does not implement the cleanup itself.
type Connections struct {
	Pool   *sql.DB
	Q      *db.Queries
	Cfg    config.Config
	Log    *zap.Logger
	Cipher *secrets.Cipher

	// intervalsBase and httpClient point the best-effort revoke at a fake.
	// Empty and nil use the production host and client.
	intervalsBase string
	httpClient    *http.Client
}

// Disconnect removes one provider kind: its connection, its import rules, and,
// only when the kind owns an export target, the pending exports for that
// target. The three deletes commit together. A user-initiated OAuth disconnect
// asks the provider to revoke access first; that call is best-effort.
func (c *Connections) Disconnect(ctx context.Context, userID int64, kind string) error {
	conn, err := c.Q.GetConnection(ctx, userID, kind)
	if errors.Is(err, sql.ErrNoRows) {
		return apperr.NotFound("no such connection")
	}
	if err != nil {
		return err
	}
	c.revoke(ctx, conn)

	tx, err := c.Pool.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	q := c.Q.WithTx(tx)
	if target, ok := exportTargetFor(kind); ok {
		if _, err := q.DeletePendingExportsForTarget(ctx, userID, target); err != nil {
			return err
		}
	}
	if _, err := q.DeleteImportRulesForConnection(ctx, userID, kind); err != nil {
		return err
	}
	affected, err := q.DeleteConnection(ctx, userID, kind)
	if err != nil {
		return err
	}
	if affected == 0 {
		return apperr.NotFound("no such connection")
	}
	return tx.Commit()
}

// exportTargetFor reports the export target a provider kind owns. intervals.icu
// kinds own none: disconnecting them must not touch a Strava queue.
func exportTargetFor(kind string) (string, bool) {
	if kind == KindStravaOAuth {
		return exportTargetStrava, true
	}
	return "", false
}

// revoke tells the provider to drop hyl's access. A failure is logged and
// ignored: the athlete asked to disconnect, so a provider outage must not keep
// the local connection.
func (c *Connections) revoke(ctx context.Context, conn db.Connection) {
	switch conn.Kind {
	case KindIntervalsOAuth:
		secret, _ := c.Cipher.DecryptString(conn.AccessTokenCipher)
		client := NewIntervalsClient(c.Cfg, conn, secret)
		if c.intervalsBase != "" {
			client.BaseURL = c.intervalsBase
		}
		if c.httpClient != nil {
			client.HTTP = c.httpClient
		}
		if err := client.DisconnectApp(ctx); err != nil {
			c.Log.Warn("telling intervals.icu about the disconnect failed", zap.Error(err))
		}
	case KindStravaOAuth:
		token, err := c.Cipher.DecryptString(conn.RefreshTokenCipher)
		if err != nil || token == "" {
			token, _ = c.Cipher.DecryptString(conn.AccessTokenCipher)
		}
		if token == "" {
			return
		}
		client := NewStravaClient(c.Cfg, "")
		if c.httpClient != nil {
			client.HTTP = c.httpClient
		}
		if err := client.RevokeAccess(ctx, token); err != nil {
			c.Log.Warn("telling Strava about the disconnect failed", zap.Error(err))
		}
	}
}
