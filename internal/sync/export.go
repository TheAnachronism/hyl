package sync

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/markbeep/hyl/internal/activity"
	"github.com/markbeep/hyl/internal/config"
	"github.com/markbeep/hyl/internal/db"
	"github.com/markbeep/hyl/internal/secrets"
)

// Export pacing and retry limits.
const (
	exportBatchSize     = 50
	exportMinInterval   = time.Second
	exportPollInterval  = 2 * time.Second
	exportPollTimeout   = 60 * time.Second
	exportMaxAttempts   = 5
	exportTargetStrava  = "strava"
	exportStatusPending = "pending"
	exportStatusSent    = "sent"
	exportStatusError   = "error"
)

// Exporter drains the activity_exports queue. It runs on the same goroutine as
// the import worker, so Strava never sees more than one request at a time.
type Exporter struct {
	Q      *db.Queries
	Cfg    config.Config
	Log    *zap.Logger
	Cipher *secrets.Cipher

	// tokenBaseURL is the OAuth endpoint used for refreshes; tests point it at
	// a fake server.
	tokenBaseURL string
	// newClient is a seam for tests.
	newClient func(accessToken string) *StravaClient
	// sleep is overridable so tests do not wait for the pacing delays.
	sleep func(context.Context, time.Duration)
}

// NewExporter builds the export drainer.
func NewExporter(pool *sql.DB, cfg config.Config, log *zap.Logger, cipher *secrets.Cipher) *Exporter {
	return &Exporter{
		Q: db.New(pool), Cfg: cfg, Log: log, Cipher: cipher,
		tokenBaseURL: stravaBaseURL,
		newClient:    func(token string) *StravaClient { return NewStravaClient(cfg, token) },
		sleep:        sleepContext,
	}
}

// Drain pushes pending exports, at most one batch per call.
func (e *Exporter) Drain(ctx context.Context) {
	if !e.Cfg.StravaEnabled() {
		return
	}
	pending, err := e.Q.ListPendingExports(ctx, exportBatchSize)
	if err != nil {
		e.Log.Error("listing pending exports failed", zap.Error(err))
		return
	}
	for _, row := range pending {
		if ctx.Err() != nil {
			return
		}
		stop, err := e.exportOne(ctx, row)
		if err != nil {
			e.Log.Warn("export failed", zap.Int64("export_id", row.ID), zap.Error(err))
		}
		if stop {
			return
		}
		e.sleep(ctx, exportMinInterval)
	}
}

// exportOne pushes one activity. It reports whether the whole drain should stop
// (a rate limit or a missing credential would make the next attempt fail too).
func (e *Exporter) exportOne(ctx context.Context, row db.ActivityExport) (stop bool, err error) {
	activityRow, err := e.Q.GetActivity(ctx, row.ActivityID)
	if errors.Is(err, sql.ErrNoRows) {
		// The activity is gone; the export row went with it via the cascade, so
		// this can only happen on a race.
		return false, nil
	}
	if err != nil {
		return false, err
	}

	conn, err := e.Q.GetConnection(ctx, row.UserID, KindStravaOAuth)
	if errors.Is(err, sql.ErrNoRows) {
		// Strava was disconnected after the export was queued.
		return false, e.markError(ctx, row, "the Strava connection was removed")
	}
	if err != nil {
		return false, err
	}

	now := time.Now()
	conn, err = ensureStravaToken(ctx, e.tokenBaseURL, e.Cfg, e.Q, e.Cipher, conn, now)
	if err != nil {
		var providerErr *ProviderError
		if errors.As(err, &providerErr) && providerErr.NeedsReauthorization() {
			return true, e.markError(ctx, row, "reauthorize")
		}
		// A refresh that fails for any other reason is retried, and eventually
		// gives up, through the same attempt accounting as an upload failure.
		return false, e.reschedule(ctx, row, err.Error())
	}
	accessToken, err := e.Cipher.DecryptString(conn.AccessTokenCipher)
	if err != nil {
		return true, e.markError(ctx, row, "reauthorize")
	}

	points, err := e.Q.ListActivityPoints(ctx, row.ActivityID)
	if err != nil {
		return false, err
	}
	var buffer bytes.Buffer
	if err := activity.WriteFIT(&buffer, activityRow, toActivityPoints(points)); err != nil {
		return false, e.markError(ctx, row, "hyl could not build a FIT file for this activity")
	}

	client := e.newClient(accessToken)
	upload, err := client.UploadFit(ctx, fmt.Sprintf("hyl-%d.fit", activityRow.ID),
		titleOrFallback(activityRow), conn.ExportMessage, row.ExternalID, buffer.Bytes())
	if err != nil {
		return e.handleProviderError(ctx, row, err)
	}

	deadline := time.Now().Add(exportPollTimeout)
	for {
		status := strings.ToLower(upload.Status)
		if strings.Contains(status, "ready") {
			return false, e.markSent(ctx, row, upload.ActivityID)
		}
		if strings.Contains(status, "error") {
			message := strings.TrimSpace(upload.Error)
			if message == "" {
				message = upload.Status
			}
			return false, e.markError(ctx, row, message)
		}
		if time.Now().After(deadline) {
			// Still processing: keep it pending and try again next pass.
			return false, e.reschedule(ctx, row, "strava is still processing this upload")
		}
		e.sleep(ctx, exportPollInterval)
		upload, err = client.GetUpload(ctx, upload.ID)
		if err != nil {
			return e.handleProviderError(ctx, row, err)
		}
	}
}

// handleProviderError classifies a provider failure.
func (e *Exporter) handleProviderError(ctx context.Context, row db.ActivityExport, err error) (bool, error) {
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) {
		return false, err
	}
	if providerErr.StatusCode == 429 {
		// Strava throttles hard: stop the whole pass and let the next one try.
		return true, e.reschedule(ctx, row, providerErr.Message)
	}
	if providerErr.NeedsReauthorization() {
		return true, e.markError(ctx, row, "reauthorize")
	}
	return false, e.reschedule(ctx, row, providerErr.Message)
}

func (e *Exporter) markSent(ctx context.Context, row db.ActivityExport, remoteID int64) error {
	remote := ""
	if remoteID != 0 {
		remote = fmt.Sprintf("%d", remoteID)
	}
	_, err := e.Q.UpdateExportStatus(ctx, exportStatusSent, nilIfEmpty(remote), row.Attempts,
		nil, time.Now().Unix(), row.ID)
	return err
}

func (e *Exporter) markError(ctx context.Context, row db.ActivityExport, message string) error {
	_, err := e.Q.UpdateExportStatus(ctx, exportStatusError, nil, row.Attempts+1,
		&message, time.Now().Unix(), row.ID)
	return err
}

// reschedule keeps the row pending and counts the attempt, so a permanently
// broken upload stops after exportMaxAttempts.
func (e *Exporter) reschedule(ctx context.Context, row db.ActivityExport, message string) error {
	attempts := row.Attempts + 1
	status := exportStatusPending
	if attempts >= exportMaxAttempts {
		status = exportStatusError
	}
	_, err := e.Q.UpdateExportStatus(ctx, status, nil, attempts, &message, time.Now().Unix(), row.ID)
	return err
}

// ExportQueuer queues an activity for upload to Strava when the uploading user
// has a Strava connection that asked for automatic exports. It is the single
// automatic-export gate: the intervals connection's own flag is inert, because
// the target of the upload is what decides.
type ExportQueuer struct {
	queries *db.Queries
}

// NewExportQueuer builds the automatic-export gate.
func NewExportQueuer(q *db.Queries) *ExportQueuer {
	return &ExportQueuer{queries: q}
}

// Queue loads the user's strava_oauth connection and, when automatic export is
// enabled, inserts a pending export row. Nil is returned when no connection
// exists, when the flag is off, or when the export is already queued — a caller
// only cares about real failures.
func (q *ExportQueuer) Queue(ctx context.Context, a db.Activity) error {
	conn, err := q.queries.GetConnection(ctx, a.UserID, KindStravaOAuth)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if !conn.AutoExport {
		return nil
	}
	_, err = QueueExport(ctx, q.queries, a, exportTargetStrava)
	return err
}

// QueueExport inserts a pending export row. It reports false when one is
// already queued or sent, which the handler turns into a 409.
func QueueExport(ctx context.Context, q *db.Queries, activity db.Activity, target string) (bool, error) {
	now := time.Now().Unix()
	affected, err := q.CreateExport(ctx, activity.ID, activity.UserID, target, activity.DedupeHash, now, now)
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func titleOrFallback(activityRow db.Activity) string {
	if strings.TrimSpace(activityRow.Title) != "" {
		return activityRow.Title
	}
	return activity.DefaultTitle(activityRow.Sport, time.Unix(activityRow.StartedAt, 0).UTC())
}

func sleepContext(ctx context.Context, d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

// toActivityPoints converts stored rows into the export writer's input.
func toActivityPoints(rows []db.ActivityPoint) []activity.Point {
	points := make([]activity.Point, 0, len(rows))
	for _, row := range rows {
		point := activity.Point{
			Seq: row.Seq, T: row.T, ElapsedS: row.ElapsedS,
			Lat: row.Lat, Lon: row.Lon, Ele: row.Ele, Spd: row.Spd, DistM: row.DistM,
		}
		if row.Hr != nil {
			hr := int(*row.Hr)
			point.HR = &hr
		}
		if row.Cad != nil {
			cadence := int(*row.Cad)
			point.Cad = &cadence
		}
		if row.Pwr != nil {
			power := int(*row.Pwr)
			point.Pwr = &power
		}
		points = append(points, point)
	}
	return points
}
