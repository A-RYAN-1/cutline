# cutline

Release-branch promotion report for teams that ship through branches such as `dev`, `stage` and `prod`: which pull requests are in one branch but not the other, with matching that survives squash merges, merge commits and cherry-picks.

Status: private, early. `cutline diff` works on a local clone; plan, notes and verify are not built yet.

## Use

```
go run ./cmd/cutline diff [-C dir] [-since 2026-10-01] [-format text|markdown|json] [-match-ticket] stage prod
```

Output has three parts: pull requests in A but not in B, in B but not in A, and a count of those in both. A pull request that is only partly on the other branch shows `[partial 1/3]`.

## How a change is matched

Git lists SHAs, and `git cherry` matches by patch id, which a squash or a conflict resolution changes. cutline compares the commits on each side of `A...B` and links them by, strongest first:

1. the `(cherry picked from commit ...)` trailer written by `git cherry-pick -x`
2. the stable patch id (`git patch-id --stable`)
3. the pull request number: `Title (#12)` (squash) or `Merge pull request #12 from ...` (merge commit, whose commits inherit the number)
4. the ticket key such as `ABC-123` in the subject, only with `-match-ticket`, because one ticket can cover several pull requests

Commits are reported per pull request, not per SHA.

## Tests

`go test ./...` builds synthetic repositories with the git command line: squash, merge commit, cherry-pick with and without `-x`, a hand-resolved pick, partial picks, a hotfix only on prod, `-since`. Line coverage 87.9% (2026-10-29, `go test -cover .`).

Not covered yet: GitLab-style merge requests, reverts, very large histories (no benchmark yet).
