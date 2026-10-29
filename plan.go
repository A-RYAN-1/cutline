package cutline

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
)

// PlanOptions for Plan.
type PlanOptions struct {
	Options
	// Picks are the pull request numbers to promote. Empty means every entry
	// of A that is missing from B or only partly in B.
	Picks []int
	// DryRun applies the picks in a temporary worktree on B and reports the
	// steps that conflict. The repository's branches and working tree are
	// not touched.
	DryRun bool
}

// Step is one pull request (or single commit) to promote.
type Step struct {
	PR    int    `json:"pr,omitempty"`
	Title string `json:"title"`
	// Commits to cherry-pick, oldest first. Commits of the entry already on
	// the other branch are left out.
	Commits []string `json:"commits"`
	// Files the commits touch.
	Files []string `json:"files"`
	// Conflict is set by a dry run when the step does not apply.
	Conflict *Conflict `json:"conflict,omitempty"`
}

// Conflict is where a dry run stopped on a step.
type Conflict struct {
	Commit string   `json:"commit"`
	Files  []string `json:"files"`
}

// Warning says a selected step touches the same files as an older change in A
// that is not part of the plan and not on B, so the pick may need it first.
type Warning struct {
	PR    int      `json:"pr"`
	Needs int      `json:"needs,omitempty"`
	Title string   `json:"title"`
	Files []string `json:"files"`
}

// Plan is the ordered list of picks that bring the chosen changes from A to B.
type Plan struct {
	From     string    `json:"from"`
	Into     string    `json:"into"`
	Steps    []Step    `json:"steps"`
	Warnings []Warning `json:"warnings"`
	// Checked is true when a dry run was made.
	Checked bool `json:"checked"`
}

// BuildPlan orders the picks that promote changes from branch a to branch b.
func BuildPlan(ctx context.Context, a, b string, opt PlanOptions) (*Plan, error) {
	res, err := Diff(ctx, a, b, opt.Options)
	if err != nil {
		return nil, err
	}
	order, parents, err := topoOrder(ctx, opt.Dir, b, a)
	if err != nil {
		return nil, err
	}
	want := map[int]bool{}
	for _, n := range opt.Picks {
		want[n] = true
	}
	known := map[int]bool{}
	type cand struct {
		entry Entry
		step  Step
		first int
	}
	var picked, left []cand
	for _, e := range res.OnlyA {
		known[e.PR] = true
		c := cand{entry: e, step: Step{PR: e.PR, Title: e.Title}, first: -1}
		for _, cm := range e.Commits {
			if cm.Match != nil || parents[cm.SHA] > 1 {
				continue
			}
			c.step.Commits = append(c.step.Commits, cm.SHA)
		}
		if len(c.step.Commits) == 0 {
			continue
		}
		sort.Slice(c.step.Commits, func(i, j int) bool { return order[c.step.Commits[i]] < order[c.step.Commits[j]] })
		c.first = order[c.step.Commits[0]]
		files := map[string]bool{}
		for _, sha := range c.step.Commits {
			fs, err := commitFiles(ctx, opt.Dir, sha)
			if err != nil {
				return nil, err
			}
			for _, f := range fs {
				files[f] = true
			}
		}
		c.step.Files = sortedKeys(files)
		if len(want) == 0 || (e.PR != 0 && want[e.PR]) {
			picked = append(picked, c)
		} else {
			left = append(left, c)
		}
	}
	for n := range want {
		if !known[n] {
			return nil, fmt.Errorf("pull request #%d is not in %s or is already in %s", n, a, b)
		}
	}
	sort.Slice(picked, func(i, j int) bool { return picked[i].first < picked[j].first })
	p := &Plan{From: a, Into: b, Steps: []Step{}, Warnings: []Warning{}}
	for _, c := range picked {
		p.Steps = append(p.Steps, c.step)
		for _, o := range left {
			if o.first > c.first {
				continue
			}
			if shared := intersect(c.step.Files, o.step.Files); len(shared) > 0 {
				p.Warnings = append(p.Warnings, Warning{PR: c.step.PR, Needs: o.step.PR, Title: o.step.Title, Files: shared})
			}
		}
	}
	if opt.DryRun {
		if err := dryRun(ctx, opt.Dir, b, p); err != nil {
			return nil, err
		}
		p.Checked = true
	}
	return p, nil
}

// topoOrder numbers the commits reachable from include but not exclude, oldest
// first, and counts the parents of each.
func topoOrder(ctx context.Context, dir, exclude, include string) (map[string]int, map[string]int, error) {
	out, err := runGit(ctx, dir, nil, "rev-list", "--topo-order", "--reverse", "--parents", exclude+".."+include, "--")
	if err != nil {
		return nil, nil, err
	}
	order, parents := map[string]int{}, map[string]int{}
	for i, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		order[f[0]] = i
		parents[f[0]] = len(f) - 1
	}
	return order, parents, nil
}

func commitFiles(ctx context.Context, dir, sha string) ([]string, error) {
	out, err := runGit(ctx, dir, nil, "diff-tree", "--root", "--no-commit-id", "--name-only", "-r", sha)
	if err != nil {
		return nil, err
	}
	return strings.Fields(string(out)), nil
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func intersect(a, b []string) []string {
	in := map[string]bool{}
	for _, f := range a {
		in[f] = true
	}
	var out []string
	for _, f := range b {
		if in[f] {
			out = append(out, f)
		}
	}
	return out
}

// dryRun applies every step in a temporary detached worktree on b. A step that
// conflicts is undone and marked, and the next step is tried on top of the
// steps that applied.
func dryRun(ctx context.Context, dir, b string, p *Plan) (err error) {
	tmp, err := os.MkdirTemp("", "cutline-dry-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	if _, err := runGit(ctx, dir, nil, "-c", "core.autocrlf=false", "worktree", "add", "--detach", "--quiet", tmp, b); err != nil {
		return err
	}
	defer func() {
		if _, rerr := runGit(ctx, dir, nil, "worktree", "remove", "--force", tmp); rerr != nil && err == nil {
			err = rerr
		}
	}()
	ident := []string{"-c", "user.name=cutline", "-c", "user.email=cutline@localhost", "-c", "commit.gpgsign=false", "-c", "core.autocrlf=false"}
	git := func(args ...string) ([]byte, error) {
		return runGit(ctx, tmp, nil, append(append([]string{}, ident...), args...)...)
	}
	for i := range p.Steps {
		st := &p.Steps[i]
		head, err := git("rev-parse", "HEAD")
		if err != nil {
			return err
		}
		for _, sha := range st.Commits {
			_, perr := git("cherry-pick", "--allow-empty", "--keep-redundant-commits", sha)
			if perr == nil {
				continue
			}
			files, _ := git("diff", "--name-only", "--diff-filter=U")
			if len(strings.Fields(string(files))) == 0 {
				return perr // not a conflict: report git's own error
			}
			st.Conflict = &Conflict{Commit: sha, Files: strings.Fields(string(files))}
			_, _ = git("cherry-pick", "--abort")
			if _, err := git("reset", "--hard", strings.TrimSpace(string(head))); err != nil {
				return err
			}
			break
		}
	}
	return nil
}
