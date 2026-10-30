package cutline

import (
	"context"
	"testing"
)

// sharedPR builds a merge-style pull request of three commits, the first of
// which prod already has (it was merged into prod early, for example by a
// hotfix branch cut from the pull request branch).
func sharedPR(t *testing.T) (*repo, []string) {
	r := newRepo(t)
	r.on("stage")
	r.git("checkout", "-q", "-b", "pr3", "stage")
	c1 := r.commit("part one")
	r.git("branch", "-f", "prod", c1)
	c2 := r.commit("part two")
	c3 := r.commit("part three")
	r.on("stage")
	r.git("merge", "-q", "--no-ff", "-m", "Merge pull request #3 from someone/pr3", "pr3")
	return r, []string{c1, c2, c3}
}

func TestSharedCommitIsListedAndCounted(t *testing.T) {
	r, shas := sharedPR(t)
	res := r.diff(Options{})
	if len(res.OnlyA) != 1 {
		t.Fatalf("OnlyA = %+v", res.OnlyA)
	}
	e := res.OnlyA[0]
	if e.PR != 3 || len(e.Commits) != 3 || e.Status != StatusPartial {
		t.Fatalf("entry = %+v", e)
	}
	var shared, open []string
	for _, c := range e.Commits {
		if c.Match != nil && c.Match.Reason == ReasonShared {
			shared = append(shared, c.SHA)
		} else if c.Match == nil {
			open = append(open, c.SHA)
		}
	}
	if len(shared) != 1 || shared[0] != shas[0] || len(open) != 2 {
		t.Fatalf("shared=%v open=%v", shared, open)
	}
}

func TestPlanLeavesSharedCommitOut(t *testing.T) {
	r, shas := sharedPR(t)
	p, err := BuildPlan(context.Background(), "stage", "prod", PlanOptions{Options: Options{Dir: r.dir}})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Steps) != 1 || len(p.Steps[0].Commits) != 2 {
		t.Fatalf("steps = %+v", p.Steps)
	}
	for _, c := range p.Steps[0].Commits {
		if c == shas[0] {
			t.Fatalf("plan picks the shared commit %s", c)
		}
	}
}

func TestFullySharedPRIsNotAnEntry(t *testing.T) {
	r := newRepo(t)
	r.on("stage")
	r.mergePR("8", "one", "two")
	r.git("branch", "-f", "prod", "stage")
	res := r.diff(Options{})
	if len(res.OnlyA)+len(res.OnlyB)+len(res.Both) != 0 {
		t.Fatalf("res = %+v", res)
	}
}
