# Plan: dashboard daemon handoff also orders same-version builds by build time

## Outcome

`wsdashboard.Ensure` (agents-plugin-tool/internal/wsdashboard/ensure.go)
replaces a running daemon only when its version is older. A rebuilt ws-mcp
with the same version string (dev rebuild to dev rebuild, or a reinstall of
the same release) leaves the old daemon serving old page code, and the user
has to kill it by hand. Make a same-version, newer build replace the daemon at
the next ws-mcp serve start.

## Settled constraints

- Order (user-approved, (46)): when versions compare equal, the build with
  the newer build time wins; an older build never replaces a newer one. Do
  not use "different build id -> replace": the Claude plugin runtime and the
  Pi runtime run different ws-mcp binaries of the same version concurrently
  on one machine and would replace each other's daemon on every session
  start. The order must stay total so replacement cannot flap. Version still
  compares first; the build time only breaks a version tie. The existing LAN
  rule for an equal daemon (replace when its LAN mode differs) stays.
- Build time source ((47)): read from the ws-mcp executable itself, not from
  ldflags or the shared launcher scripts. It must identify the binary that was
  built, so check `DaemonExecutable` (it may copy the executable into the
  state dir): a copy's own mtime is the copy time, not the build time. Pick a
  source that survives that path (for example the source executable's mtime
  taken before any copy and carried to the daemon, or preserving mtime on the
  copy), and justify it in the commit.
- The daemon reports its build time in `HealthInfo` (daemon.go) as a new
  JSON field; an older daemon omits it. A daemon without the field and a
  client with one, same version: treat the missing value as older (so a
  first new build replaces a pre-change daemon), and a client without a
  usable build time never replaces on the tie.
- A build time that cannot be read (stat error) is "unknown": unknown never
  wins a tie, so the result stays the current behavior.

## Boundary

Touch only agents-plugin-tool/internal/wsdashboard (ensure.go, daemon.go,
version or a new small helper, their tests) and the ws-mcp dashboard command
wiring in agents-plugin-tool/cmd/ws-mcp/dashboard.go if the build time has to
be passed in. No launcher, install script, or plugin manifest change. Another
worker is concurrently editing wsdashboard page/inbox files
(inbox_routes.go, pages.go, assets/); do not touch those. Return to the lead
if the change needs a launcher or runtime.json change.

## Verification

Tests: Ensure replaces an equal-version daemon with an older build time;
leaves one with a newer or equal build time; leaves when the client's build
time is unknown; replaces a daemon that omits the field when the client has
one; existing version and LAN cases unchanged. Run
`go test ./internal/wsdashboard/ ./cmd/ws-mcp/ -count=1`, `go vet ./...`, and
`go test ./... -count=1` in agents-plugin-tool.
