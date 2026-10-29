package cutline

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// repo is a synthetic repository built with the git command line.
type repo struct {
	t   *testing.T
	dir string
	n   int
}

func newRepo(t *testing.T) *repo {
	t.Helper()
	r := &repo{t: t, dir: t.TempDir()}
	r.git("init", "-q", "-b", "main")
	r.write("base.txt", "base\n")
	r.git("add", "-A")
	r.git("commit", "-q", "-m", "initial")
	r.git("branch", "stage")
	r.git("branch", "prod")
	return r
}

func (r *repo) git(args ...string) string {
	r.t.Helper()
	full := append([]string{
		"-c", "user.name=Tester", "-c", "user.email=tester@example.com",
		"-c", "commit.gpgsign=false", "-c", "core.autocrlf=false",
	}, args...)
	out, err := runGit(context.Background(), r.dir, nil, full...)
	if err != nil {
		r.t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}

func (r *repo) write(name, content string) {
	r.t.Helper()
	if err := os.WriteFile(filepath.Join(r.dir, name), []byte(content), 0o644); err != nil {
		r.t.Fatal(err)
	}
}

func (r *repo) head() string { return r.git("rev-parse", "HEAD") }

// commit writes a new file and commits it on the current branch.
func (r *repo) commit(msg string) string {
	r.n++
	r.write("f"+strings.Repeat("x", r.n)+".txt", msg+"\n")
	r.git("add", "-A")
	r.git("commit", "-q", "-m", msg)
	return r.head()
}

// squashPR lands a pull request on the current branch as one squash commit.
func (r *repo) squashPR(pr, title string, commits ...string) string {
	cur := r.git("rev-parse", "--abbrev-ref", "HEAD")
	r.git("checkout", "-q", "-b", "pr"+pr, cur)
	for _, c := range commits {
		r.commit(c)
	}
	r.git("checkout", "-q", cur)
	r.git("merge", "--squash", "pr"+pr)
	r.git("commit", "-q", "-m", title+" (#"+pr+")")
	return r.head()
}

// mergePR lands a pull request as a merge commit and returns the SHAs of its
// commits, oldest first.
func (r *repo) mergePR(pr string, commits ...string) []string {
	cur := r.git("rev-parse", "--abbrev-ref", "HEAD")
	r.git("checkout", "-q", "-b", "pr"+pr, cur)
	var shas []string
	for _, c := range commits {
		shas = append(shas, r.commit(c))
	}
	r.git("checkout", "-q", cur)
	r.git("merge", "-q", "--no-ff", "-m", "Merge pull request #"+pr+" from someone/pr"+pr, "pr"+pr)
	return shas
}

func (r *repo) on(branch string) { r.git("checkout", "-q", branch) }

func (r *repo) diff(opt Options) *Result {
	r.t.Helper()
	opt.Dir = r.dir
	res, err := Diff(context.Background(), "stage", "prod", opt)
	if err != nil {
		r.t.Fatal(err)
	}
	return res
}

func titles(es []Entry) []string {
	out := []string{}
	for _, e := range es {
		out = append(out, e.Title)
	}
	return out
}
