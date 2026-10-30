# Security policy

## Supported versions

Only the latest released version receives fixes. There is no release yet.

## Reporting a vulnerability

Please do not open a public issue. Use GitHub's private vulnerability reporting (Security tab, "Report a vulnerability") on this repository. Expect an acknowledgement within 7 days.

cutline runs the `git` command on a local repository and builds a plan of `git cherry-pick` commands. The likely reports are argument or path injection through branch names, commit subjects or file names (a value reaching `git` or a shell unescaped), and the dry run (`plan -dry-run`) touching a branch or the working tree. Both are in scope.
