package social

import (
	"reflect"
	"testing"
)

func TestParseMentions(t *testing.T) {
	cases := []struct {
		name string
		body string
		want []string
	}{
		{"single", "@bob nice ride", []string{"bob"}},
		{"multiple", "@bob and @carol well done", []string{"bob", "carol"}},
		{"duplicates collapse", "@bob @bob @bob", []string{"bob"}},
		{"case folds", "@Bob @BOB", []string{"bob"}},
		{"punctuation delimited", "great pace @bob, keep it up", []string{"bob"}},
		{"at sign in an address", "mail me at email@example.com", nil},
		{"word before the at sign", "a@b_c chains are not mentions", nil},
		{"start of the body", "@bob", []string{"bob"}},
		{"after a newline", "line one\n@bob line two", []string{"bob"}},
		{"underscores and digits", "@user_2 nice", []string{"user_2"}},
		{"too short", "@ab nope", nil},
		{"no at sign", "nothing here", nil},
		{"trailing colon", "@bob:", []string{"bob"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseMentions(tc.body)
			if len(got) == 0 && len(tc.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("ParseMentions(%q) = %v, want %v", tc.body, got, tc.want)
			}
		})
	}
}

func TestMentionPolicyAllows(t *testing.T) {
	cases := []struct {
		policy        string
		followsAuthor bool
		want          bool
	}{
		{"everyone", true, true},
		{"everyone", false, true},
		{"followers", true, true},
		{"followers", false, false},
		{"nobody", true, false},
		{"nobody", false, false},
		{"", true, false},
	}
	for _, tc := range cases {
		if got := MentionPolicyAllows(tc.followsAuthor, tc.policy); got != tc.want {
			t.Errorf("policy=%q followsAuthor=%v: got %v want %v", tc.policy, tc.followsAuthor, got, tc.want)
		}
	}
}
