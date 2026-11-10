# Contributing

Thanks for looking. cutline answers one question: which pull requests are in one branch but not in another, and what to cherry-pick to close the gap. It reads git and does not open pull requests or run the picks.

1. **Open an issue first** for anything beyond a typo or an obvious bug, so we agree it belongs before you spend time.
2. **Tests are required.** A bug fix starts with a test that fails; a feature comes with tests. Tests build small synthetic repositories with the git command line; never use or paste a private repository or its names. Run `go vet ./... && go test -race ./...` and `gofmt -l .` (it must print nothing).
3. **Keep the module dependency-free** (standard library and the `git` binary only).
4. **Commit style:** one imperative line in the Conventional Commits form (`fix: match a revert of a merge`); the pull request title uses the same form. Body if the reason is not obvious.
5. **Review:** every change goes through a pull request approved by the maintainer; CI (`go vet`, `gofmt`, `go test` on Linux, Windows and macOS, lint) must pass.
6. **Contributor terms:** by submitting a pull request you agree that your contribution is licensed under the MIT License in `LICENSE`, the same as the rest of the project.

Performance changes need a before and after: run `go test -run xxx -bench . -benchtime 3x -count 3 .` on the base commit and on yours and show both.
