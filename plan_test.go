package cutline

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

// sharedRepo is a repository whose stage and prod both start with shared.txt.
func sharedRepo(t *testing.T) *repo {
	r := newRepo(t)
	r.on("main")
	r.write("shared.txt", "a\n")
	r.git("add", "-A")
	r.git("commit", "-q", "-m", "add shared")
	r.git("branch", "-f", "stage", "main")
	r.git("branch", "-f", "prod", "main")
	return r
}

// editPR lands a pull request that rewrites shared.txt, as one squash commit.
func (r *repo) editPR(pr, title, content string) string {
	r.write("shared.txt", content)
	r.git("add", "-A")
	r.git("commit", "-q", "-m", title+" (#"+pr+")")
	return r.head()
}

func (r *repo) plan(opt PlanOptions) (*Plan, error) {
	opt.Dir = r.dir
	return BuildPlan(context.Background(), "stage", "prod", opt)
}

func stepPRs(p *Plan) []int {
	out := []int{}
	for _, s := range p.Steps {
		out = append(out, s.PR)
	}
	return out
}

func TestPlanOrdersOldestFirst(t *testing.T) {
	r := newRepo(t)
	r.on("stage")
	r.squashPR("1", "One", "a")
	r.squashPR("2", "Two", "b")
	r.squashPR("3", "Three", "c")
	p, err := r.plan(PlanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := stepPRs(p); !reflect.DeepEqual(got, []int{1, 2, 3}) {
		t.Fatalf("steps = %v", got)
	}
	p, err = r.plan(PlanOptions{Picks: []int{3, 1}})
	if err != nil {
		t.Fatal(err)
	}
	if got := stepPRs(p); !reflect.DeepEqual(got, []int{1, 3}) {
		t.Fatalf("picked steps = %v, want oldest first", got)
	}
	if p.Checked {
		t.Fatal("plan without -dry-run claims to be checked")
	}
}

func TestPlanSkipsCommitsAlreadyOnTarget(t *testing.T) {
	r := newRepo(t)
	r.on("stage")
	shas := r.mergePR("1", "first", "second", "third")
	r.on("prod")
	r.git("cherry-pick", "-x", shas[0])
	p, err := r.plan(PlanOptions{Picks: []int{1}})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Steps) != 1 || !reflect.DeepEqual(p.Steps[0].Commits, shas[1:]) {
		t.Fatalf("steps = %+v, want commits %v", p.Steps, shas[1:])
	}
}

func TestPlanWarnsAboutOlderChangeOnSameFile(t *testing.T) {
	r := sharedRepo(t)
	r.on("stage")
	r.editPR("1", "Change to b", "b\n")
	r.squashPR("2", "Unrelated", "x")
	r.editPR("3", "Change to c", "c\n")
	p, err := r.plan(PlanOptions{Picks: []int{3}})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Warnings) != 1 || p.Warnings[0].PR != 3 || p.Warnings[0].Needs != 1 ||
		!reflect.DeepEqual(p.Warnings[0].Files, []string{"shared.txt"}) {
		t.Fatalf("warnings = %+v", p.Warnings)
	}
	// Picking both removes the warning; the newer change never warns about an
	// older one that is in the plan.
	p, err = r.plan(PlanOptions{Picks: []int{1, 3}})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Warnings) != 0 {
		t.Fatalf("warnings = %+v", p.Warnings)
	}
}

func TestPlanDoesNotWarnAboutNewerChange(t *testing.T) {
	r := sharedRepo(t)
	r.on("stage")
	r.editPR("1", "Change to b", "b\n")
	r.editPR("2", "Change to c", "c\n")
	p, err := r.plan(PlanOptions{Picks: []int{1}})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Warnings) != 0 {
		t.Fatalf("warnings = %+v", p.Warnings)
	}
}

func TestDryRunFindsConflictAndLeavesRepoUntouched(t *testing.T) {
	r := sharedRepo(t)
	r.on("stage")
	r.editPR("1", "Change to b", "b\n")
	r.editPR("2", "Change to c", "c\n")
	r.squashPR("3", "Independent", "z")
	r.on("prod")
	before := r.git("rev-parse", "stage", "prod", "HEAD")

	p, err := r.plan(PlanOptions{Picks: []int{2, 3}, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if !p.Checked {
		t.Fatal("dry run not marked checked")
	}
	if p.Steps[0].PR != 2 || p.Steps[0].Conflict == nil ||
		!reflect.DeepEqual(p.Steps[0].Conflict.Files, []string{"shared.txt"}) {
		t.Fatalf("step 2 = %+v, want a conflict in shared.txt", p.Steps[0])
	}
	if p.Steps[1].PR != 3 || p.Steps[1].Conflict != nil {
		t.Fatalf("step 3 = %+v, want it to apply after the conflict is undone", p.Steps[1])
	}
	if after := r.git("rev-parse", "stage", "prod", "HEAD"); after != before {
		t.Fatalf("refs moved: %s -> %s", before, after)
	}
	if got := r.git("status", "--porcelain"); got != "" {
		t.Fatalf("working tree dirty: %s", got)
	}
	if n := strings.Count(r.git("worktree", "list"), "\n"); n != 0 {
		t.Fatalf("temporary worktree left behind:\n%s", r.git("worktree", "list"))
	}
}

func TestDryRunCleanWhenOlderChangeIsIncluded(t *testing.T) {
	r := sharedRepo(t)
	r.on("stage")
	r.editPR("1", "Change to b", "b\n")
	r.editPR("2", "Change to c", "c\n")
	p, err := r.plan(PlanOptions{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range p.Steps {
		if s.Conflict != nil {
			t.Fatalf("step %d conflicts: %+v", s.PR, s.Conflict)
		}
	}
	if got := stepPRs(p); !reflect.DeepEqual(got, []int{1, 2}) {
		t.Fatalf("steps = %v", got)
	}
}

func TestPlanUnknownPick(t *testing.T) {
	r := newRepo(t)
	r.on("stage")
	r.squashPR("1", "One", "a")
	if _, err := r.plan(PlanOptions{Picks: []int{9}}); err == nil {
		t.Fatal("want an error for a pull request that is not pending")
	}
}

func TestPlanOutputFormats(t *testing.T) {
	r := sharedRepo(t)
	r.on("stage")
	r.editPR("1", "Change to b", "b\n")
	r.editPR("2", "Change to c", "c\n")
	p, err := r.plan(PlanOptions{Picks: []int{2}, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	var text, md, js strings.Builder
	for f, w := range map[Format]*strings.Builder{FormatText: &text, FormatMarkdown: &md, FormatJSON: &js} {
		if err := p.Write(w, f); err != nil {
			t.Fatal(err)
		}
	}
	for _, want := range []string{"git cherry-pick -x " + p.Steps[0].Commits[0], "WARNING", "CONFLICT"} {
		if !strings.Contains(text.String(), want) {
			t.Errorf("text lacks %q:\n%s", want, text.String())
		}
	}
	for _, want := range []string{"#2 Change to c", "conflict", "## Warnings"} {
		if !strings.Contains(md.String(), want) {
			t.Errorf("markdown lacks %q:\n%s", want, md.String())
		}
	}
	if !strings.Contains(js.String(), `"checked": true`) {
		t.Errorf("json lacks checked:\n%s", js.String())
	}
	if err := p.Write(&text, "bogus"); err == nil {
		t.Error("want an error for an unknown format")
	}
}
