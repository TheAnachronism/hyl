// Package api holds every type the HTTP API returns. Nothing else is exposed to
// tygo, and every field carries an explicit camelCase JSON tag.
package api

import "time"

// ErrorResponse is the single error envelope used by every endpoint.
type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

// ErrorBody carries a machine code, a human message and an optional
// machine-readable detail.
type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	// ActivityID is set on duplicate_activity responses so the upload UI can
	// link to the activity that already exists.
	ActivityID *int64 `json:"activityId,omitempty"`
}

// Me is the authenticated user's own profile.
type Me struct {
	ID                   int64  `json:"id"`
	Username             string `json:"username"`
	Email                string `json:"email"`
	DisplayName          string `json:"displayName"`
	Bio                  string `json:"bio"`
	AvatarURL            string `json:"avatarUrl"`
	EmailVerified        bool   `json:"emailVerified"`
	IsAdmin              bool   `json:"isAdmin"`
	ProfileVisibility    string `json:"profileVisibility"`
	ActivitiesVisibility string `json:"activitiesVisibility"`
	FollowPolicy         string `json:"followPolicy"`
	MentionPolicy        string `json:"mentionPolicy"`
	TrimScope            string `json:"trimScope"`
	TrimRadiusM          int64  `json:"trimRadiusM"`
	CreatedAt            string `json:"createdAt"`
}

// UserProfile is another user's public profile. FollowState is one of
// none | following | pending_outgoing | pending_incoming | self.
type UserProfile struct {
	ID             int64  `json:"id"`
	Username       string `json:"username"`
	DisplayName    string `json:"displayName"`
	Bio            string `json:"bio"`
	AvatarURL      string `json:"avatarUrl"`
	FollowerCount  int64  `json:"followerCount"`
	FollowingCount int64  `json:"followingCount"`
	ActivityCount  int64  `json:"activityCount"`
	FollowState    string `json:"followState"`
	IsMe           bool   `json:"isMe"`
	IsPrivate      bool   `json:"isPrivate"`
}

// UserRef is a compact user reference used in lists and notifications.
type UserRef struct {
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	AvatarURL   string `json:"avatarUrl"`
}

// Photo is one stored image variant pair.
type Photo struct {
	ID       int64  `json:"id"`
	URL      string `json:"url"`
	ThumbURL string `json:"thumbUrl"`
	Width    int32  `json:"width"`
	Height   int32  `json:"height"`
}

// Comment is one activity comment.
type Comment struct {
	ID          int64    `json:"id"`
	Username    string   `json:"username"`
	DisplayName string   `json:"displayName"`
	AvatarURL   string   `json:"avatarUrl"`
	Body        string   `json:"body"`
	Mentions    []string `json:"mentions"`
	CreatedAt   string   `json:"createdAt"`
	CanDelete   bool     `json:"canDelete"`
}

// CommentCreated is the POST /comments response.
type CommentCreated struct {
	Comment         Comment  `json:"comment"`
	DroppedMentions []string `json:"droppedMentions"`
}

// Notification is one in-app notification.
type Notification struct {
	ID         int64   `json:"id"`
	Kind       string  `json:"kind"`
	Actor      UserRef `json:"actor"`
	ActivityID *int64  `json:"activityId"`
	CommentID  *int64  `json:"commentId"`
	CreatedAt  string  `json:"createdAt"`
	ReadAt     *string `json:"readAt"`
}

// NotificationCount is the unread badge payload.
type NotificationCount struct {
	Unread int64 `json:"unread"`
}

// ProviderInfo reports one OAuth provider's availability.
type ProviderInfo struct {
	ID      string `json:"id"`
	Enabled bool   `json:"enabled"`
}

// LinkedIdentity is one OAuth provider linked to the current account.
type LinkedIdentity struct {
	Provider  string  `json:"provider"`
	Email     *string `json:"email"`
	CreatedAt string  `json:"createdAt"`
}

// ConfigResponse is the public boot configuration.
type ConfigResponse struct {
	RegistrationOpen bool           `json:"registrationOpen"`
	Providers        []ProviderInfo `json:"providers"`
	PMTilesURL       *string        `json:"pmtilesUrl"`
	TilesAttribution string         `json:"tilesAttribution"`
	Version          string         `json:"version"`
	// IntervalsOAuth and Strava tell the settings page which connect buttons
	// this instance can actually offer.
	IntervalsOAuth bool `json:"intervalsOAuth"`
	Strava         bool `json:"strava"`
	Webhooks       bool `json:"webhooks"`
}

// SportTotals aggregates one sport over a date range.
type SportTotals struct {
	Sport          string  `json:"sport"`
	Count          int64   `json:"count"`
	DistanceM      float64 `json:"distanceM"`
	ElevationGainM float64 `json:"elevationGainM"`
	MovingTimeS    int64   `json:"movingTimeS"`
	ElapsedTimeS   int64   `json:"elapsedTimeS"`
}

// StatsResponse is the per-sport statistics payload.
type StatsResponse struct {
	Sports []SportTotals `json:"sports"`
	All    SportTotals   `json:"all"`
}

// ActivityPage is one page-numbered activity list. Page is 1-based, clamped to
// [1, TotalPages] (or 1 when there are no rows); PageSize is the effective
// ?limit=; TotalPages is 0 when there are no rows.
type ActivityPage struct {
	Items      []ActivitySummary `json:"items"`
	Page       int64             `json:"page"`
	PageSize   int64             `json:"pageSize"`
	Total      int64             `json:"total"`
	TotalPages int64             `json:"totalPages"`
}

// CommentPage is one keyset-paginated comment list.
type CommentPage struct {
	Items      []Comment `json:"items"`
	NextBefore *int64    `json:"nextBefore"`
}

// NotificationPage is one keyset-paginated notification list.
type NotificationPage struct {
	Items      []Notification `json:"items"`
	NextBefore *int64         `json:"nextBefore"`
}

// UserPage is one keyset-paginated user list.
type UserPage struct {
	Items      []UserRef `json:"items"`
	NextBefore *int64    `json:"nextBefore"`
}

// FollowResult is the follow/unfollow endpoint payload.
type FollowResult struct {
	FollowState   string `json:"followState"`
	FollowerCount int64  `json:"followerCount"`
}

// LikeResult is the like endpoint payload.
type LikeResult struct {
	LikeCount int64 `json:"likeCount"`
	LikedByMe bool  `json:"likedByMe"`
}

// ActivitySummary is one row of any activity list.
type ActivitySummary struct {
	ID          int64  `json:"id"`
	UserID      int64  `json:"userId"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	// AvatarURL is the owner's avatar thumbnail; empty when they have none.
	AvatarURL      string   `json:"avatarUrl"`
	Title          string   `json:"title"`
	Sport          string   `json:"sport"`
	StartedAt      string   `json:"startedAt"`
	DistanceM      float64  `json:"distanceM"`
	MovingTimeS    int64    `json:"movingTimeS"`
	ElapsedTimeS   int64    `json:"elapsedTimeS"`
	ElevationGainM float64  `json:"elevationGainM"`
	AvgSpeedMps    *float64 `json:"avgSpeedMps"`
	MaxSpeedMps    *float64 `json:"maxSpeedMps"`
	AvgHeartRate   *int64   `json:"avgHeartRate"`
	MaxHeartRate   *int64   `json:"maxHeartRate"`
	AvgCadence     *float64 `json:"avgCadence"`
	AvgPowerW      *float64 `json:"avgPowerW"`
	LikeCount      int64    `json:"likeCount"`
	CommentCount   int64    `json:"commentCount"`
	LikedByMe      bool     `json:"likedByMe"`
	PhotoCount     int64    `json:"photoCount"`
	Visibility     string   `json:"visibility"`
	// HasGps and RouteHidden are the owner's inputs: the client needs them to
	// render the "show route" switch, which MapAvailable alone cannot express
	// (an indoor activity has no route to hide but is not hidden either).
	HasGps      bool `json:"hasGps"`
	RouteHidden bool `json:"routeHidden"`
	// Track is a flattened [lat,lon,lat,lon,…] list with 5 decimal places,
	// already decimated to <=200 points and privacy-trimmed. Empty when the
	// activity has no drawable route.
	Track        []float64 `json:"track"`
	MapAvailable bool      `json:"mapAvailable"`
	// Photos is the first four photos by position, so feed cards can show a strip.
	Photos []Photo `json:"photos"`
	// Description is populated in list responses too, so the feed can render it.
	Description string `json:"description"`
}

// ActivityDetail is the single-activity payload.
type ActivityDetail struct {
	ActivitySummary `tstype:",extends"`
	CanEdit         bool `json:"canEdit"`
	// Route is the untrimmed-but-decimated coordinate list for the map page,
	// subject to the same privacy rules as Track.
	Route   []float64       `json:"route"`
	Streams ActivityStreams `json:"streams"`
}

// ActivityStreams are the charted series, decimated to <=600 values each.
type ActivityStreams struct {
	ElapsedS   []int64    `json:"elapsedS"`
	HR         []*int64   `json:"hr"`
	Cadence    []*int64   `json:"cadence"`
	Power      []*int64   `json:"power"`
	SpeedMps   []*float64 `json:"speedMps"`
	ElevationM []*float64 `json:"elevationM"`
}

// Connection is one configured provider connection.
type Connection struct {
	Kind          string  `json:"kind"`
	AthleteID     *string `json:"athleteId"`
	AutoExport    bool    `json:"autoExport"`
	ExportMessage string  `json:"exportMessage"`
	LastError     *string `json:"lastError"`
	LastSuccessAt *string `json:"lastSuccessAt"`
	NextAttemptAt *string `json:"nextAttemptAt"`
	Reauthorize   bool    `json:"reauthorize"`
}

// ImportRule is one per-sport import switch.
type ImportRule struct {
	ConnectionKind string `json:"connectionKind"`
	Sport          string `json:"sport"`
	Enabled        bool   `json:"enabled"`
}

// SyncRun summarises one sync pass.
type SyncRun struct {
	ConnectionKind string  `json:"connectionKind"`
	StartedAt      string  `json:"startedAt"`
	FinishedAt     *string `json:"finishedAt"`
	Imported       int64   `json:"imported"`
	Skipped        int64   `json:"skipped"`
	Error          *string `json:"error"`
}

// ConnectionsResponse is the connections settings payload.
type ConnectionsResponse struct {
	Connections []Connection `json:"connections"`
	ImportRules []ImportRule `json:"importRules"`
	LastRuns    []SyncRun    `json:"lastRuns"`
}

// ExportState reports the export queue state of one activity.
type ExportState struct {
	Target    string  `json:"target"`
	Status    string  `json:"status"`
	LastError *string `json:"lastError"`
	UpdatedAt string  `json:"updatedAt"`
}

// PrivacyZone is one circular privacy zone.
type PrivacyZone struct {
	ID      int64   `json:"id"`
	Label   string  `json:"label"`
	Lat     float64 `json:"lat"`
	Lon     float64 `json:"lon"`
	RadiusM int64   `json:"radiusM"`
}

// APIKey is one developer key, never carrying the secret.
type APIKey struct {
	ID         int64   `json:"id"`
	Name       string  `json:"name"`
	Prefix     string  `json:"prefix"`
	CreatedAt  string  `json:"createdAt"`
	LastUsedAt *string `json:"lastUsedAt"`
}

// DeveloperMe is the identity a developer key resolves to.
type DeveloperMe struct {
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
	CreatedAt   string `json:"createdAt"`
}

// DeveloperPoint is one stored sample of the raw stream.
type DeveloperPoint struct {
	Seq        int64    `json:"seq"`
	T          string   `json:"t"`
	ElapsedS   int64    `json:"elapsedS"`
	Lat        *float64 `json:"lat"`
	Lon        *float64 `json:"lon"`
	ElevationM *float64 `json:"elevationM"`
	HeartRate  *int64   `json:"heartRate"`
	Cadence    *int64   `json:"cadence"`
	PowerW     *int64   `json:"powerW"`
	SpeedMps   *float64 `json:"speedMps"`
	DistanceM  *float64 `json:"distanceM"`
}

// DeveloperActivity is the activity payload of the developer API: the summary
// plus the full stored point stream and its provenance.
type DeveloperActivity struct {
	ActivitySummary `tstype:",extends"`
	Source          string           `json:"source"`
	SourceRef       *string          `json:"sourceRef"`
	Points          []DeveloperPoint `json:"points"`
}

// APIKeyCreated is the one-time key reveal payload.
type APIKeyCreated struct {
	APIKey `tstype:",extends"`
	Key    string `json:"key"`
}

// Timestamp renders unix seconds as an RFC3339 UTC string.
func Timestamp(unix int64) string {
	return time.Unix(unix, 0).UTC().Format(time.RFC3339)
}

// TimestampPtr renders an optional unix timestamp.
func TimestampPtr(unix *int64) *string {
	if unix == nil {
		return nil
	}
	s := Timestamp(*unix)
	return &s
}
