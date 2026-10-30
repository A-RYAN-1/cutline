# Changelog

All notable changes are listed here, newest first. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project follows [Semantic Versioning](https://semver.org/); until `v1.0.0` a minor version may change the API.

## [Unreleased]

### Added

- `cutline diff A B`: the pull requests in A but not in B and in B but not in A, matched across squash merges, merge commits and cherry-picks by the `cherry picked from` trailer, the stable patch id, the pull request number and, with `-match-ticket`, the ticket key. Text, markdown and JSON output; `-since` limits the history.
- `cutline plan A B`: the `git cherry-pick -x` commands that promote chosen pull requests (`-pick`) or every missing one, oldest first, leaving out commits already on B. Warns when an older change that is not in the plan touches the same files. `-dry-run` applies the picks in a temporary worktree and marks the steps that conflict, without touching branches or the working tree.
- `cutline notes A B`: release notes for the pending changes, grouped by conventional commit type, with breaking changes marked.
- `cutline verify A B`: exits 1 when an expected change is missing or only partly in B, or when B has a change A lacks.
- Reverts: a change that a `Revert "..."` commit (or `Reapply`) undid on the same branch is reported as reverted, not missing; a revert pair cancels out, a revert of a revert keeps the change, and a revert of one commit of a merge-style pull request drops only that commit. A revert whose target is outside the compared range stays a change of its own.
- Rebase-merged pull requests: commits whose subjects carry the same `(#n)` suffix are one entry; commits with no pull request number are listed one by one with a reason (`note` in JSON, `[single commit, no PR number]` in text and markdown). Rebased copies on the other branch match by patch id.
- Shared commits: a commit of a merge-style pull request that both branches already have is listed in the entry (match reason `already on both branches`), so the entry shows its full commit list and reads as partial; `plan` does not pick it again.
- Go API: `Diff`, `BuildPlan`, `BuildNotes`, `Verify` and the parsers `PRNumber`, `IsPRMerge`, `PickedFrom` and `TicketKey`.
- Proof: a model-based property test over random histories (checked against seven mutations of the matcher), `FuzzParsers`, a race run in CI, and a benchmark over 1000 and 5000 pull requests.
- Releases with GoReleaser (binaries for Linux, macOS and Windows, checksums); the release notes are the matching section of this file, and a tag must be on `main`.
