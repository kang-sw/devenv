# Plan: git.search grep works on git 2.43

## Problem

`ws/git.search` with `command: grep` fails on git 2.43.0 (Ubuntu 24.04 LTS
default) with `fatal: unable to resolve revision: --end-of-options`:
`git grep` there reads `--end-of-options` as a revision. Shipped in 0.48.11
(592ddd994); the arg layout was only checked on git 2.50
(`agents-plugin-tool/internal/mcp/git_search.go` `gitSearchArgs`, ~:627-655).
Locally, `TestGitSearchCommands`, `TestGitSearchDashPaths`, and
`TestGitSearchNeutralisesAHostileRepoConfig` fail; this blocks the 0.48.12
ship pre-flight. User decision (whiteboard (01) a): fix in this release.

## Change

In `gitSearchArgs`, do not emit `--end-of-options` for `grep`. It is not
needed there: the pattern already goes through `-e <pattern>`, revisions are
validated earlier to not start with `-` (the request parser refuses them),
and paths follow `--`. Keep the other commands' layout unchanged unless the
same failure reproduces for them on git 2.43 (check `log`, `show`,
`ls-files`, `blame`, and any other allowed command with the local git; if one
also fails, fix it the same way and say so). Update the function's doc
comment to state the grep exception and why (git versions before <the version
you can establish, or "2.50, the version checked"> read it as a revision in
grep), as a trap comment at the site.

## Verification

- `cd agents-plugin-tool && go test ./internal/mcp/ -run GitSearch` green on
  the local git 2.43.0, then the whole `go test ./internal/mcp/`.
- The hostile-config and dash-path tests must still prove what they prove
  (a dash-leading path stays a path; a revision starting with `-` is refused).

## Out of scope

The version bump (already committed as 133521651) and the release.
