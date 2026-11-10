package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/A-RYAN-1/cutline"
)

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	full := append([]string{"-c", "user.name=T", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false"}, args...)
	cmd := exec.Command("git", full...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func repo(t *testing.T) string {
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "main")
	write := func(name string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("base")
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", "initial")
	git(t, dir, "branch", "prod")
	git(t, dir, "checkout", "-q", "-b", "stage")
	write("one")
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", "feat: add one (#1)")
	return dir
}

func do(args ...string) (code int, out, errs string) {
	var o, e bytes.Buffer
	code = run(args, &o, &e)
	return code, o.String(), e.String()
}

func TestCommands(t *testing.T) {
	dir := repo(t)
	cases := []struct {
		name string
		args []string
		code int
		out  string
	}{
		{"diff", []string{"diff", "-C", dir, "stage", "prod"}, 0, "add one"},
		{"diff json", []string{"diff", "-C", dir, "-format", "json", "stage", "prod"}, 0, `"only_a"`},
		{"plan", []string{"plan", "-C", dir, "-pick", "#1", "-dry-run", "stage", "prod"}, 0, "git cherry-pick -x"},
		{"notes", []string{"notes", "-C", dir, "stage", "prod"}, 0, "## Features"},
		{"verify fails", []string{"verify", "-C", dir, "stage", "prod"}, 1, "NOT OK"},
		{"verify ok", []string{"verify", "-C", dir, "stage", "stage"}, 0, "OK:"},
	}
	for _, c := range cases {
		code, out, errs := do(c.args...)
		if code != c.code || !strings.Contains(out, c.out) {
			t.Errorf("%s: code %d (want %d), out %q, err %q", c.name, code, c.code, out, errs)
		}
	}
}

func TestUsageErrors(t *testing.T) {
	dir := repo(t)
	for name, args := range map[string][]string{
		"no command":      {},
		"unknown command": {"frobnicate"},
		"one branch":      {"diff", "-C", dir, "stage"},
		"bad flag":        {"diff", "-nope", "a", "b"},
		"bad pick":        {"plan", "-C", dir, "-pick", "x", "stage", "prod"},
		"bad expect":      {"verify", "-C", dir, "-expect", "0", "stage", "prod"},
	} {
		if code, _, _ := do(args...); code != 2 {
			t.Errorf("%s: code %d, want 2", name, code)
		}
	}
	for name, args := range map[string][]string{
		"bad ref":      {"diff", "-C", dir, "nope", "prod"},
		"bad format":   {"diff", "-C", dir, "-format", "xml", "stage", "prod"},
		"unknown pick": {"plan", "-C", dir, "-pick", "9", "stage", "prod"},
	} {
		if code, _, errs := do(args...); code != 1 || errs == "" {
			t.Errorf("%s: code %d, stderr %q, want 1 with a message", name, code, errs)
		}
	}
}

func TestEnricherFlag(t *testing.T) {
	c := &common{}
	if e, err := c.enricher(); e != nil || err != nil {
		t.Errorf("empty flag: %v, %v", e, err)
	}
	c = &common{github: "acme/app", githubAPI: "https://ghe.example.test/api/v3"}
	e, err := c.enricher()
	if err != nil {
		t.Fatal(err)
	}
	if g, ok := e.(*cutline.GitHub); !ok || g.Owner != "acme" || g.Repo != "app" || g.BaseURL != "https://ghe.example.test/api/v3" {
		t.Errorf("enricher = %+v", e)
	}
	for _, bad := range []string{"acme", "a/b/c", "/app"} {
		if _, err := (&common{github: bad}).enricher(); err == nil {
			t.Errorf("%q should be rejected", bad)
		}
	}
}

func TestRemoteRepo(t *testing.T) {
	for url, want := range map[string]string{
		"git@github.com:acme/app.git":     "acme/app",
		"https://github.com/acme/app":     "acme/app",
		"https://github.com/acme/app.git": "acme/app",
		"ssh://git@github.com/acme/app/":  "acme/app",
	} {
		m := remoteRepo.FindStringSubmatch(url)
		if m == nil || m[1]+"/"+m[2] != want {
			t.Errorf("%s: %v, want %s", url, m, want)
		}
	}
}
