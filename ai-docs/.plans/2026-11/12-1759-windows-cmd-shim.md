# Plan: hosted harness starts resolve a Windows npm cmd-shim to node

Ticket: `261112-bug-ws-hosted-windows-cmd-shim-host-pid-mismatch` (idea/;
read it first: background with code pointers, settled decisions,
constraints). The user chose to fix it before ws 0.49.

## Outcome

On Windows, when the harness command resolves to an npm cmd-shim (`.cmd` or
`.bat`), wsagent starts node directly so the holder's child PID is node's,
not cmd.exe's:

- One helper beside `lookPath` in `agents-plugin-tool/internal/wsagent`,
  used by every hosted start site: `pi.go` (~337 and ~372) and `claude.go`
  (~315), including a user-set `Command`. Verify the call sites; do not
  guess.
- The helper parses the shim's `"%_prog%"  "%dp0%\<script>" %*` line
  (npm `cmd-shim` format; note the real file has two spaces between the two
  quoted parts and ends with `%*`), and returns argv: `<shim dir>\node.exe`
  when that file exists, else `node` resolved from PATH, then
  `<shim dir>\<script>` (with `%dp0%` expanded to the shim's directory and
  the path normalised), then the original arguments.
- A `.cmd`/`.bat` that does not parse as a cmd-shim refuses the start with a
  clear error naming the file (no silent fallback to running the shim).
- Non-Windows behaviour and any non-shim command (an `.exe`, the native
  `claude.exe`) are unchanged.

## Settled constraints

- Rejected (ticket): matching an ancestor PID in ws-mcp and the Pi
  extension; looking up cmd.exe's child after the spawn.
- Pin the format with a fixture copied verbatim from a real npm-generated
  shim: `/mnt/c/Users/user/AppData/Roaming/npm/pi.cmd` on this machine
  (copy it into a testdata file; it contains no secrets). Keep CRLF line
  endings if the original has them.
- The parse itself must be testable on Linux (pure function over the shim
  text plus a directory and an exists check); only the "is this Windows"
  gate is OS-dependent. Make the gate injectable or split so tests cover
  both the Windows path and the non-Windows no-op.
- Compatibility tier T0: no record, registry, snapshot, holder protocol, or
  MCP shape change.
- Manuals: `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/release-compatibility.md`,
  `ai-docs/manuals/windows-dogfood.md` (for the later host check).
- Heavy `go build`/`go test`/`go vet` only through
  `~/.local/bin/ws-rgo -C agents-plugin-tool -m <min> -- <go args>`. Also
  cross-compile once: `go vet` (or build) with `GOOS=windows` for
  `./internal/wsagent/...` through ws-rgo (env before the go args as ws-rgo
  allows; if it does not pass env, report it).

## Verification

- Tests: the real fixture parses to node.exe-in-dir when it exists, `node`
  from PATH otherwise, with the script path and original args in order;
  CRLF and LF both parse; a `.cmd` that is not a cmd-shim refuses with an
  error naming the file; an `.exe` and a non-Windows host pass through
  unchanged; each start site goes through the helper (at least one test per
  backend, or one test of the shared resolution path both use).
- `go test ./internal/wsagent/...`, `go vet`, and the Windows cross-check
  through ws-rgo; `git diff` reviewed against this plan before commit on
  `impl/develop/windows-cmd-shim`, with `## Ticket Updates` naming the
  ticket. Live Windows-host verification happens after merge, outside this
  delegation.
