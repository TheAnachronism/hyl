package sync

import (
	"strings"
	"testing"

	"github.com/markbeep/hyl/internal/config"
)

// TestIntervalsAuthorizeURLAsksForReadOnly pins the permission hyl asks an
// athlete for. hyl only ever reads from intervals.icu — imports are GETs and
// activity export goes to Strava — so a write scope would be a permission that
// is granted and never used. The assertion is on the URL the athlete is
// actually shown, so re-adding a write scope fails here.
func TestIntervalsAuthorizeURLAsksForReadOnly(t *testing.T) {
	cfg := config.Config{BaseURL: "https://hyl.example.org", IntervalsClientID: "1234"}

	url := IntervalsAuthorizeURL(cfg, "state-token")
	if !strings.Contains(url, "scope=ACTIVITY%3AREAD") {
		t.Fatalf("authorize URL does not request ACTIVITY:READ: %q", url)
	}
	if strings.Contains(url, "WRITE") {
		t.Fatalf("authorize URL requests a write scope: %q", url)
	}
}
