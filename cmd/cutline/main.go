// Command cutline reports which pull requests are in one release branch but not in another.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/A-RYAN-1/cutline"
)

const usage = `usage: cutline diff [flags] A B

Lists the pull requests in branch A but not in B, and in B but not in A,
matching across squash merges and cherry-picks.

flags:
`

func main() {
	if len(os.Args) < 2 || os.Args[1] != "diff" {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	os.Exit(runDiff(os.Args[2:]))
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
