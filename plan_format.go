package cutline

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// Write renders the plan in the given format. Text is a shell script of
// cherry-pick commands, one block per pull request, with warnings and dry-run
// conflicts as comments.
func (p *Plan) Write(w io.Writer, f Format) error {
	switch f {
	case FormatJSON:
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(p)
	case FormatMarkdown:
		return p.writeMarkdown(w)
	case FormatText, "":
		return p.writeText(w)
	}
	return fmt.Errorf("unknown format %q (use text, markdown or json)", f)
}

func (s Step) name() string {
	if s.PR != 0 {
		return fmt.Sprintf("#%d %s", s.PR, s.Title)
	}
	return s.Title
}

func (p *Plan) writeText(w io.Writer) error {
	pr := &printer{w: w}
	pr.printf("# Promote %d change(s) from %s into %s, oldest first\n", len(p.Steps), p.From, p.Into)
	switch {
	case !p.Checked:
		pr.printf("# Not checked: run with -dry-run to find conflicts\n")
	case p.conflicts() == 0:
		pr.printf("# Dry run: every step applies cleanly\n")
	default:
		pr.printf("# Dry run: %d step(s) conflict, marked below; the others apply\n", p.conflicts())
	}
	for _, s := range p.Steps {
		pr.printf("\n# %s\n", s.name())
		for _, wn := range p.Warnings {
			if wn.PR == s.PR && wn.PR != 0 || (s.PR == 0 && wn.PR == 0 && wn.Title != "") {
				pr.printf("# WARNING: older %s touches the same files (%s) and is not in the plan\n", needs(wn), strings.Join(wn.Files, ", "))
			}
		}
		if s.Conflict != nil {
			pr.printf("# CONFLICT at %s in: %s\n", short(s.Conflict.Commit), strings.Join(s.Conflict.Files, ", "))
		}
		for _, sha := range s.Commits {
			pr.printf("git cherry-pick -x %s\n", sha)
		}
	}
	return pr.err
}

func needs(w Warning) string {
	if w.Needs != 0 {
		return fmt.Sprintf("#%d %s", w.Needs, w.Title)
	}
	return w.Title
}

func (p *Plan) writeMarkdown(w io.Writer) error {
	pr := &printer{w: w}
	pr.printf("## Promote from `%s` into `%s` (%d)\n\n", p.From, p.Into, len(p.Steps))
	for i, s := range p.Steps {
		status := ""
		switch {
		case s.Conflict != nil:
			status = " - conflict at `" + short(s.Conflict.Commit) + "` in " + strings.Join(s.Conflict.Files, ", ")
		case p.Checked:
			status = " - applies cleanly"
		}
		pr.printf("%d. %s%s\n", i+1, s.name(), status)
	}
	if len(p.Warnings) > 0 {
		pr.printf("\n## Warnings\n\n")
		for _, wn := range p.Warnings {
			pr.printf("- #%d: older %s touches the same files (%s) and is not in the plan\n", wn.PR, needs(wn), strings.Join(wn.Files, ", "))
		}
	}
	if !p.Checked {
		pr.printf("\nNot checked: run with `-dry-run` to find conflicts.\n")
	}
	return pr.err
}

func (p *Plan) conflicts() int {
	n := 0
	for _, s := range p.Steps {
		if s.Conflict != nil {
			n++
		}
	}
	return n
}
