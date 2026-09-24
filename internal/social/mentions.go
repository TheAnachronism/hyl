package social

import (
	"regexp"
	"strings"
)

// maxMentionsPerComment bounds the work one comment can cause.
const maxMentionsPerComment = 20

// mentionPattern matches an @username that is not part of a longer word, so
// "email@example" and "a@b_c" do not count while "@bob," and a leading "@bob"
// do.
var mentionPattern = regexp.MustCompile(`(?:^|[^\w@])@([A-Za-z0-9_]{3,30})`)

// ParseMentions returns the distinct usernames mentioned in a comment body, in
// order of first appearance.
func ParseMentions(body string) []string {
	matches := mentionPattern.FindAllStringSubmatch(body, -1)
	seen := make(map[string]bool, len(matches))
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		name := strings.ToLower(match[1])
		if seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
		if len(out) == maxMentionsPerComment {
			break
		}
	}
	return out
}

// MentionPolicyAllows applies a mentioned user's mention_policy to one mention.
//
// authorID is the comment author, mentioned the person who would be notified:
//
//	everyone  → always allowed
//	followers → allowed only when the mentioned user follows the author
//	nobody    → never allowed
func MentionPolicyAllows(mentionedFollowersAuthor bool, policy string) bool {
	switch policy {
	case "everyone":
		return true
	case "followers":
		return mentionedFollowersAuthor
	default: // nobody
		return false
	}
}
