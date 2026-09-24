package auth

import "testing"

// TestSafeNextRejectsBrowserStrippedCharacters is the regression test for the
// redirect bypass: browsers remove tab, CR and LF before parsing a URL, so a
// path that only begins with a slash after that removal becomes protocol
// relative and leaves the site.
func TestSafeNextRejectsBrowserStrippedCharacters(t *testing.T) {
	rejected := []string{
		"/\t/evil.com",
		"/\n/evil.com",
		"/\r/evil.com",
		"/\x00/evil.com",
		"/\x7f/evil.com",
		"//evil.com",
		"\\evil.com",
		"/\\evil.com",
		"https://evil.com",
		"",
		"relative/path",
	}
	for _, next := range rejected {
		if got := safeNext(next); got != "" {
			t.Errorf("safeNext(%q) = %q, want rejection", next, got)
		}
	}

	kept := []string{"/settings", "/activities/12", "/a/b?c=d"}
	for _, next := range kept {
		if got := safeNext(next); got != next {
			t.Errorf("safeNext(%q) = %q, want it kept", next, got)
		}
	}
}
