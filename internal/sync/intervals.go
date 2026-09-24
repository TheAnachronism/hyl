// Package sync imports activities from intervals.icu and exports them to
// Strava. The worker is one goroutine: every pass is sequential, so a slow
// provider can never fan out into many concurrent requests.
package sync

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/markbeep/hyl/internal/apperr"
	"github.com/markbeep/hyl/internal/config"
	"github.com/markbeep/hyl/internal/db"
)

// Connection kinds stored in connections.kind.
const (
	KindIntervalsOAuth  = "intervals_oauth"
	KindIntervalsAPIKey = "intervals_apikey"
	KindStravaOAuth     = "strava_oauth"
)

// intervalsBaseURL is overridable for tests through IntervalsClient.BaseURL.
const intervalsBaseURL = "https://intervals.icu"

// intervalsScopes is what hyl asks for: reading activities to import and
// writing them back to support auto-export.
const intervalsScopes = "ACTIVITY:READ,ACTIVITY:WRITE"

// IntervalsActivity is one row of the activities listing.
type IntervalsActivity struct {
	ID             string  `json:"id"`
	StartDateLocal string  `json:"start_date_local"`
	Type           string  `json:"type"`
	Name           string  `json:"name"`
	Distance       float64 `json:"distance"`
	MovingTime     int64   `json:"moving_time"`
	Source         string  `json:"source"`
	FileType       string  `json:"file_type"`
	Sport          string  `json:"sport"`
}

// IntervalsClient talks to the intervals.icu REST API. It is used with either
// an API key (Basic auth, the primary self-host path) or an OAuth bearer token.
type IntervalsClient struct {
	BaseURL   string
	APIKey    string
	Token     string
	UserAgent string
	HTTP      *http.Client
}

// NewIntervalsClient picks the credential a connection carries.
func NewIntervalsClient(cfg config.Config, conn db.Connection, secret string) *IntervalsClient {
	client := &IntervalsClient{
		BaseURL:   intervalsBaseURL,
		UserAgent: fmt.Sprintf("hyl/%s (+%s)", cfg.Version, strings.TrimSuffix(cfg.BaseURL, "/")),
		HTTP:      &http.Client{Timeout: 60 * time.Second},
	}
	if conn.Kind == KindIntervalsOAuth {
		client.Token = secret
	} else {
		client.APIKey = secret
	}
	return client
}

// ListActivities returns the activities in a date window. intervals.icu has no
// pagination cursor: the window is narrowed instead.
func (c *IntervalsClient) ListActivities(ctx context.Context, athleteID, oldest, newest string, limit int) ([]IntervalsActivity, error) {
	query := url.Values{}
	query.Set("oldest", oldest)
	query.Set("newest", newest)
	if limit > 0 {
		query.Set("limit", strconv.Itoa(limit))
	}
	path := fmt.Sprintf("/api/v1/athlete/%s/activities?%s", url.PathEscape(athleteID), query.Encode())

	var activities []IntervalsActivity
	if err := c.request(ctx, http.MethodGet, path, nil, &activities); err != nil {
		return nil, err
	}
	return activities, nil
}

// AthleteID resolves the athlete a credential belongs to. intervals accepts "0"
// as "the owner of this credential", but webhook events always name the real
// athlete id, so a connection has to learn its own id to match them.
func (c *IntervalsClient) AthleteID(ctx context.Context) (string, error) {
	var athlete struct {
		ID string `json:"id"`
	}
	if err := c.request(ctx, http.MethodGet, "/api/v1/athlete/0", nil, &athlete); err != nil {
		return "", err
	}
	if athlete.ID == "" {
		return "", errors.New("intervals.icu returned no athlete id")
	}
	return athlete.ID, nil
}

// DownloadFit fetches the original file of one activity. intervals refuses to
// serve files for activities that came from Strava, so callers skip those.
func (c *IntervalsClient) DownloadFit(ctx context.Context, activityID string) (io.ReadCloser, error) {
	path := fmt.Sprintf("/api/v1/activity/%s/fit-file?power=true&hr=true", url.PathEscape(activityID))
	req, err := c.newRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	response, err := c.client().Do(req)
	if err != nil {
		return nil, err
	}
	if response.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 512))
		_ = response.Body.Close()
		return nil, &ProviderError{
			StatusCode: response.StatusCode,
			Message:    fmt.Sprintf("intervals.icu returned %d for the file: %s", response.StatusCode, strings.TrimSpace(string(body))),
			RetryAfter: retryAfter(response),
		}
	}
	return response.Body, nil
}

// DisconnectApp tells intervals.icu to drop hyl's access.
func (c *IntervalsClient) DisconnectApp(ctx context.Context) error {
	return c.request(ctx, http.MethodDelete, "/api/v1/disconnect-app", nil, nil)
}

// ExchangeOAuthCode turns an authorization code into an access token.
func ExchangeIntervalsCode(ctx context.Context, cfg config.Config, code string) (token string, athleteID string, err error) {
	form := url.Values{}
	form.Set("client_id", cfg.IntervalsClientID)
	form.Set("client_secret", cfg.IntervalsClientSecret)
	form.Set("code", code)
	form.Set("grant_type", "authorization_code")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		intervalsBaseURL+"/api/oauth/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	response, err := tokenHTTPClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer func() { _ = response.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if response.StatusCode >= 400 {
		return "", "", apperr.BadRequest("intervals.icu rejected the authorization code (%d)", response.StatusCode)
	}

	var payload struct {
		AccessToken string `json:"access_token"`
		Athlete     struct {
			ID string `json:"id"`
		} `json:"athlete"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", "", apperr.BadRequest("intervals.icu returned an unexpected token response")
	}
	if payload.AccessToken == "" {
		return "", "", apperr.BadRequest("intervals.icu returned no access token")
	}
	return payload.AccessToken, payload.Athlete.ID, nil
}

// AuthorizeURL builds the OAuth consent URL.
func IntervalsAuthorizeURL(cfg config.Config, state string) string {
	query := url.Values{}
	query.Set("client_id", cfg.IntervalsClientID)
	query.Set("redirect_uri", cfg.BaseURL+"/api/connections/intervals/callback")
	query.Set("scope", intervalsScopes)
	query.Set("state", state)
	query.Set("response_type", "code")
	return intervalsBaseURL + "/api/oauth/authorize?" + query.Encode()
}

func (c *IntervalsClient) request(ctx context.Context, method, path string, body io.Reader, out any) error {
	req, err := c.newRequest(ctx, method, path, body)
	if err != nil {
		return err
	}
	response, err := c.client().Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()

	payload, err := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if err != nil {
		return err
	}
	if response.StatusCode >= 400 {
		return &ProviderError{
			StatusCode: response.StatusCode,
			Message:    fmt.Sprintf("intervals.icu returned %d: %s", response.StatusCode, summarize(payload)),
			RetryAfter: retryAfter(response),
		}
	}
	if out == nil || len(payload) == 0 {
		return nil
	}
	return json.Unmarshal(payload, out)
}

func (c *IntervalsClient) newRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	base := c.BaseURL
	if base == "" {
		base = intervalsBaseURL
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	// The API sits behind Cloudflare and rejects the default Go user agent.
	req.Header.Set("User-Agent", c.UserAgent)
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	} else if c.APIKey != "" {
		req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("API_KEY:"+c.APIKey)))
	}
	return req, nil
}

func (c *IntervalsClient) client() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	// Provider endpoints are pinned to TLS 1.2+; the default transport already
	// negotiates that, the field exists so tests can inject a fake.
	return &http.Client{Timeout: 60 * time.Second, Transport: &http.Transport{
		TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
	}}
}

// ProviderError carries the HTTP status and any Retry-After hint so the worker
// can back off correctly.
type ProviderError struct {
	StatusCode int
	Message    string
	RetryAfter time.Duration
}

func (e *ProviderError) Error() string { return e.Message }

// NeedsReauthorization reports whether a stored OAuth token has gone stale.
func (e *ProviderError) NeedsReauthorization() bool {
	return e.StatusCode == http.StatusUnauthorized || e.StatusCode == http.StatusForbidden
}

// rateLimitWindow is the providers' rolling throttle window.
const rateLimitWindow = 15 * time.Minute

// retryAfter reads whatever backoff hint the provider sent. Both providers
// express their 15-minute budget as a comma-separated pair, but intervals
// reports what is left while Strava reports what was used, and Strava never
// sends Retry-After at all, so all three forms are handled.
func retryAfter(response *http.Response) time.Duration {
	raw := response.Header.Get("Retry-After")
	if raw != "" {
		if seconds, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil {
			return time.Duration(seconds) * time.Second
		}
		if when, err := http.ParseTime(raw); err == nil {
			if delta := time.Until(when); delta > 0 {
				return delta
			}
		}
	}
	if budgetExhausted(response.Header) {
		return rateLimitWindow
	}
	return 0
}

// budgetExhausted reports whether the 15-minute quota is spent: Strava sends
// X-RateLimit-Limit and X-RateLimit-Usage, intervals sends X-RateLimit-Limit
// and X-RateLimit-Remaining, both as "<15m>,<daily>".
func budgetExhausted(header http.Header) bool {
	limit, ok := windowQuota(header.Get("X-RateLimit-Limit"))
	if !ok {
		return false
	}
	if used, ok := windowQuota(header.Get("X-RateLimit-Usage")); ok {
		return used >= limit
	}
	if left, ok := windowQuota(header.Get("X-RateLimit-Remaining")); ok {
		return left == 0
	}
	return false
}

// windowQuota parses the 15-minute member of a "<15m>,<daily>" header.
func windowQuota(raw string) (int, bool) {
	if raw == "" {
		return 0, false
	}
	part, _, _ := strings.Cut(raw, ",")
	value, err := strconv.Atoi(strings.TrimSpace(part))
	if err != nil {
		return 0, false
	}
	return value, true
}

func summarize(payload []byte) string {
	text := strings.TrimSpace(string(payload))
	if len(text) > 200 {
		text = text[:200] + "…"
	}
	if text == "" {
		text = "(no body)"
	}
	return text
}
