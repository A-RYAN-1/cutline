package cutline

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestParsers(t *testing.T) {
	cases := []struct {
		subject string
		pr      int
	}{
		{"Add retries (#123)", 123},
		{"Merge pull request #45 from a/b", 45},
		{"Fix thing (#7) extra", 0},
		{"plain subject", 0},
		{"Revert \"x (#9)\"", 0},
	}
	for _, c := range cases {
		if got := PRNumber(c.subject); got != c.pr {
			t.Errorf("PRNumber(%q) = %d, want %d", c.subject, got, c.pr)
		}
	}
	if got := PickedFrom("body\n\n(cherry picked from commit abcdef1234567)"); got != "abcdef1234567" {
		t.Errorf("PickedFrom = %q", got)
	}
	if got := PickedFrom("no trailer"); got != "" {
		t.Errorf("PickedFrom = %q, want empty", got)
	}
	if got := TicketKey("ABC-12 fix the thing"); got != "ABC-12" {
		t.Errorf("TicketKey = %q", got)
	}
	if got := TicketKey("lowercase abc-12"); got != "" {
		t.Errorf("TicketKey = %q, want empty", got)
	}
}

func TestSquashOnlyOnStage(t *testing.T) {
	r := newRepo(t)
	r.on("stage")
	r.squashPR("1", "Add search", "wip a", "wip b")
	res := r.diff(Options{})
	if !reflect.DeepEqual(titles(res.OnlyA), []string{"Add search"}) {
		t.Fatalf("OnlyA = %v", titles(res.OnlyA))
	}
	if res.OnlyA[0].PR != 1 || res.OnlyA[0].Status != StatusMissing {
		t.Fatalf("entry = %+v", res.OnlyA[0])
	}
	if len(res.OnlyB) != 0 || len(res.Both) != 0 {
		t.Fatalf("OnlyB=%v Both=%v", titles(res.OnlyB), titles(res.Both))
	}
}

func TestCherryPickWithTrailer(t *testing.T) {
	r := newRepo(t)
	r.on("stage")
	sha := r.squashPR("2", "Add export", "wip")
	r.on("prod")
	r.git("cherry-pick", "-x", sha)
	res := r.diff(Options{})
	if len(res.OnlyA) != 0 || len(res.OnlyB) != 0 {
		t.Fatalf("OnlyA=%v OnlyB=%v", titles(res.OnlyA), titles(res.OnlyB))
	}
	if len(res.Both) != 1 || res.Both[0].Commits[0].Match.Reason != ReasonPick {
		t.Fatalf("Both = %+v", res.Both)
	}
}

func TestCherryPickWithoutTrailerMatchesByPatch(t *testing.T) {
	r := newRepo(t)
	r.on("stage")
	sha := r.squashPR("3", "Add import", "wip")
	r.on("prod")
	r.git("cherry-pick", sha)
	// strip the PR number so only the patch id can match
	r.git("commit", "-q", "--amend", "-m", "unrelated subject")
	res := r.diff(Options{})
	if len(res.OnlyA) != 0 || len(res.OnlyB) != 0 {
		t.Fatalf("OnlyA=%v OnlyB=%v", titles(res.OnlyA), titles(res.OnlyB))
	}
	if got := res.Both[0].Commits[0].Match.Reason; got != ReasonPatch {
		t.Fatalf("reason = %q, want %q", got, ReasonPatch)
	}
}

func TestResolvedPickMatchesByPRNumber(t *testing.T) {
	r := newRepo(t)
	r.on("stage")
	r.squashPR("4", "Add billing page", "wip")
	// prod lands the same pull request by hand with a different patch
	r.on("prod")
	r.write("other.txt", "different content\n")
	r.git("add", "-A")
	r.git("commit", "-q", "-m", "Add billing page (#4)")
	res := r.diff(Options{})
	if len(res.OnlyA) != 0 || len(res.OnlyB) != 0 {
		t.Fatalf("OnlyA=%v OnlyB=%v", titles(res.OnlyA), titles(res.OnlyB))
	}
	if got := res.Both[0].Commits[0].Match.Reason; got != ReasonPR {
		t.Fatalf("reason = %q, want %q", got, ReasonPR)
	}
}

func TestMergeCommitPRPartiallyPicked(t *testing.T) {
	r := newRepo(t)
	r.on("stage")
	shas := r.mergePR("5", "part one", "part two", "part three")
	r.on("prod")
	r.git("cherry-pick", "-x", shas[0])
	res := r.diff(Options{})
	if len(res.OnlyA) != 1 {
		t.Fatalf("OnlyA = %v", titles(res.OnlyA))
	}
	e := res.OnlyA[0]
	if e.PR != 5 || e.Status != StatusPartial || len(e.Commits) != 3 {
		t.Fatalf("entry = %+v", e)
	}
	if !strings.Contains(e.line(), "[partial 1/3]") {
		t.Fatalf("line = %q", e.line())
	}
	// the pick itself is matched, so it is not reported as only on prod
	if len(res.OnlyB) != 0 {
		t.Fatalf("OnlyB = %v", titles(res.OnlyB))
	}
}

func TestMergeCommitPRFullyPickedAsSquash(t *testing.T) {
	r := newRepo(t)
	r.on("stage")
	r.mergePR("6", "alpha", "beta")
	r.on("prod")
	r.write("squashed.txt", "x\n")
	r.git("add", "-A")
	r.git("commit", "-q", "-m", "Alpha and beta (#6)")
	res := r.diff(Options{})
	if len(res.OnlyA) != 0 || len(res.OnlyB) != 0 || len(res.Both) != 1 {
		t.Fatalf("OnlyA=%v OnlyB=%v Both=%v", titles(res.OnlyA), titles(res.OnlyB), titles(res.Both))
	}
}

func TestHotfixOnlyOnProd(t *testing.T) {
	r := newRepo(t)
	r.on("prod")
	r.commit("hotfix: stop the crash")
	res := r.diff(Options{})
	if len(res.OnlyA) != 0 || !reflect.DeepEqual(titles(res.OnlyB), []string{"hotfix: stop the crash"}) {
		t.Fatalf("OnlyA=%v OnlyB=%v", titles(res.OnlyA), titles(res.OnlyB))
	}
}

func TestTicketMatchIsOptIn(t *testing.T) {
	r := newRepo(t)
	r.on("stage")
	r.commit("ABC-77 add widget")
	r.on("prod")
	r.write("w.txt", "reworked\n")
	r.git("add", "-A")
	r.git("commit", "-q", "-m", "ABC-77 add widget, reworked for prod")
	off := r.diff(Options{})
	if len(off.OnlyA) != 1 || len(off.OnlyB) != 1 {
		t.Fatalf("off: OnlyA=%v OnlyB=%v", titles(off.OnlyA), titles(off.OnlyB))
	}
	on := r.diff(Options{MatchTicket: true})
	if len(on.OnlyA) != 0 || len(on.OnlyB) != 0 || on.Both[0].Commits[0].Match.Reason != ReasonTicket {
		t.Fatalf("on: %+v", on)
	}
}

func TestSinceLimitsHistory(t *testing.T) {
	r := newRepo(t)
	r.on("stage")
	t.Setenv("GIT_COMMITTER_DATE", "2020-01-01T00:00:00Z")
	r.commit("old change")
	t.Setenv("GIT_COMMITTER_DATE", "2026-10-01T00:00:00Z")
	r.commit("new change")
	all := r.diff(Options{})
	if len(all.OnlyA) != 2 {
		t.Fatalf("all: OnlyA = %v", titles(all.OnlyA))
	}
	res := r.diff(Options{Since: "2025-01-01"})
	if !reflect.DeepEqual(titles(res.OnlyA), []string{"new change"}) {
		t.Fatalf("since: OnlyA = %v", titles(res.OnlyA))
	}
}

func TestBadRefIsAnError(t *testing.T) {
	r := newRepo(t)
	if _, err := Diff(context.Background(), "stage", "nope", Options{Dir: r.dir}); err == nil {
		t.Fatal("want an error for an unknown ref")
	}
}

func TestOutputFormats(t *testing.T) {
	r := newRepo(t)
	r.on("stage")
	r.squashPR("8", "Add reports", "wip")
	res := r.diff(Options{})
	var buf bytes.Buffer
	if err := res.Write(&buf, FormatText); err != nil || !strings.Contains(buf.String(), "#8 Add reports") {
		t.Fatalf("text: %v %q", err, buf.String())
	}
	buf.Reset()
	if err := res.Write(&buf, FormatMarkdown); err != nil || !strings.Contains(buf.String(), "- #8 Add reports") {
		t.Fatalf("markdown: %v %q", err, buf.String())
	}
	buf.Reset()
	if err := res.Write(&buf, FormatJSON); err != nil {
		t.Fatal(err)
	}
	var back Result
	if err := json.Unmarshal(buf.Bytes(), &back); err != nil || len(back.OnlyA) != 1 || back.OnlyA[0].PR != 8 {
		t.Fatalf("json: %v %+v", err, back)
	}
	if err := res.Write(&buf, "yaml"); err == nil {
		t.Fatal("want an error for an unknown format")
	}
}
