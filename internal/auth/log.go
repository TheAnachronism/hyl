package auth

import (
	"strings"

	"go.uber.org/zap"
)

func zapUser(id int64) zap.Field      { return zap.Int64("user_id", id) }
func zapErr(err error) zap.Field      { return zap.Error(err) }
func zapString(k, v string) zap.Field { return zap.String(k, v) }

// safeNext accepts only same-origin relative paths, so an OAuth round trip can
// never be turned into an open redirect.
func safeNext(next string) string {
	if next == "" || !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") {
		return ""
	}
	for _, r := range next {
		// Browsers strip ASCII tab, newline and carriage return before parsing a
		// URL, so "/%09/evil.com" would resolve to the protocol-relative
		// "//evil.com". Reject every control character, and a backslash, which
		// several parsers also fold into a slash.
		if r < 0x20 || r == 0x7f || r == '\\' {
			return ""
		}
	}
	return next
}
