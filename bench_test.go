package cutline

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
)

// bigRepo writes a synthetic history with git fast-import: n squash-merged
// pull requests on stage, the first half of them also on prod as picks with
// the trailer, a quarter more as picks without it.
func bigRepo(b *testing.B, n int) string {
	b.Helper()
	dir := b.TempDir()
	ctx := context.Background()
	if _, err := runGit(ctx, dir, nil, "init", "-q", "-b", "main"); err != nil {
		b.Fatal(err)
	}
	var s bytes.Buffer
	mark := 0
	commit := func(ref, msg string, file int, from int) int {
		mark++
		fmt.Fprintf(&s, "commit refs/heads/%s\nmark :%d\ncommitter T <t@example.com> %d +0000\ndata %d\n%s\n", ref, mark, 1700000000+mark, len(msg), msg)
		if from > 0 {
			fmt.Fprintf(&s, "from :%d\n", from)
		}
		content := "content " + strconv.Itoa(file) + "\n"
		fmt.Fprintf(&s, "M 100644 inline f%d.txt\ndata %d\n%s\n", file, len(content), content)
		return mark
	}
	base := commit("main", "initial", 0, 0)
	stage, prod := base, base
	for i := 1; i <= n; i++ {
		stage = commit("stage", fmt.Sprintf("Feature %d (#%d)", i, i), i, stage)
		switch {
		case i <= n/2:
			msg := fmt.Sprintf("Feature %d (#%d)\n\n(cherry picked from commit %040x)", i, i, i)
			prod = commit("prod", msg, i, prod)
		case i <= n*3/4:
			prod = commit("prod", fmt.Sprintf("Pick %d", i), i, prod)
		}
	}
	// The trailer above names a made-up id; fast-import cannot know real ids,
	// so the picks match through the patch id, which is the stricter test.
	out, err := runGitInput(ctx, dir, s.Bytes(), "fast-import", "--quiet")
	if err != nil {
		b.Fatal(err, out)
	}
	return dir
}

func runGitInput(ctx context.Context, dir string, in []byte, args ...string) (string, error) {
	out, err := runGit(ctx, dir, in, args...)
	return string(out), err
}

func BenchmarkDiffLargeHistory(b *testing.B) {
	for _, n := range []int{1000, 5000} {
		dir := bigRepo(b, n)
		b.Run(strconv.Itoa(n)+"_prs", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				res, err := Diff(context.Background(), "stage", "prod", Options{Dir: dir})
				if err != nil {
					b.Fatal(err)
				}
				if got, want := len(res.OnlyA), n-n*3/4; got != want {
					b.Fatalf("OnlyA = %d, want %d", got, want)
				}
			}
		})
	}
}

// squashedRepo writes a history where every pull request has two commits on
// stage and lands on prod as one squash commit: the case "git cherry" cannot
// match, because the patch id of the squash differs from either commit.
func squashedRepo(tb testing.TB, n int) string {
	tb.Helper()
	dir := tb.TempDir()
	ctx := context.Background()
	if _, err := runGit(ctx, dir, nil, "init", "-q", "-b", "main"); err != nil {
		tb.Fatal(err)
	}
	var s bytes.Buffer
	mark := 0
	commit := func(ref, msg string, files []int, from int) int {
		mark++
		fmt.Fprintf(&s, "commit refs/heads/%s\nmark :%d\ncommitter T <t@example.com> %d +0000\ndata %d\n%s\n", ref, mark, 1700000000+mark, len(msg), msg)
		if from > 0 {
			fmt.Fprintf(&s, "from :%d\n", from)
		}
		for _, f := range files {
			content := "content " + strconv.Itoa(f) + "\n"
			fmt.Fprintf(&s, "M 100644 inline f%d.txt\ndata %d\n%s\n", f, len(content), content)
		}
		return mark
	}
	base := commit("main", "initial", []int{0}, 0)
	stage, prod := base, base
	for i := 1; i <= n; i++ {
		stage = commit("stage", fmt.Sprintf("Feature %d part one (#%d)", i, i), []int{2*i - 1}, stage)
		stage = commit("stage", fmt.Sprintf("Feature %d part two (#%d)", i, i), []int{2 * i}, stage)
		if i <= n/2 {
			prod = commit("prod", fmt.Sprintf("Feature %d (#%d)", i, i), []int{2*i - 1, 2 * i}, prod)
		}
	}
	if out, err := runGitInput(ctx, dir, s.Bytes(), "fast-import", "--quiet"); err != nil {
		tb.Fatal(err, out)
	}
	return dir
}

// gitCherryMissing counts the commits "git cherry prod stage" marks with "+".
func gitCherryMissing(tb testing.TB, dir string) int {
	tb.Helper()
	out, err := runGit(context.Background(), dir, nil, "cherry", "prod", "stage")
	if err != nil {
		tb.Fatal(err)
	}
	n := 0
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "+") {
			n++
		}
	}
	return n
}

// TestAgainstGitCherryOnSquashedPicks records what the baseline gets wrong: git
// cherry reports every commit of a squashed pull request as missing, cutline
// reports only the pull requests that really are.
func TestAgainstGitCherryOnSquashedPicks(t *testing.T) {
	const n = 40
	dir := squashedRepo(t, n)
	if got := gitCherryMissing(t, dir); got != 2*n {
		t.Fatalf("git cherry marks %d commits, want %d (it cannot match squashes)", got, 2*n)
	}
	res, err := Diff(context.Background(), "stage", "prod", Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(res.OnlyA), n-n/2; got != want {
		t.Fatalf("cutline reports %d pull requests missing, want %d", got, want)
	}
	if got := len(res.Both); got != n/2 {
		t.Fatalf("cutline reports %d pull requests present, want %d", got, n/2)
	}
}

// BenchmarkGitCherryBaseline times "git cherry" and cutline on the same
// history of plain single-commit picks, where git cherry is correct.
func BenchmarkGitCherryBaseline(b *testing.B) {
	for _, n := range []int{1000, 5000} {
		dir := bigRepo(b, n)
		want := n - n*3/4
		b.Run(strconv.Itoa(n)+"_git_cherry", func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				if got := gitCherryMissing(b, dir); got != want {
					b.Fatalf("git cherry missing = %d, want %d", got, want)
				}
			}
		})
		b.Run(strconv.Itoa(n)+"_cutline", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				res, err := Diff(context.Background(), "stage", "prod", Options{Dir: dir})
				if err != nil || len(res.OnlyA) != want {
					b.Fatal(err, len(res.OnlyA))
				}
			}
		})
	}
}

// BenchmarkPlanLargeHistory times plan (no dry run) over a large history.
func BenchmarkPlanLargeHistory(b *testing.B) {
	for _, n := range []int{1000, 5000} {
		dir := bigRepo(b, n)
		b.Run(strconv.Itoa(n)+"_prs", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				p, err := BuildPlan(context.Background(), "stage", "prod", PlanOptions{Options: Options{Dir: dir}})
				if err != nil {
					b.Fatal(err)
				}
				if got, want := len(p.Steps), n-n*3/4; got != want {
					b.Fatalf("steps = %d, want %d", got, want)
				}
			}
		})
	}
}
