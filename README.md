# cutline

Which pull requests are in `stage` but not in `prod`, and the other way round? cutline answers from your local git history, matching changes across squash merges, merge commits and cherry-picks, then turns the answer into a checked cherry-pick plan, release notes and a post-release gate.

Status: pre-release (v0). Licensed under MIT.

## Try it

```
go install github.com/A-RYAN-1/cutline/cmd/cutline@latest
cutline diff stage prod
```

On a demo repository with four pull requests landed on `stage` as squash commits, one of them already cherry-picked to `prod`, and a hotfix made on `prod` only (real output, 2026-10-29):

```
$ cutline diff stage prod
In stage but not in prod: 3
  #104 docs: explain the retry policy (6b46f70, Dev, 2026-10-29)
  #103 feat(billing)!: drop the v1 invoice format (5a1f14f, Dev, 2026-10-29)
  #101 feat(search): add fuzzy matching (cae6db0, Dev, 2026-10-29)
In prod but not in stage: 1
  #105 fix: rotate leaked key (26daa44, Dev, 2026-10-29)
In both (matched): 1
```

`git log prod..stage` would list all four stage commits, including the one that already shipped under a different SHA.

## Why it exists

Teams that ship through a branch per environment ask the same questions at every release: what is in stage that prod lacks, what do I pick and in which order, will it conflict, did it all land, and did a hotfix on prod ever come back? Git answers with SHAs, which stop meaning anything after a squash or a cherry-pick, and the tools that do answer are hosted products (Aviator Releases, Runway), a Ruby gem that goes one way (`git-pr-release`), or conventions of one project (`nodejs/branch-diff`, `electron/unreleased`). The comparison with sources is in [docs/FAQ.md](docs/FAQ.md).

## Commands

| Command | Use it to | Exit code |
|---|---|---|
| `cutline diff A B` | list pull requests in A but not B, in B but not A, and count those in both | 0 |
| `cutline plan A B` | get the `git cherry-pick -x` commands, oldest first, with same-file warnings; `-pick 103,104` chooses pull requests, `-dry-run` finds conflicts in a temporary worktree | 0 |
| `cutline notes A B` | write release notes for what is pending, grouped by conventional commit type | 0 |
| `cutline verify A B` | gate a release: B must have everything A has and nothing A lacks; `-expect 103,104` narrows it | 1 when not OK |

Shared flags: `-C dir`, `-since 2026-10-01` (or `"2 weeks ago"`), `-format text|markdown|json`. `diff` also takes `-match-ticket`.

Optional GitHub data: `-github owner/repo` (or `-github auto` to read the origin remote) adds the real pull request title, author, labels and link to every command, and finds the pull request of a single commit that has no number in its subject. `notes` then groups by label (`bug`, `enhancement`, `documentation`, ...) when a title is not a conventional commit. Without the flag nothing leaves your machine.

```
$ cutline plan -pick 103,104 -dry-run stage prod
# Promote 2 change(s) from stage into prod, oldest first
# Dry run: every step applies cleanly

# #103 feat(billing)!: drop the v1 invoice format
git cherry-pick -x 5a1f14f3112f0fb31ae63d477985994724c40743

# #104 docs: explain the retry policy
git cherry-pick -x 6b46f7074bc42242d9c9aa98ee33f6acdb785911

$ cutline notes stage prod
## Features

- search: add fuzzy matching (#101) by Dev
- billing: drop the v1 invoice format (breaking) (#103) by Dev

## Documentation

- explain the retry policy (#104) by Dev

$ cutline verify stage prod
NOT OK
Not in prod yet (3):
  #104 docs: explain the retry policy (6b46f70, Dev, 2026-10-29)
  #103 feat(billing)!: drop the v1 invoice format (5a1f14f, Dev, 2026-10-29)
  #101 feat(search): add fuzzy matching (cae6db0, Dev, 2026-10-29)
In prod but not in stage (1), merge back:
  #105 fix: rotate leaked key (26daa44, Dev, 2026-10-29)
```

The plan is a script you read and run yourself. cutline never changes a branch, opens a pull request or runs a pick outside the temporary worktree.

## How a change is matched

Git lists SHAs, and `git cherry` matches by patch id, which a squash or a hand-resolved pick changes. cutline compares the commits on each side and links them by, strongest first:

1. the `(cherry picked from commit ...)` trailer written by `git cherry-pick -x`
2. the stable patch id (`git patch-id --stable`)
3. the pull request number: `Title (#12)` (squash) or `Merge pull request #12 from ...` (merge commit, whose commits inherit the number)
4. the ticket key such as `ABC-123` in the subject, only with `-match-ticket`, because one ticket can cover several pull requests

Commits are reported per pull request. One that is only partly on the other branch shows as `[partial 1/3]`, and a plan lists only the commits still missing.

## Architecture

One Go package with no dependencies beyond the standard library and the `git` command line; `cmd/cutline` is a thin command on top.

| File | Job |
|---|---|
| `git.go` | runs git: `log` records, stable patch ids, merge members |
| `fingerprint.go` | parsers for the pull request number, the pick trailer and the ticket key |
| `cutline.go` | groups commits into pull request entries and matches the two sides |
| `plan.go`, `plan_format.go` | pick order, same-file warnings, the worktree dry run |
| `notes.go`, `verify.go` | release notes, the release gate |

## Use as a Go package

```go
res, err := cutline.Diff(ctx, "stage", "prod", cutline.Options{Dir: repoDir})
plan, err := cutline.BuildPlan(ctx, "stage", "prod", cutline.PlanOptions{Options: cutline.Options{Dir: repoDir}, DryRun: true})
```

`BuildNotes` and `Verify` take the same shape. The package guide is `go doc github.com/A-RYAN-1/cutline`.

## Engineering quality

| Check | Result | Reproduce |
|---|---|---|
| Matcher against a reference model on random histories (squash and merge-commit pull requests; trailer, plain, partial and hand-edited picks; re-created squashes; hotfixes), both directions | 60 seeds, all pass (2026-10-29) | `CUTLINE_SEEDS=60 go test -count=1 -run TestMatcherAgainstModel .` |
| Seven deliberate matcher bugs, one at a time | each makes the test fail | see `libraries/libs/cutline.md` in the author's workspace; the edits are listed in the commit message of PR 2 |
| Parser fuzzing | 878,480 executions in 30 s, no failure (2026-10-29) | `go test -run xxx -fuzz FuzzParsers -fuzztime 30s .` |
| Data race run | in CI on Linux | `go test -race ./...` |
| `diff` speed, i5-13450HX, Go 1.27.0, Windows, git subprocess time dominates | 1000 pull requests 238 to 254 ms; 5000 pull requests 1.58 to 2.41 s (2026-10-29) | `go test -run xxx -bench DiffLargeHistory -benchtime 3x -count 3 .` |
| Line coverage | see the CI run (`go test -cover ./...`) | `go test -cover ./...` |

The benchmark history is synthetic and every pick in it matches by patch id; there is no comparison with `git cherry` yet.

## Run locally

```
git clone git@github.com:A-RYAN-1/cutline.git   # private for now
cd cutline
go build -o cutline ./cmd/cutline
go test ./...
```

Needs Go 1.23 or newer and `git` on PATH. CI runs on Linux, Windows and macOS.

## Environment variables

`GITHUB_TOKEN` or `GH_TOKEN`, read only when `-github` is used: it raises the API rate limit and allows private repositories. Without `-github` cutline reads no environment variables and makes no network calls.

## Deployment

Not deployed yet: it is a command and a library, not a service. Releases are built with GoReleaser from a `v*` tag on `main` (binaries for Linux, macOS and Windows, checksums, notes from the CHANGELOG); no tag exists yet.

## Limits

Out of scope for now:

- GitLab merge requests (`See merge request group/project!12`) are not read. Only GitHub's subjects (`Title (#12)`, `Merge pull request #12 from ...`) are understood; a request for GitLab or Bitbucket is welcome as an issue.

Known, by design:

- Same-file warnings do not look at hunks, so a warning can name a change that touches other lines of the same file. It is a prompt to look, never a block.
- `plan` prints the `git cherry-pick` commands and, with `-dry-run`, checks them in a temporary worktree. It never runs the picks on a real branch and never opens a pull request.
- A hand-resolved pick that lost both the trailer and the number shows as missing.
- Rebase-and-merge lands commits without a number unless the subjects carry `(#12)`; commits without one are listed one by one, each with a note saying so.
- A commit of a rebase-merged pull request that both branches already have cannot be told apart from an unrelated commit, so only merge-style pull requests list such shared commits.
- A revert is paired with what it undoes only when both are on the same side of the comparison; otherwise it is a change of its own.

## Related tools

See [docs/FAQ.md](docs/FAQ.md) for how this differs from `git cherry`, `nodejs/branch-diff`, `sorenlouv/backport`, `git-pr-release`, and the hosted Aviator Releases and Runway.

## Status and roadmap

Built: `diff`, `plan`, `notes`, `verify`, optional GitHub enrichment, the proof group, docs and tooling. Not built: GitLab merge requests.

## Credits

Written by Aryan Deshmukh (A-RYAN-1).

Last updated: 2026-11-10

## Contributing and license

Released under the [MIT License](LICENSE). Contributions are welcome, see [CONTRIBUTING.md](CONTRIBUTING.md). Changes are listed in [CHANGELOG.md](CHANGELOG.md).
