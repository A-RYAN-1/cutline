package cutline

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
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
