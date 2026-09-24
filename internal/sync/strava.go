package sync

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/markbeep/hyl/internal/apperr"
	"github.com/markbeep/hyl/internal/config"
	"github.com/markbeep/hyl/internal/db"
	"github.com/markbeep/hyl/internal/secrets"
)

// stravaBaseURL is overridable so the export path can be tested without an
// account: Strava requires a subscription and a single-athlete app.
const stravaBaseURL = "https://www.strava.com"

// stravaRefreshWindow is how long before expiry a token is refreshed.
const stravaRefreshWindow = 3600

// stravaScope is the only scope hyl needs: writing activities back.
const stravaScope = "activity:write"

// StravaClient talks to the Strava v3 API with a stored OAuth token.
type StravaClient struct {
	BaseURL      string
	ClientID     string
	ClientSecret string
	AccessToken  string
	HTTP         *http.Client
}

// NewStravaClient builds a client for one stored connection.
func NewStravaClient(cfg config.Config, accessToken string) *StravaClient {
	return &StravaClient{
		BaseURL:      stravaBaseURL,
		ClientID:     cfg.StravaClientID,
		ClientSecret: cfg.StravaClientSecret,
		AccessToken:  accessToken,
		HTTP:         &http.Client{Timeout: 120 * time.Second},
	}
}

// StravaTokens is the token payload both the code exchange and the refresh
// return.
type StravaTokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    int64  `json:"expires_at"`
	Athlete      struct {
		ID int64 `json:"id"`
	} `json:"athlete"`
}

// ExchangeStravaCode trades an authorization code for tokens.
func ExchangeStravaCode(ctx context.Context, cfg config.Config, code string) (StravaTokens, error) {
	return exchangeStravaCode(ctx, stravaBaseURL, cfg, code)
}

// RefreshStravaToken rotates an expired access token. Strava rotates the refresh
// token too, so the caller must persist whatever comes back immediately.
func RefreshStravaToken(ctx context.Context, cfg config.Config, refreshToken string) (StravaTokens, error) {
	return refreshStravaToken(ctx, stravaBaseURL, cfg, refreshToken)
}

// exchangeStravaCode and refreshStravaToken take the base URL explicitly so the
// export path can be driven against a test server.
func exchangeStravaCode(ctx context.Context, baseURL string, cfg config.Config, code string) (StravaTokens, error) {
	form := url.Values{}
	form.Set("client_id", cfg.StravaClientID)
	form.Set("client_secret", cfg.StravaClientSecret)
	form.Set("code", code)
	form.Set("grant_type", "authorization_code")
	return stravaTokenRequest(ctx, baseURL, form)
}

func refreshStravaToken(ctx context.Context, baseURL string, cfg config.Config, refreshToken string) (StravaTokens, error) {
	form := url.Values{}
	form.Set("client_id", cfg.StravaClientID)
	form.Set("client_secret", cfg.StravaClientSecret)
	form.Set("refresh_token", refreshToken)
	form.Set("grant_type", "refresh_token")
	return stravaTokenRequest(ctx, baseURL, form)
}

// StravaAuthorizeURL builds the consent URL.
func StravaAuthorizeURL(cfg config.Config, state string) string {
	query := url.Values{}
	query.Set("client_id", cfg.StravaClientID)
	query.Set("redirect_uri", cfg.BaseURL+"/api/connections/strava/callback")
	query.Set("response_type", "code")
	query.Set("approval_prompt", "auto")
	query.Set("scope", stravaScope)
	query.Set("state", state)
	return stravaBaseURL + "/oauth/authorize?" + query.Encode()
}

func stravaTokenRequest(ctx context.Context, baseURL string, form url.Values) (StravaTokens, error) {
	var tokens StravaTokens
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		baseURL+"/oauth/token", strings.NewReader(form.Encode()))
	if err != nil {
		return tokens, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return tokens, err
	}
	defer func() { _ = response.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if response.StatusCode >= 400 {
		return tokens, &ProviderError{
			StatusCode: response.StatusCode,
			Message:    fmt.Sprintf("strava rejected the token request (%d): %s", response.StatusCode, summarize(body)),
			RetryAfter: retryAfter(response),
		}
	}
	if err := json.Unmarshal(body, &tokens); err != nil {
		return tokens, apperr.BadRequest("strava returned an unexpected token response")
	}
	if tokens.AccessToken == "" {
		return tokens, apperr.BadRequest("strava returned no access token")
	}
	return tokens, nil
}

// StravaUpload is one item of the upload queue.
type StravaUpload struct {
	ID         int64  `json:"id"`
	Status     string `json:"status"`
	ActivityID int64  `json:"activity_id"`
	Error      string `json:"error"`
}

// UploadFit posts a synthesized FIT file. Strava answers with an upload id that
// has to be polled until the file is processed.
func (c *StravaClient) UploadFit(ctx context.Context, filename, name, description, externalID string, payload []byte) (StravaUpload, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return StravaUpload{}, err
	}
	if _, err := part.Write(payload); err != nil {
		return StravaUpload{}, err
	}
	fields := map[string]string{
		"data_type":   "fit",
		"name":        name,
		"description": description,
		"external_id": externalID,
		"trainer":     "0",
	}
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			return StravaUpload{}, err
		}
	}
	if err := writer.Close(); err != nil {
		return StravaUpload{}, err
	}

	req, err := c.newRequest(ctx, http.MethodPost, "/api/v3/uploads", &body)
	if err != nil {
		return StravaUpload{}, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	var upload StravaUpload
	if err := c.do(req, &upload); err != nil {
		return StravaUpload{}, err
	}
	return upload, nil
}

// GetUpload polls the processing state of an upload.
func (c *StravaClient) GetUpload(ctx context.Context, uploadID int64) (StravaUpload, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/api/v3/uploads/"+strconv.FormatInt(uploadID, 10), nil)
	if err != nil {
		return StravaUpload{}, err
	}
	var upload StravaUpload
	if err := c.do(req, &upload); err != nil {
		return StravaUpload{}, err
	}
	return upload, nil
}

// Deauthorize tells Strava the app no longer needs access.
func (c *StravaClient) Deauthorize(ctx context.Context) error {
	req, err := c.newRequest(ctx, http.MethodPost, "/oauth/deauthorize", nil)
	if err != nil {
		return err
	}
	return c.do(req, nil)
}

func (c *StravaClient) newRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	base := c.BaseURL
	if base == "" {
		base = stravaBaseURL
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.AccessToken)
	return req, nil
}

func (c *StravaClient) do(req *http.Request, out any) error {
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 120 * time.Second}
	}
	response, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	payload, _ := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if response.StatusCode >= 400 {
		return &ProviderError{
			StatusCode: response.StatusCode,
			Message:    fmt.Sprintf("strava returned %d: %s", response.StatusCode, summarize(payload)),
			RetryAfter: retryAfter(response),
		}
	}
	if out == nil || len(payload) == 0 {
		return nil
	}
	return json.Unmarshal(payload, out)
}

// ensureStravaToken refreshes the access token when it is close to expiry and
// persists the rotated pair.
func ensureStravaToken(ctx context.Context, baseURL string, cfg config.Config, q *db.Queries, cipher *secrets.Cipher, conn db.Connection, now time.Time) (db.Connection, error) {
	if conn.TokenExpiresAt != nil && *conn.TokenExpiresAt-now.Unix() > stravaRefreshWindow {
		return conn, nil
	}
	refreshToken, err := cipher.DecryptString(conn.RefreshTokenCipher)
	if err != nil || refreshToken == "" {
		return conn, apperr.BadRequest("this Strava connection has no refresh token; reconnect it")
	}
	tokens, err := refreshStravaToken(ctx, baseURL, cfg, refreshToken)
	if err != nil {
		return conn, err
	}
	access, err := cipher.EncryptString(tokens.AccessToken)
	if err != nil {
		return conn, err
	}
	rotated, err := cipher.EncryptString(tokens.RefreshToken)
	if err != nil {
		return conn, err
	}
	if _, err := q.UpdateConnectionTokens(ctx, access, rotated, &tokens.ExpiresAt, now.Unix(), conn.ID); err != nil {
		return conn, err
	}
	conn.AccessTokenCipher = access
	conn.RefreshTokenCipher = rotated
	conn.TokenExpiresAt = &tokens.ExpiresAt
	return conn, nil
}
