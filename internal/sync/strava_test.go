package sync

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/markbeep/hyl/internal/activity"
)

func TestStravaSportTypePrefersTheProvidersOwnType(t *testing.T) {
	tests := []struct {
		name        string
		sourceSport string
		storedSport string
		want        string
	}{
		{"provider type wins over the coarse key", "GravelRide", activity.SportRide, "GravelRide"},
		{"provider enum is matched case-insensitively", "virtualride", activity.SportRide, "VirtualRide"},
		{"a type Strava added in 2026 round-trips", "Padel", activity.SportOther, "Padel"},
		{"unknown provider values fall back to the stored key", "Kitesurfing!", activity.SportRun, "Run"},
		{"a stored sport Strava cannot infer is still named", "", activity.SportSwim, "Swim"},
		{"ambiguous stored sports are left to the file", "", activity.SportSki, ""},
		{"an empty activity sends nothing", "", activity.SportOther, ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := StravaSportType(test.sourceSport, test.storedSport); got != test.want {
				t.Fatalf("StravaSportType(%q, %q) = %q, want %q", test.sourceSport, test.storedSport, got, test.want)
			}
		})
	}
}

// TestStravaSportTypeAcceptsEverythingItEmits guards the whitelist against a
// typo: a value hyl sends that Strava does not know makes every upload fail.
func TestStravaSportTypeAcceptsEverythingItEmits(t *testing.T) {
	for key, canonical := range stravaSportTypes {
		if got := StravaSportType(key, ""); got != canonical {
			t.Errorf("StravaSportType(%q) = %q, want %q", key, got, canonical)
		}
		if !strings.EqualFold(key, canonical) {
			t.Errorf("lookup key %q is not the lowercase form of %q", key, canonical)
		}
	}
}

func TestRetryAfterReadsProviderRateLimits(t *testing.T) {
	tests := []struct {
		name    string
		headers map[string]string
		want    time.Duration
	}{
		{
			name:    "retry-after seconds",
			headers: map[string]string{"Retry-After": "370"},
			want:    370 * time.Second,
		},
		{
			name: "strava usage at the limit",
			headers: map[string]string{
				"X-RateLimit-Limit": "100,1000",
				"X-RateLimit-Usage": "100,1000",
			},
			want: rateLimitWindow,
		},
		{
			name: "intervals remaining exhausted",
			headers: map[string]string{
				"X-RateLimit-Limit":     "2500,5000",
				"X-RateLimit-Remaining": "0,4900",
			},
			want: rateLimitWindow,
		},
		{
			name: "intervals remaining healthy",
			headers: map[string]string{
				"X-RateLimit-Limit":     "2500,5000",
				"X-RateLimit-Remaining": "12,4900",
			},
			want: 0,
		},
		{
			name: "a daily-only problem is not a window problem",
			headers: map[string]string{
				"X-RateLimit-Limit": "100,1000",
				"X-RateLimit-Usage": "10,1000",
			},
			want: 0,
		},
		{
			name:    "no hints at all",
			headers: map[string]string{},
			want:    0,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := &http.Response{Header: http.Header{}}
			for key, value := range test.headers {
				response.Header.Set(key, value)
			}
			if got := retryAfter(response); got != test.want {
				t.Fatalf("retryAfter = %v, want %v", got, test.want)
			}
		})
	}
}

// TestStravaTokensGrantedScope pins the check that keeps a connection whose
// athlete unticked activity:write from looking healthy.
func TestStravaTokensGrantedScope(t *testing.T) {
	tests := []struct {
		scope string
		want  bool
	}{
		{"activity:write", true},
		{"activity:read activity:write", true},
		{"activity:read,activity:write", true},
		{"activity:write,read", true},
		{"activity:read", false},
		{"", true}, // a response without the field gets the benefit of the doubt
	}
	for _, test := range tests {
		tokens := StravaTokens{Scope: test.scope}
		if got := tokens.GrantsActivityWrite(); got != test.want {
			t.Errorf("GrantsActivityWrite(%q) = %v, want %v", test.scope, got, test.want)
		}
	}
}
