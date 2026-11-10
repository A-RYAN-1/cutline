// Command cutline reports which pull requests are in one release branch but not in another.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/A-RYAN-1/cutline"
)

const usage = `usage: cutline diff [flags] A B
       cutline plan [flags] A B
       cutline notes [flags] A B
       cutline verify [flags] A B

diff lists the pull requests in branch A but not in B, and in B but not in A,
matching across squash merges and cherry-picks.

plan orders the cherry-picks that bring changes from A into B, warns about
older changes in A that touch the same files, and with -dry-run applies them
in a temporary worktree to find conflicts. It never changes your branches.

notes writes release notes for the changes in A that are not yet in B.

verify checks that B has everything A has and nothing A lacks. It exits 1 when
it does not, so a release pipeline can gate on it.

flags:
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run executes one subcommand and returns the process exit code: 0 success,
// 1 failure (or, for verify, a failed check), 2 usage error.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		rest := args[1:]
		switch args[0] {
		case "diff":
			return runDiff(rest, stdout, stderr)
		case "plan":
			return runPlan(rest, stdout, stderr)
		case "notes":
			return runNotes(rest, stdout, stderr)
		case "verify":
			return runVerify(rest, stdout, stderr)
		}
	}
	_, _ = fmt.Fprint(stderr, usage)
	return 2
}

// common holds the flags every subcommand shares.
type common struct {
	dir, since, format string
	github, githubAPI  string
}

func newFlags(name string, stderr io.Writer, c *common) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&c.dir, "C", "", "repository directory (default: current directory)")
	fs.StringVar(&c.since, "since", "", "only commits newer than this date, for example 2026-10-01 or \"2 weeks ago\"")
	fs.StringVar(&c.format, "format", "text", "output format: text, markdown or json")
	fs.StringVar(&c.github, "github", "", "add titles, authors, labels and links from GitHub: owner/repo, or auto to read the origin remote (token from GITHUB_TOKEN or GH_TOKEN)")
	fs.StringVar(&c.githubAPI, "github-api", "", "GitHub API root for Enterprise Server (default https://api.github.com)")
	fs.Usage = func() {
		_, _ = fmt.Fprint(fs.Output(), usage)
		fs.PrintDefaults()
	}
	return fs
}

// parse reads the flags and the two branch arguments.
func parse(fs *flag.FlagSet, args []string) (a, b string, ok bool) {
	if err := fs.Parse(args); err != nil || fs.NArg() != 2 {
		if err == nil {
			fs.Usage()
		}
		return "", "", false
	}
	return fs.Arg(0), fs.Arg(1), true
}

func parsePicks(s string) ([]int, error) {
	var nums []int
	for _, f := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' }) {
		n, err := strconv.Atoi(strings.TrimPrefix(f, "#"))
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("bad pull request number %q", f)
		}
		nums = append(nums, n)
	}
	return nums, nil
}

type writer interface {
	Write(io.Writer, cutline.Format) error
}

func finish(w writer, format string, stdout, stderr io.Writer, err error) int {
	if err == nil {
		err = w.Write(stdout, cutline.Format(format))
	}
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "cutline:", err)
		return 1
	}
	return 0
}

func runDiff(args []string, stdout, stderr io.Writer) int {
	var c common
	fs := newFlags("diff", stderr, &c)
	ticket := fs.Bool("match-ticket", false, "also match by ticket key such as ABC-123 in the subject")
	a, b, ok := parse(fs, args)
	if !ok {
		return 2
	}
	enr, err := c.enricher()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "cutline:", err)
		return 2
	}
	res, err := cutline.Diff(context.Background(), a, b, cutline.Options{Dir: c.dir, Since: c.since, MatchTicket: *ticket, Enricher: enr})
	if err != nil {
		return finish(nil, c.format, stdout, stderr, err)
	}
	return finish(res, c.format, stdout, stderr, nil)
}

func runPlan(args []string, stdout, stderr io.Writer) int {
	var c common
	fs := newFlags("plan", stderr, &c)
	picks := fs.String("pick", "", "comma separated pull request numbers to promote (default: every change missing from B)")
	dry := fs.Bool("dry-run", false, "apply the picks in a temporary worktree and report conflicts")
	a, b, ok := parse(fs, args)
	if !ok {
		return 2
	}
	enr, err := c.enricher()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "cutline:", err)
		return 2
	}
	nums, err := parsePicks(*picks)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "cutline:", err)
		return 2
	}
	plan, err := cutline.BuildPlan(context.Background(), a, b, cutline.PlanOptions{
		Options: cutline.Options{Dir: c.dir, Since: c.since, Enricher: enr}, Picks: nums, DryRun: *dry,
	})
	if err != nil {
		return finish(nil, c.format, stdout, stderr, err)
	}
	return finish(plan, c.format, stdout, stderr, nil)
}

func runNotes(args []string, stdout, stderr io.Writer) int {
	var c common
	fs := newFlags("notes", stderr, &c)
	picks := fs.String("pick", "", "comma separated pull request numbers to include (default: every change missing from B)")
	a, b, ok := parse(fs, args)
	if !ok {
		return 2
	}
	enr, err := c.enricher()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "cutline:", err)
		return 2
	}
	nums, err := parsePicks(*picks)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "cutline:", err)
		return 2
	}
	notes, err := cutline.BuildNotes(context.Background(), a, b, cutline.PlanOptions{
		Options: cutline.Options{Dir: c.dir, Since: c.since, Enricher: enr}, Picks: nums,
	})
	if err != nil {
		return finish(nil, c.format, stdout, stderr, err)
	}
	return finish(notes, c.format, stdout, stderr, nil)
}

func runVerify(args []string, stdout, stderr io.Writer) int {
	var c common
	fs := newFlags("verify", stderr, &c)
	expect := fs.String("expect", "", "comma separated pull request numbers that must be in B (default: everything in A)")
	a, b, ok := parse(fs, args)
	if !ok {
		return 2
	}
	enr, err := c.enricher()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "cutline:", err)
		return 2
	}
	nums, err := parsePicks(*expect)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "cutline:", err)
		return 2
	}
	v, err := cutline.Verify(context.Background(), a, b, cutline.VerifyOptions{
		Options: cutline.Options{Dir: c.dir, Since: c.since, Enricher: enr}, Expect: nums,
	})
	if err != nil {
		return finish(nil, c.format, stdout, stderr, err)
	}
	if code := finish(v, c.format, stdout, stderr, nil); code != 0 {
		return code
	}
	if !v.OK {
		return 1
	}
	return 0
}
