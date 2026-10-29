package cutline

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	prSquash    = regexp.MustCompile(`\(#(\d+)\)\s*$`)
	prMerge     = regexp.MustCompile(`^Merge pull request #(\d+)\b`)
	pickTrailer = regexp.MustCompile(`\(cherry picked from commit ([0-9a-f]{7,64})\)`)
	ticketKey   = regexp.MustCompile(`\b([A-Z][A-Z0-9]{1,9}-\d+)\b`)
)

// PRNumber returns the pull request number a subject line carries, from a
// squash subject "Title (#12)" or a merge subject "Merge pull request #12 from
// ...". It returns 0 when there is none.
func PRNumber(subject string) int {
	if m := prMerge.FindStringSubmatch(subject); m != nil {
		return atoi(m[1])
	}
	if m := prSquash.FindStringSubmatch(subject); m != nil {
		return atoi(m[1])
	}
	return 0
}

// IsPRMerge reports whether the subject is a merge-commit pull request subject.
func IsPRMerge(subject string) bool { return prMerge.MatchString(subject) }

// PickedFrom returns the commit id written by "git cherry-pick -x", or "".
func PickedFrom(body string) string {
	m := pickTrailer.FindStringSubmatch(body)
	if m == nil {
		return ""
	}
	return m[1]
}

// TicketKey returns the first ticket key such as ABC-123 in the subject, or "".
func TicketKey(subject string) string {
	return ticketKey.FindString(subject)
}

func atoi(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return n
}
