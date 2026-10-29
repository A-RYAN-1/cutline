package cutline

import (
	"strings"
	"testing"
)

// FuzzParsers feeds arbitrary subject and body text to every parser. The
// parsers read commit text written by anyone, so they must never panic and
// must only return what is really in the input.
func FuzzParsers(f *testing.F) {
	for _, s := range []string{
		"Add retries (#123)",
		"Merge pull request #45 from a/b",
		"Fix thing (#7) extra",
		"Revert \"x (#9)\"",
		"ABC-12 fix the thing",
		"body\n\n(cherry picked from commit abcdef1234567)",
		"(#99999999999999999999)",
		"(cherry picked from commit ABCDEF1)",
		"",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if n := PRNumber(s); n < 0 {
			t.Fatalf("PRNumber(%q) = %d", s, n)
		}
		if IsPRMerge(s) && PRNumber(s) == 0 && !strings.Contains(s, "#0") {
			// A merge subject names a number unless it overflows int.
			if !overflows(s) {
				t.Fatalf("merge subject %q has no number", s)
			}
		}
		if p := PickedFrom(s); p != "" {
			if len(p) < 7 || !strings.Contains(s, p) || strings.Trim(p, "0123456789abcdef") != "" {
				t.Fatalf("PickedFrom(%q) = %q", s, p)
			}
		}
		if k := TicketKey(s); k != "" {
			if !strings.Contains(s, k) || !strings.Contains(k, "-") {
				t.Fatalf("TicketKey(%q) = %q", s, k)
			}
		}
	})
}

// overflows reports whether the first run of digits after "#" is too long for
// an int, the one case where a merge subject yields 0.
func overflows(s string) bool {
	i := strings.Index(s, "#")
	if i < 0 {
		return false
	}
	n := 0
	for _, r := range s[i+1:] {
		if r < '0' || r > '9' {
			break
		}
		n++
	}
	return n > 18
}
