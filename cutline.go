package cutline

import (
	"context"
	"strconv"
	"strings"
)

// Reason says which fingerprint matched a change on one side to the other.
type Reason string

// The match reasons, strongest first.
const (
	ReasonPick   Reason = "cherry-pick trailer"
	ReasonPatch  Reason = "patch id"
	ReasonPR     Reason = "pull request number"
	ReasonTicket Reason = "ticket key"
)

// Status of an entry on the other branch.
const (
	StatusMissing = "missing" // no commit of the entry is on the other branch
	StatusPartial = "partial" // some commits are, some are not
	StatusPresent = "present" // every commit is
	// StatusReverted marks a change that a revert on the same branch undid.
	StatusReverted = "reverted"
)

// Options for Diff.
type Options struct {
	// Dir is the repository directory. Empty means the current directory.
	Dir string
	// Since limits both sides to commits newer than this date or git date
	// expression such as "2026-10-01" or "2 weeks ago".
	Since string
	// MatchTicket also matches by the ticket key (ABC-123) in the subject. It
	// is off by default because one ticket can cover several pull requests.
	MatchTicket bool
}

// Commit is one commit with the data cutline fingerprints.
type Commit struct {
	SHA     string   `json:"sha"`
	Parents []string `json:"-"`
	Author  string   `json:"author"`
	Date    string   `json:"date"`
	Subject string   `json:"subject"`
	Body    string   `json:"-"`
	PatchID string   `json:"-"`

	pr         int // pull request number, own or inherited from the merge that brought the commit in
	match      *Match
	revert     string // commit id this commit reverts, if it is a revert
	reverted   bool   // a revert on the same side undoes this commit
	revertedBy string
	dropped    bool // a revert that cancels something, or was cancelled itself: not a change
}

// Match links a commit to its counterpart on the other branch.
type Match struct {
	Other  string `json:"other"`
	Reason Reason `json:"reason"`
}

// CommitMatch is a commit of an entry and how it matched, if it did.
type CommitMatch struct {
	SHA     string `json:"sha"`
	Subject string `json:"subject"`
	Match   *Match `json:"match,omitempty"`
}

// Entry is one change as a reader thinks of it: a pull request (its commits
// together) or a single commit.
type Entry struct {
	PR      int           `json:"pr,omitempty"`
	Title   string        `json:"title"`
	Author  string        `json:"author"`
	Date    string        `json:"date"`
	Status  string        `json:"status"`
	Commits []CommitMatch `json:"commits"`
	// RevertedBy is the revert commit when Status is "reverted".
	RevertedBy string `json:"reverted_by,omitempty"`
	// RevertedCommits are commits of the entry that a revert on the same
	// branch undid; they are left out of Commits.
	RevertedCommits []string `json:"reverted_commits,omitempty"`
}

// Result of comparing branch A with branch B.
type Result struct {
	A string `json:"a"`
	B string `json:"b"`
	// OnlyA are entries of A that are missing from B or only partly in B.
	OnlyA []Entry `json:"only_a"`
	// OnlyB are entries of B that are missing from A or only partly in A.
	OnlyB []Entry `json:"only_b"`
	// Both are entries of A fully present in B, with how they matched.
	Both []Entry `json:"both"`
	// RevertedA and RevertedB are changes a revert on that branch undid. They
	// are neither missing from the other branch nor present in it.
	RevertedA []Entry `json:"reverted_a"`
	RevertedB []Entry `json:"reverted_b"`
}

// Diff compares branch (or any ref) a with b.
func Diff(ctx context.Context, a, b string, opt Options) (*Result, error) {
	ca, err := loadRange(ctx, opt.Dir, b, a, opt.Since)
	if err != nil {
		return nil, err
	}
	cb, err := loadRange(ctx, opt.Dir, a, b, opt.Since)
	if err != nil {
		return nil, err
	}
	cancelReverts(ca)
	cancelReverts(cb)
	ga, err := group(ctx, opt.Dir, ca)
	if err != nil {
		return nil, err
	}
	gb, err := group(ctx, opt.Dir, cb)
	if err != nil {
		return nil, err
	}
	matchSides(live(ca), live(cb), opt.MatchTicket)
	res := &Result{A: a, B: b, OnlyA: []Entry{}, OnlyB: []Entry{}, Both: []Entry{}, RevertedA: []Entry{}, RevertedB: []Entry{}}
	for _, g := range ga {
		e, ok := g.entry()
		switch {
		case !ok:
		case e.Status == StatusReverted:
			res.RevertedA = append(res.RevertedA, e)
		case e.Status == StatusPresent:
			res.Both = append(res.Both, e)
		default:
			res.OnlyA = append(res.OnlyA, e)
		}
	}
	for _, g := range gb {
		e, ok := g.entry()
		switch {
		case !ok:
		case e.Status == StatusReverted:
			res.RevertedB = append(res.RevertedB, e)
		case e.Status != StatusPresent:
			res.OnlyB = append(res.OnlyB, e)
		}
	}
	return res, nil
}

type change struct {
	pr      int
	anchor  *Commit   // the merge or squash commit that names the pull request
	members []*Commit // non-merge commits that make up the change
}

// entry builds the report line of a change. ok is false when nothing is left
// of it: every commit was a revert that cancelled something.
func (g *change) entry() (e Entry, ok bool) {
	var counted []*Commit
	var undone []*Commit
	for _, c := range g.members {
		switch {
		case c.dropped:
		case c.reverted || (g.anchor != nil && g.anchor.reverted && len(g.anchor.Parents) > 1):
			undone = append(undone, c)
		default:
			counted = append(counted, c)
		}
	}
	if len(g.members) == 0 && g.anchor != nil && !g.anchor.dropped {
		if g.anchor.reverted {
			undone = []*Commit{g.anchor}
		} else {
			counted = []*Commit{g.anchor}
		}
	}
	first := g.anchor
	if first == nil || first.dropped {
		switch {
		case len(counted) > 0:
			first = counted[0]
		case len(undone) > 0:
			first = undone[0]
		default:
			return Entry{}, false
		}
	}
	e = Entry{PR: g.pr, Title: cleanTitle(first.Subject), Author: first.Author, Date: first.Date}
	if len(counted) == 0 {
		if len(undone) == 0 {
			return Entry{}, false
		}
		e.Status = StatusReverted
		e.RevertedBy = undone[0].revertedBy
		if g.anchor != nil && g.anchor.reverted {
			e.RevertedBy = g.anchor.revertedBy
		}
		for _, c := range undone {
			e.Commits = append(e.Commits, CommitMatch{SHA: c.SHA, Subject: c.Subject})
		}
		return e, true
	}
	for _, c := range undone {
		e.RevertedCommits = append(e.RevertedCommits, c.SHA)
	}
	matched := 0
	for _, c := range counted {
		e.Commits = append(e.Commits, CommitMatch{SHA: c.SHA, Subject: c.Subject, Match: c.match})
		if c.match != nil {
			matched++
		}
	}
	switch {
	case matched == 0:
		e.Status = StatusMissing
	case matched == len(counted):
		e.Status = StatusPresent
	default:
		e.Status = StatusPartial
	}
	return e, true
}

// live returns the commits that take part in matching.
func live(commits []*Commit) []*Commit {
	out := make([]*Commit, 0, len(commits))
	for _, c := range commits {
		if !c.dropped && !c.reverted {
			out = append(out, c)
		}
	}
	return out
}

func cleanTitle(s string) string {
	return strings.TrimSpace(prSquash.ReplaceAllString(s, ""))
}

// group turns a list of commits (newest first) into changes.
func group(ctx context.Context, dir string, commits []*Commit) ([]*change, error) {
	inRange := make(map[string]*Commit, len(commits))
	for _, c := range commits {
		inRange[c.SHA] = c
	}
	owner := map[string]*change{}
	byPR := map[int]*change{}
	for _, c := range commits {
		if len(c.Parents) < 2 || !IsPRMerge(c.Subject) {
			continue
		}
		g := &change{pr: PRNumber(c.Subject), anchor: c}
		members, err := mergeMembers(ctx, dir, c.SHA)
		if err != nil {
			return nil, err
		}
		for _, sha := range members {
			if m := inRange[sha]; m != nil && len(m.Parents) < 2 {
				m.pr = g.pr
				g.members = append(g.members, m)
				owner[sha] = g
			}
		}
		owner[c.SHA] = g
		byPR[g.pr] = g
	}
	var out []*change
	seen := map[*change]bool{}
	for _, c := range commits {
		if g := owner[c.SHA]; g != nil {
			if !seen[g] {
				seen[g] = true
				out = append(out, g)
			}
			continue
		}
		if len(c.Parents) > 1 {
			continue // plain merge commit: carries no change of its own
		}
		pr := PRNumber(c.Subject)
		if pr == 0 {
			out = append(out, &change{members: []*Commit{c}})
			continue
		}
		if g := byPR[pr]; g != nil {
			g.members = append(g.members, c)
			continue
		}
		g := &change{pr: pr, anchor: c, members: []*Commit{c}}
		byPR[pr] = g
		out = append(out, g)
	}
	return out, nil
}

// matchSides sets the match of every commit that has a counterpart on the
// other side. Passes run strongest fingerprint first, so a commit keeps its
// strongest reason.
func matchSides(a, b []*Commit, ticket bool) {
	for _, x := range a {
		if p := PickedFrom(x.Body); p != "" {
			for _, y := range b {
				if strings.HasPrefix(y.SHA, p) {
					link(x, y, ReasonPick)
				}
			}
		}
	}
	for _, y := range b {
		if p := PickedFrom(y.Body); p != "" {
			for _, x := range a {
				if strings.HasPrefix(x.SHA, p) {
					link(x, y, ReasonPick)
				}
			}
		}
	}
	pass(a, b, ReasonPatch, func(c *Commit) string { return c.PatchID })
	pass(a, b, ReasonPR, func(c *Commit) string {
		if c.pr != 0 {
			return strconv.Itoa(c.pr)
		}
		return ""
	})
	if ticket {
		pass(a, b, ReasonTicket, func(c *Commit) string { return TicketKey(c.Subject) })
	}
}

func pass(a, b []*Commit, r Reason, key func(*Commit) string) {
	idx := map[string][]*Commit{}
	for _, y := range b {
		if k := key(y); k != "" {
			idx[k] = append(idx[k], y)
		}
	}
	for _, x := range a {
		if k := key(x); k != "" {
			for _, y := range idx[k] {
				link(x, y, r)
			}
		}
	}
}

func link(x, y *Commit, r Reason) {
	if x.match == nil {
		x.match = &Match{Other: y.SHA, Reason: r}
	}
	if y.match == nil {
		y.match = &Match{Other: x.SHA, Reason: r}
	}
}
