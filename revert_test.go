package cutline

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

func TestRevertOf(t *testing.T) {
	body := "This reverts commit abcdef1234567890."
	if got := RevertOf(`Revert "Add search (#1)"`, body); got != "abcdef1234567890" {
		t.Errorf("RevertOf = %q", got)
	}
	if got := RevertOf("Add search", body); got != "" {
		t.Errorf("non-revert subject: RevertOf = %q", got)
	}
	if got := RevertOf(`Revert "x"`, "no id"); got != "" {
		t.Errorf("no body id: RevertOf = %q", got)
	}
}

func (r *repo) revert(sha, title string) string {
	r.git("revert", "--no-edit", sha)
	return r.head()
}

func TestRevertedSquashIsReportedNotMissing(t *testing.T) {
	r := newRepo(t)
	r.on("stage")
	sq := r.squashPR("1", "Add search", "a")
	r.squashPR("2", "Add export", "b")
	rv := r.revert(sq, "")
	res := r.diff(Options{})
	if got := titles(res.OnlyA); !reflect.DeepEqual(got, []string{"Add export"}) {
		t.Fatalf("OnlyA = %v", got)
	}
	if len(res.RevertedA) != 1 || res.RevertedA[0].PR != 1 || res.RevertedA[0].Status != StatusReverted || res.RevertedA[0].RevertedBy != rv {
		t.Fatalf("RevertedA = %+v", res.RevertedA)
	}
}

func TestRevertOfRevertKeepsTheChange(t *testing.T) {
	r := newRepo(t)
	r.on("stage")
	sq := r.squashPR("1", "Add search", "a")
	r1 := r.revert(sq, "")
	r.revert(r1, "")
	res := r.diff(Options{})
	if got := titles(res.OnlyA); !reflect.DeepEqual(got, []string{"Add search"}) {
		t.Fatalf("OnlyA = %v", got)
	}
	if len(res.RevertedA) != 0 {
		t.Fatalf("RevertedA = %+v", res.RevertedA)
	}
	if len(res.OnlyA[0].Commits) != 1 {
		t.Fatalf("commits = %+v", res.OnlyA[0].Commits)
	}
}

func TestRevertPairOnOneSideCancelsOut(t *testing.T) {
	r := newRepo(t)
	r.on("prod")
	hot := r.commit("hotfix")
	r.revert(hot, "")
	res := r.diff(Options{})
	if len(res.OnlyB) != 0 || len(res.OnlyA) != 0 {
		t.Fatalf("OnlyA=%v OnlyB=%v", titles(res.OnlyA), titles(res.OnlyB))
	}
	if len(res.RevertedB) != 1 || res.RevertedB[0].Title != "hotfix" {
		t.Fatalf("RevertedB = %+v", res.RevertedB)
	}
}

func TestRevertOfMergePR(t *testing.T) {
	r := newRepo(t)
	r.on("stage")
	r.mergePR("5", "one", "two")
	merge := r.head()
	r.git("revert", "--no-edit", "-m", "1", merge)
	res := r.diff(Options{})
	if len(res.OnlyA) != 0 {
		t.Fatalf("OnlyA = %v", titles(res.OnlyA))
	}
	if len(res.RevertedA) != 1 || res.RevertedA[0].PR != 5 || len(res.RevertedA[0].Commits) != 2 {
		t.Fatalf("RevertedA = %+v", res.RevertedA)
	}
}

func TestPartialRevertInsideMergePR(t *testing.T) {
	r := newRepo(t)
	r.on("stage")
	shas := r.mergePR("7", "keep", "drop")
	r.revert(shas[1], "")
	res := r.diff(Options{})
	if len(res.OnlyA) != 1 || res.OnlyA[0].PR != 7 || len(res.OnlyA[0].Commits) != 1 {
		t.Fatalf("OnlyA = %+v", res.OnlyA)
	}
	if !reflect.DeepEqual(res.OnlyA[0].RevertedCommits, []string{shas[1]}) {
		t.Fatalf("RevertedCommits = %v", res.OnlyA[0].RevertedCommits)
	}
	if len(res.RevertedA) != 0 {
		t.Fatalf("RevertedA = %+v", res.RevertedA)
	}
}

func TestRevertOfPickedCommitCancelsOnThatBranch(t *testing.T) {
	r := newRepo(t)
	r.on("stage")
	sq := r.squashPR("1", "Add search", "a")
	r.on("prod")
	r.git("cherry-pick", "-x", sq)
	r.revert(r.head(), "")
	res := r.diff(Options{})
	if len(res.RevertedB) != 1 || len(res.OnlyB) != 0 {
		t.Fatalf("RevertedB=%+v OnlyB=%v", res.RevertedB, titles(res.OnlyB))
	}
	if got := titles(res.OnlyA); !reflect.DeepEqual(got, []string{"Add search"}) {
		t.Fatalf("OnlyA = %v", got)
	}
}

func TestRevertOfCommitOutsideRangeIsAChange(t *testing.T) {
	r := newRepo(t)
	r.on("prod")
	base := r.commit("shared work")
	r.git("branch", "-f", "stage", "prod")
	r.revert(base, "")
	res := r.diff(Options{})
	if len(res.OnlyB) != 1 || len(res.RevertedB) != 0 {
		t.Fatalf("OnlyB=%v RevertedB=%+v", titles(res.OnlyB), res.RevertedB)
	}
}

func TestRevertedShownInOutputs(t *testing.T) {
	r := newRepo(t)
	r.on("stage")
	sq := r.squashPR("1", "Add search", "a")
	r.revert(sq, "")
	res := r.diff(Options{})
	for _, f := range []Format{FormatText, FormatMarkdown} {
		var b bytes.Buffer
		if err := res.Write(&b, f); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(b.String(), "Add search") || !strings.Contains(b.String(), "reverted by") {
			t.Errorf("%s output lacks the reverted entry:\n%s", f, b.String())
		}
	}
}
