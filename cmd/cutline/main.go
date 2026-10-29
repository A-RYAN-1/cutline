// Command cutline reports which pull requests are in one release branch but not in another.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/A-RYAN-1/cutline"
)

const usage = `usage: cutline diff [flags] A B
       cutline plan [flags] A B

diff lists the pull requests in branch A but not in B, and in B but not in A,
matching across squash merges and cherry-picks.

plan orders the cherry-picks that bring changes from A into B, warns about
older changes in A that touch the same files, and with -dry-run applies them
in a temporary worktree to find conflicts. It never changes your branches.

flags:
`

func main() {
	if len(os.Args) >= 2 {
		switch os.Args[1] {
		case "diff":
			os.Exit(runDiff(os.Args[2:]))
		case "plan":
			os.Exit(runPlan(os.Args[2:]))
		}
	}
	fmt.Fprint(os.Stderr, usage)
	os.Exit(2)
}

func runDiff(args []string) int {
	fs := flag.NewFlagSet("diff", flag.ContinueOnError)
	dir := fs.String("C", "", "repository directory (default: current directory)")
	since := fs.String("since", "", "only commits newer than this date, for example 2026-10-01 or \"2 weeks ago\"")
	format := fs.String("format", "text", "output format: text, markdown or json")
	ticket := fs.Bool("match-ticket", false, "also match by ticket key such as ABC-123 in the subject")
	fs.Usage = func() {
		fmt.Fprint(fs.Output(), usage)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 2 {
		fs.Usage()
		return 2
	}
	res, err := cutline.Diff(context.Background(), fs.Arg(0), fs.Arg(1), cutline.Options{
		Dir: *dir, Since: *since, MatchTicket: *ticket,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "cutline:", err)
		return 1
	}
	if err := res.Write(os.Stdout, cutline.Format(*format)); err != nil {
		fmt.Fprintln(os.Stderr, "cutline:", err)
		return 1
	}
	return 0
}

func runPlan(args []string) int {
	fs := flag.NewFlagSet("plan", flag.ContinueOnError)
	dir := fs.String("C", "", "repository directory (default: current directory)")
	since := fs.String("since", "", "only commits newer than this date, for example 2026-10-01 or \"2 weeks ago\"")
	format := fs.String("format", "text", "output format: text, markdown or json")
	picks := fs.String("pick", "", "comma separated pull request numbers to promote (default: every change missing from B)")
	dry := fs.Bool("dry-run", false, "apply the picks in a temporary worktree and report conflicts")
	fs.Usage = func() {
		fmt.Fprint(fs.Output(), usage)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 2 {
		fs.Usage()
		return 2
	}
	var nums []int
	for _, f := range strings.FieldsFunc(*picks, func(r rune) bool { return r == ',' || r == ' ' }) {
		n, err := strconv.Atoi(strings.TrimPrefix(f, "#"))
		if err != nil || n <= 0 {
			fmt.Fprintf(os.Stderr, "cutline: bad pull request number %q\n", f)
			return 2
		}
		nums = append(nums, n)
	}
	plan, err := cutline.BuildPlan(context.Background(), fs.Arg(0), fs.Arg(1), cutline.PlanOptions{
		Options: cutline.Options{Dir: *dir, Since: *since}, Picks: nums, DryRun: *dry,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "cutline:", err)
		return 1
	}
	if err := plan.Write(os.Stdout, cutline.Format(*format)); err != nil {
		fmt.Fprintln(os.Stderr, "cutline:", err)
		return 1
	}
	return 0
}
