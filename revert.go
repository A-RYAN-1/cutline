package cutline

import (
	"regexp"
	"strings"
)

var revertBody = regexp.MustCompile(`This reverts commit ([0-9a-f]{7,64})`)

// RevertOf returns the commit id a revert names in its body ("This reverts
// commit <sha>."), or "" when the commit is not a revert.
func RevertOf(subject, body string) string {
	if !strings.HasPrefix(subject, `Revert "`) && !strings.HasPrefix(subject, `Reapply "`) {
		return ""
	}
	m := revertBody.FindStringSubmatch(body)
	if m == nil {
		return ""
	}
	return m[1]
}

// cancelReverts marks the commits of one side that a revert undoes. Commits
// are newest first, so a revert of a revert is seen before the revert it
// undoes: the inner revert is inactive and the original change stays live. A
// revert whose target is outside the range is a change of its own and is left
// alone.
func cancelReverts(commits []*Commit) {
	for _, c := range commits {
		c.revert = RevertOf(c.Subject, c.Body)
	}
	find := func(prefix string) *Commit {
		for _, t := range commits {
			if strings.HasPrefix(t.SHA, prefix) {
				return t
			}
		}
		return nil
	}
	for _, r := range commits {
		if r.revert == "" || r.reverted {
			continue
		}
		t := find(r.revert)
		if t == nil || t == r {
			continue
		}
		t.reverted = true
		t.revertedBy = r.SHA
		r.dropped = true
	}
	for _, c := range commits {
		if c.reverted && c.revert != "" {
			c.dropped = true // a revert that was itself reverted vanishes
		}
	}
}
