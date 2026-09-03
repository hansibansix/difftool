package notes

import "testing"

func TestNoteMatches(t *testing.T) {
	for _, c := range []struct {
		f, key string
		want   bool
	}{
		{"a/b.txt", "a/b.txt", true},
		{"b.txt", "proj/a/b.txt", true},   // agent path shorter than ours
		{"repo/a/b.txt", "a/b.txt", true}, // agent path longer than ours
		{"ab.txt", "a/b.txt", false},
		{"", "a/b.txt", false},
	} {
		if got := Matches(c.f, c.key); got != c.want {
			t.Errorf("Matches(%q, %q) = %v", c.f, c.key, got)
		}
	}
}
