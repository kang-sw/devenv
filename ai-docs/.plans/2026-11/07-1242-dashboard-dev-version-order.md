# Plan: a dev build outranks the release of the same core

## Outcome

`wsdashboard.CompareVersions` (`agents-plugin-tool/internal/wsdashboard/version.go`)
orders a pre-release suffix below its release core ("0.48.10-dev" < "0.48.10").
In this repository the source `var version` (`cmd/ws-mcp/main.go`) becomes
`"X-dev"` in the same commit that releases X, so a `-dev` build is post-release
develop code. The result: a stale release daemon of the same core is never
replaced by a local dev build (dogfood finding, idea ticket
`261107-bug-dashboard-dev-daemon-shadowed-by-release`).

Flip the equal-core rule: with equal numeric cores, a version carrying a
`-<suffix>` is newer than the bare release.

## Settled constraints

- Keep the order total so replacement cannot flap among builds that share
  this logic. Two suffixed versions with equal cores keep comparing as strings
  (two `-dev` builds stay equal: the dev-to-dev restart stays the documented
  manual `POST /shutdown`; out of scope).
- Unparsable-version handling is unchanged.
- Update the function's doc comment and any caller comment that states the old
  pre-release rule (grep `pre-release`, `-dev` under `internal/wsdashboard`
  and `cmd/ws-mcp`). Note in a comment why the order departs from semver: the
  source version names the release it follows.
- Update tests that pinned the old order; add a case for "0.48.10-dev" vs
  "0.48.10" in both argument orders and an Ensure-level case if an existing
  Ensure test already covers older/newer replacement with versions (reuse its
  pattern; do not build new harness).
- If `ai-docs/manuals/ws-mcp.md` states the pre-release ordering, fix that
  sentence; otherwise leave it.
- Known transition limit, record in the commit `## AI Context`, no code for
  it: already-shipped 0.48.10 binaries keep the old rule, so until the next
  release a release-runtime serve start can still replace a dev daemon.

## Boundary

Touch only the version ordering, its comments/tests, and that manual
sentence. Return to the lead if the change needs anything else (the launcher's
`version_compatible`, the Pi bridge's dev-version refusal, a build id).

## Verification

`go test ./internal/wsdashboard/ ./cmd/ws-mcp/ -count=1`, `go vet ./...`,
then `go test ./... -count=1` in `agents-plugin-tool`.
