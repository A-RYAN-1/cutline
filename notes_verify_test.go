package cutline

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestNotesGroupByConventionalType(t *testing.T) {
	r := newRepo(t)
	r.on("stage")
	r.squashPR("1", "feat(api): add export", "a")
	r.squashPR("2", "fix: stop the crash", "b")
	r.squashPR("3", "feat!: drop v1", "c")
	r.squashPR("4", "Plain title", "d")
	r.squashPR("5", "chore: bump deps", "e")
	r.squashPR("6", "Feat: shout", "f")
	n, err := BuildNotes(context.Background(), "stage", "prod", PlanOptions{Options: Options{Dir: r.dir}})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string][]string{}
	var order []string
	for _, s := range n.Sections {
		order = append(order, s.Name)
		for _, it := range s.Items {
			got[s.Name] = append(got[s.Name], it.line())
		}
	}
	if want := []string{"Features", "Fixes", "Maintenance", "Other"}; !reflect.DeepEqual(order, want) {
		t.Fatalf("sections = %v, want %v", order, want)
	}
	if !strings.HasPrefix(got["Features"][0], "api: add export (#1)") {
		t.Errorf("scope not kept: %v", got["Features"])
	}
	if !strings.Contains(got["Features"][1], "drop v1 (breaking) (#3)") {
		t.Errorf("breaking not marked: %v", got["Features"])
	}
	if !strings.HasPrefix(got["Features"][2], "shout (#6)") {
		t.Errorf("type should be case-insensitive: %v", got["Features"])
	}
	if !strings.HasPrefix(got["Other"][0], "Plain title (#4)") {
		t.Errorf("other = %v", got["Other"])
	}
}

func TestNotesPickAndEmpty(t *testing.T) {
	r := newRepo(t)
	r.on("stage")
	r.squashPR("1", "fix: one", "a")
	r.squashPR("2", "fix: two", "b")
	n, err := BuildNotes(context.Background(), "stage", "prod", PlanOptions{Options: Options{Dir: r.dir}, Picks: []int{2}})
	if err != nil {
		t.Fatal(err)
	}
	if len(n.Sections) != 1 || len(n.Sections[0].Items) != 1 || n.Sections[0].Items[0].PR != 2 {
		t.Fatalf("notes = %+v", n)
	}
	if _, err := BuildNotes(context.Background(), "stage", "prod", PlanOptions{Options: Options{Dir: r.dir}, Picks: []int{9}}); err == nil {
		t.Fatal("want an error for an unknown pull request")
	}
	r.on("prod")
	r.git("merge", "-q", "--ff-only", "stage")
	n, err = BuildNotes(context.Background(), "stage", "prod", PlanOptions{Options: Options{Dir: r.dir}})
	if err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	if err := n.Write(&sb, FormatMarkdown); err != nil || !strings.Contains(sb.String(), "Nothing to release") {
		t.Fatalf("empty notes = %q, %v", sb.String(), err)
	}
	if err := n.Write(&sb, "bogus"); err == nil {
		t.Fatal("want an error for an unknown format")
	}
}

func TestVerify(t *testing.T) {
	r := newRepo(t)
	r.on("stage")
	one := r.squashPR("1", "One", "a")
	two := r.squashPR("2", "Two", "b")
	r.on("prod")
	r.git("cherry-pick", "-x", one)
	hot := r.squashPR("3", "Hotfix", "h")
	ctx := context.Background()

	v, err := Verify(ctx, "stage", "prod", VerifyOptions{Options: Options{Dir: r.dir}})
	if err != nil {
		t.Fatal(err)
	}
	if v.OK || !reflect.DeepEqual(prs(v.Missing), []int{2}) || !reflect.DeepEqual(prs(v.Ahead), []int{3}) {
		t.Fatalf("verify = %+v", v)
	}
	// Only the promoted pull request is expected: the hotfix still fails it.
	v, err = Verify(ctx, "stage", "prod", VerifyOptions{Options: Options{Dir: r.dir}, Expect: []int{1}})
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Missing) != 0 || v.OK {
		t.Fatalf("expect 1: %+v", v)
	}
	if _, err := Verify(ctx, "stage", "prod", VerifyOptions{Options: Options{Dir: r.dir}, Expect: []int{42}}); err == nil {
		t.Fatal("want an error for an unknown expected pull request")
	}
	var sb strings.Builder
	if err := v.Write(&sb, FormatText); err != nil || !strings.Contains(sb.String(), "merge back") {
		t.Fatalf("text = %q, %v", sb.String(), err)
	}

	// Promote the rest and merge the hotfix back: clean.
	r.git("cherry-pick", "-x", two)
	r.on("stage")
	r.git("cherry-pick", "-x", hot)
	v, err = Verify(ctx, "stage", "prod", VerifyOptions{Options: Options{Dir: r.dir}})
	if err != nil {
		t.Fatal(err)
	}
	sb.Reset()
	if err := v.Write(&sb, FormatText); err != nil {
		t.Fatal(err)
	}
	if !v.OK {
		t.Fatalf("want OK, got %s", sb.String())
	}
}
