# FAQ

Comparisons are from each tool's own documentation, read 2026-10-29 (links in `checks/cutline.md` of the author's workspace are the source list). Star counts and dates change; check them yourself.

## Why not `git log prod..stage`?

It lists commits, not pull requests, and after a squash merge or a cherry-pick the same change has a different SHA on each branch, so the list is full of changes that already shipped.

## Why not `git cherry` or `git log --cherry-pick`?

They match by patch id. A patch id is the same only when the diff is the same. A squash of three commits has a different diff from each of them, and a pick whose conflict you resolved by hand has a different diff from the original. cutline uses the patch id as one of four fingerprints and adds the cherry-pick trailer and the pull request number, which survive both.

## How is it different from `nodejs/branch-diff`?

branch-diff lists commits on one branch that are not on another, matching by SHA or by an identical summary plus the `PR-URL` trailer that Node core writes. It is built around Node conventions and goes one way. cutline needs no trailer convention, matches by patch id and pull request number too, reports both directions, and plans the picks.

## How is it different from `sorenlouv/backport` or `korthout/backport-action`?

Those cherry-pick a merged pull request onto branches you name and open the new pull request. They do not tell you what is missing. cutline is the report and the plan; you can feed its plan to one of them. cutline never opens a pull request.

## How is it different from `git-pr-release`?

git-pr-release opens a release pull request listing what was merged into the staging branch, one way, from the GitHub API. cutline reads local git, reports what is in the production branch but not in staging (a hotfix never merged back), and checks the picks for conflicts.

## How is it different from Aviator Releases or Runway?

Those are hosted, paid products with dashboards. cutline is a command and a Go package that runs on your clone.

## Does it need GitHub?

No. It reads `git log` and nothing else. Pull request numbers come from the squash subject `Title (#12)` or the merge subject `Merge pull request #12 from ...`, which GitHub writes by default. Titles, labels and authors from the GitHub API are not read yet.

## Does it work with rebase-and-merge, GitLab or Bitbucket?

Rebase-and-merge lands each commit without a pull request number, so each commit is its own entry, matched by patch id or trailer. GitLab merge requests (`See merge request group/project!12`) are not read yet. Both are listed as limits in the README.

## What happens with a revert?

It is an ordinary change with its own entry. cutline does not pair it with the change it undoes.

## Does `-dry-run` touch my branches?

No. It adds a temporary detached worktree on the target branch, with `core.autocrlf` off, applies the picks there, undoes any that conflict, and removes the worktree. The test suite checks that the refs, the working tree and `git worktree list` are the same afterwards.

## Why warn only about older changes?

A pick can depend on an earlier change to the same file that is not on the target yet. A newer change cannot be a prerequisite. The warning looks at files, not hunks, so it can warn more than needed.
