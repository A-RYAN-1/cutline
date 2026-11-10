// Package cutline reports which pull requests are in one branch but not in
// another, matching changes across squash merges, merge commits and
// cherry-picks, and turns the answer into a checked cherry-pick plan.
//
// Teams that ship through a branch per environment (dev, stage, prod) ask the
// same questions at every release: what is in stage that prod lacks, what do I
// pick and in which order, will it conflict, did it all land, and did a hotfix
// on prod ever come back? Git lists SHAs, and "git cherry" matches by patch id,
// which a squash of several commits or a hand-resolved pick changes. cutline
// compares pull requests instead.
//
// # Matching
//
// Every commit carries a chain of fingerprints. Two commits on different
// branches are the same change when one fingerprint matches, tried strongest
// first:
//
//   - the "(cherry picked from commit ...)" trailer written by git cherry-pick -x
//   - the stable patch id (git patch-id --stable)
//   - the pull request number, from "Title (#12)" or "Merge pull request #12
//     from ..."; the commits a merge commit brought in inherit its number
//   - the ticket key such as ABC-123, only when Options.MatchTicket is set,
//     because one ticket can cover several pull requests
//
// Commits are grouped into entries: a pull request with all its commits, or a
// single commit that names none. An entry is StatusMissing, StatusPartial or
// StatusPresent on the other branch.
//
// # Functions
//
//   - [Diff] compares two refs and returns a [Result].
//   - [BuildPlan] orders the cherry-picks for chosen pull requests, warns about
//     older changes on the same files, and with PlanOptions.DryRun applies them
//     in a temporary worktree to find conflicts.
//   - [BuildNotes] writes release notes for the pending changes.
//   - [Verify] checks that B has what A has and nothing A lacks.
//
// Set Options.Enricher (for example a [GitHub] value) to add pull request
// titles, authors, labels and links from the hosting service; without it
// nothing leaves the machine.
//
// All of them run the git command line in Options.Dir, never change a branch or
// the working tree, and need the git command line on PATH.
//
// # What it does not do
//
// It does not open pull requests or run the picks for you (backport tools do
// that), it does not read GitLab merge requests, and its same-file warnings look at
// files, not hunks. See the README for the limits.
package cutline
