package cutline

import (
	"reflect"
	"strings"
	"testing"
)

// rebasePR lands a pull request the way "rebase and merge" does: its commits
// go straight onto the current branch, one by one, with no merge commit.
func (r *repo) rebasePR(subjects ...string) []string {
	var shas []string
	for _, s := range subjects {
		shas = append(shas, r.commit(s))
	}
	return shas
}

func TestRebaseMergedWithNumberOnEveryCommitIsOneEntry(t *testing.T) {
	r := newRepo(t)
	r.on("stage")
	r.rebasePR("Add parser (#9)", "Add parser tests (#9)", "Document parser (#9)")
	res := r.diff(Options{})
	if len(res.OnlyA) != 1 || res.OnlyA[0].PR != 9 || len(res.OnlyA[0].Commits) != 3 {
		t.Fatalf("OnlyA = %+v", res.OnlyA)
	}
	if res.OnlyA[0].Note != "" {
		t.Errorf("Note = %q, want empty", res.OnlyA[0].Note)
	}
}

func TestRebaseMergedWithNumberOnSomeCommits(t *testing.T) {
	r := newRepo(t)
	r.on("stage")
	r.rebasePR("Add parser", "Add parser tests (#9)")
	res := r.diff(Options{})
	if len(res.OnlyA) != 2 {
		t.Fatalf("OnlyA = %v", titles(res.OnlyA))
	}
	var named, single *Entry
	for i := range res.OnlyA {
		if res.OnlyA[i].PR == 9 {
			named = &res.OnlyA[i]
		} else {
			single = &res.OnlyA[i]
		}
	}
	if named == nil || single == nil {
		t.Fatalf("OnlyA = %+v", res.OnlyA)
	}
	if single.Title != "Add parser" || single.Note != NoteUnmatched {
		t.Errorf("single = %+v", single)
	}
	if named.Note != "" {
		t.Errorf("named Note = %q", named.Note)
	}
}

func TestRebaseMergedWithNoNumberListsEachCommitWithReason(t *testing.T) {
	r := newRepo(t)
	r.on("stage")
	r.rebasePR("First change", "Second change")
	res := r.diff(Options{})
	if got := titles(res.OnlyA); !reflect.DeepEqual(got, []string{"Second change", "First change"}) {
		t.Fatalf("OnlyA = %v", got)
	}
	for _, e := range res.OnlyA {
		if e.PR != 0 || e.Note != NoteUnmatched || e.Status != StatusMissing {
			t.Errorf("entry = %+v", e)
		}
	}
	var md strings.Builder
	if err := res.Write(&md, FormatMarkdown); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(md.String(), "single commit, no PR number") {
		t.Errorf("markdown lacks the reason:\n%s", md.String())
	}
}

func TestRebaseMergedThenPromotedByPickMatches(t *testing.T) {
	r := newRepo(t)
	r.on("stage")
	shas := r.rebasePR("First change", "Second change (#4)")
	r.on("prod")
	for _, s := range shas {
		r.git("cherry-pick", "-x", s)
	}
	res := r.diff(Options{})
	if len(res.OnlyA) != 0 || len(res.OnlyB) != 0 || len(res.Both) != 2 {
		t.Fatalf("OnlyA=%v OnlyB=%v Both=%v", titles(res.OnlyA), titles(res.OnlyB), titles(res.Both))
	}
}

func TestRebaseMergedOntoBothBranchesMatchesByPatch(t *testing.T) {
	r := newRepo(t)
	r.on("stage")
	r.rebasePR("Add parser (#9)")
	r.on("prod")
	r.commit("prod only")
	// the same change rebased onto prod: new SHA, no trailer, same patch
	r.git("cherry-pick", "stage")
	res := r.diff(Options{})
	if len(res.Both) != 1 || res.Both[0].Commits[0].Match.Reason != ReasonPatch {
		t.Fatalf("Both = %+v", res.Both)
	}
}
