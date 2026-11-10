package cutline

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
)

var conventional = regexp.MustCompile(`^([A-Za-z]+)(?:\(([^)]*)\))?(!)?:\s+(.*)$`)

// sectionOf maps a conventional-commit type to a release-notes heading.
var sectionOf = map[string]string{
	"feat": "Features", "fix": "Fixes", "perf": "Performance", "docs": "Documentation",
	"refactor": "Maintenance", "chore": "Maintenance", "ci": "Maintenance",
	"build": "Maintenance", "test": "Maintenance", "style": "Maintenance",
}

// sectionOrder is the order headings appear in; Other comes last.
var sectionOrder = []string{"Features", "Fixes", "Performance", "Documentation", "Maintenance", "Other"}

// NoteItem is one change in the release notes.
type NoteItem struct {
	PR     int    `json:"pr,omitempty"`
	Title  string `json:"title"`
	Author string `json:"author"`
	// Breaking is set when the title says so with "!" after the type.
	Breaking bool `json:"breaking,omitempty"`
	// Labels and URL come from an Enricher.
	Labels []string `json:"labels,omitempty"`
	URL    string   `json:"url,omitempty"`
}

// Section groups items under one heading.
type Section struct {
	Name  string     `json:"name"`
	Items []NoteItem `json:"items"`
}

// Notes are release notes for the changes that go from A to B.
type Notes struct {
	From     string    `json:"from"`
	Into     string    `json:"into"`
	Sections []Section `json:"sections"`
	// Notices are non-fatal problems, such as a refused hosting-service request.
	Notices []string `json:"notices,omitempty"`
}

// BuildNotes writes release notes for the changes in a that are not yet in b,
// or only those named in opt.Picks. Titles that follow the conventional commit
// form ("feat(api): add x") are grouped by type; the rest go under Other.
func BuildNotes(ctx context.Context, a, b string, opt PlanOptions) (*Notes, error) {
	res, err := Diff(ctx, a, b, opt.Options)
	if err != nil {
		return nil, err
	}
	want := map[int]bool{}
	for _, n := range opt.Picks {
		want[n] = true
	}
	known := map[int]bool{}
	by := map[string][]NoteItem{}
	// OnlyA is newest first; notes read oldest first.
	for i := len(res.OnlyA) - 1; i >= 0; i-- {
		e := res.OnlyA[i]
		known[e.PR] = true
		if len(want) > 0 && !want[e.PR] {
			continue
		}
		item := NoteItem{PR: e.PR, Title: e.Title, Author: e.Author, Labels: e.Labels, URL: e.URL}
		sec := sectionFromLabels(e.Labels)
		if m := conventional.FindStringSubmatch(e.Title); m != nil {
			if s, ok := sectionOf[strings.ToLower(m[1])]; ok {
				sec = s
				item.Title = m[4]
				if m[2] != "" {
					item.Title = m[2] + ": " + m[4]
				}
				item.Breaking = m[3] == "!"
			}
		}
		by[sec] = append(by[sec], item)
	}
	for n := range want {
		if !known[n] {
			return nil, fmt.Errorf("pull request #%d is not in %s or is already in %s", n, a, b)
		}
	}
	n := &Notes{From: a, Into: b, Sections: []Section{}, Notices: res.Notices}
	for _, name := range sectionOrder {
		if items := by[name]; len(items) > 0 {
			n.Sections = append(n.Sections, Section{Name: name, Items: items})
		}
	}
	return n, nil
}

func (it NoteItem) line() string {
	var b strings.Builder
	b.WriteString(it.Title)
	if it.Breaking {
		b.WriteString(" (breaking)")
	}
	if it.PR != 0 {
		fmt.Fprintf(&b, " (#%d)", it.PR)
	}
	if it.Author != "" {
		fmt.Fprintf(&b, " by %s", it.Author)
	}
	return b.String()
}

// Write renders the notes. Text and markdown are the same list; json is the
// structure.
func (n *Notes) Write(w io.Writer, f Format) error {
	switch f {
	case FormatJSON:
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(n)
	case FormatText, FormatMarkdown, "":
		p := &printer{w: w}
		for i, s := range n.Sections {
			if i > 0 {
				p.printf("\n")
			}
			p.printf("## %s\n\n", s.Name)
			for _, it := range s.Items {
				p.printf("- %s\n", it.line())
			}
		}
		if len(n.Sections) == 0 {
			p.printf("Nothing to release: everything in %s is already in %s.\n", n.From, n.Into)
		}
		writeNotices(p, n.Notices)
		return p.err
	}
	return fmt.Errorf("unknown format %q (use text, markdown or json)", f)
}

// labelSection maps common pull request labels to a release-notes heading.
var labelSection = map[string]string{
	"feature": "Features", "enhancement": "Features", "feat": "Features",
	"bug": "Fixes", "fix": "Fixes", "bugfix": "Fixes",
	"performance": "Performance", "perf": "Performance",
	"documentation": "Documentation", "docs": "Documentation",
	"chore": "Maintenance", "maintenance": "Maintenance", "dependencies": "Maintenance", "refactor": "Maintenance",
}

// sectionFromLabels picks a heading from pull request labels, or Other. A
// conventional-commit title, when there is one, overrides it.
func sectionFromLabels(labels []string) string {
	for _, l := range labels {
		if s, ok := labelSection[strings.ToLower(strings.TrimSpace(l))]; ok {
			return s
		}
	}
	return "Other"
}
