// Package cutline reports which pull requests are in one branch but not in
// another, matching changes across squash merges, merge commits and
// cherry-picks.
//
// Git alone lists SHAs, and "git cherry" matches by patch id, which a squash
// of several commits or a conflict resolution changes. cutline keeps a chain of
// fingerprints per commit (cherry-pick trailer, patch id, pull request number,
// optionally a ticket key) and reports at the level of a pull request.
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

	pr    int // pull request number, own or inherited from the merge that brought the commit in
	match *Match
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
	ga, err := group(ctx, opt.Dir, ca)
	if err != nil {
		return nil, err
	}
	gb, err := group(ctx, opt.Dir, cb)
	if err != nil {
		return nil, err
	}
	matchSides(ca, cb, opt.MatchTicket)
	res := &Result{A: a, B: b, OnlyA: []Entry{}, OnlyB: []Entry{}, Both: []Entry{}}
	for _, g := range ga {
		if e := g.entry(); e.Status == StatusPresent {
			res.Both = append(res.Both, e)
		} else {
			res.OnlyA = append(res.OnlyA, e)
		}
	}
	for _, g := range gb {
		if e := g.entry(); e.Status != StatusPresent {
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

func (g *change) entry() Entry {
	counted := g.members
	if len(counted) == 0 {
		counted = []*Commit{g.anchor}
	}
	first := g.anchor
	if first == nil {
		first = counted[0]
	}
	e := Entry{PR: g.pr, Title: cleanTitle(first.Subject), Author: first.Author, Date: first.Date}
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
	return e
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
