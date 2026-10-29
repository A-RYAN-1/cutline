package cutline

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// Format names an output format.
type Format string

// The output formats.
const (
	FormatText     Format = "text"
	FormatMarkdown Format = "markdown"
	FormatJSON     Format = "json"
)

// Write renders the result in the given format.
func (r *Result) Write(w io.Writer, f Format) error {
	switch f {
	case FormatJSON:
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(r)
	case FormatMarkdown:
		return r.writeMarkdown(w)
	case FormatText, "":
		return r.writeText(w)
	}
	return fmt.Errorf("unknown format %q (use text, markdown or json)", f)
}

func (r *Result) writeText(w io.Writer) error {
	p := &printer{w: w}
	p.printf("In %s but not in %s: %d\n", r.A, r.B, len(r.OnlyA))
	for _, e := range r.OnlyA {
		p.printf("  %s\n", e.line())
	}
	p.printf("In %s but not in %s: %d\n", r.B, r.A, len(r.OnlyB))
	for _, e := range r.OnlyB {
		p.printf("  %s\n", e.line())
	}
	p.printf("In both (matched): %d\n", len(r.Both))
	return p.err
}

func (r *Result) writeMarkdown(w io.Writer) error {
	p := &printer{w: w}
	p.printf("## In `%s` but not in `%s` (%d)\n\n", r.A, r.B, len(r.OnlyA))
	for _, e := range r.OnlyA {
		p.printf("- %s\n", e.line())
	}
	p.printf("\n## In `%s` but not in `%s` (%d)\n\n", r.B, r.A, len(r.OnlyB))
	for _, e := range r.OnlyB {
		p.printf("- %s\n", e.line())
	}
	p.printf("\n## In both, matched (%d)\n\n", len(r.Both))
	for _, e := range r.Both {
		p.printf("- %s (%s)\n", e.line(), e.reasons())
	}
	return p.err
}

// line is one entry on one line: "#12 Title (abc1234, Author, 2026-10-01) [partial 1/3]".
func (e Entry) line() string {
	var b strings.Builder
	if e.PR != 0 {
		fmt.Fprintf(&b, "#%d ", e.PR)
	}
	b.WriteString(e.Title)
	date := e.Date
	if len(date) >= 10 {
		date = date[:10]
	}
	fmt.Fprintf(&b, " (%s, %s, %s)", short(e.Commits[0].SHA), e.Author, date)
	if e.Status == StatusPartial {
		n := 0
		for _, c := range e.Commits {
			if c.Match != nil {
				n++
			}
		}
		fmt.Fprintf(&b, " [partial %d/%d]", n, len(e.Commits))
	}
	return b.String()
}

func (e Entry) reasons() string {
	seen := map[Reason]bool{}
	var out []string
	for _, c := range e.Commits {
		if c.Match != nil && !seen[c.Match.Reason] {
			seen[c.Match.Reason] = true
			out = append(out, string(c.Match.Reason))
		}
	}
	return strings.Join(out, ", ")
}

func short(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

type printer struct {
	w   io.Writer
	err error
}

func (p *printer) printf(format string, a ...any) {
	if p.err == nil {
		_, p.err = fmt.Fprintf(p.w, format, a...)
	}
}
