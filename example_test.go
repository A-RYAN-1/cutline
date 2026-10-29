package cutline_test

import (
	"context"
	"fmt"
	"log"

	"github.com/A-RYAN-1/cutline"
)

// Report what stage has that prod lacks, then plan the picks.
func Example() {
	ctx := context.Background()
	res, err := cutline.Diff(ctx, "stage", "prod", cutline.Options{Dir: "."})
	if err != nil {
		log.Fatal(err)
	}
	for _, e := range res.OnlyA {
		fmt.Printf("#%d %s (%s)\n", e.PR, e.Title, e.Status)
	}
	plan, err := cutline.BuildPlan(ctx, "stage", "prod", cutline.PlanOptions{
		Options: cutline.Options{Dir: "."},
		DryRun:  true,
	})
	if err != nil {
		log.Fatal(err)
	}
	for _, s := range plan.Steps {
		if s.Conflict != nil {
			fmt.Printf("#%d conflicts in %v\n", s.PR, s.Conflict.Files)
		}
	}
}
