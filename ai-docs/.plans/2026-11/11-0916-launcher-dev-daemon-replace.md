# launcher-dev-daemon-replace

## Intent

Fix idea ticket `261111-bug-ws-launcher-dev-daemon-never-replaced`. Read the
ticket for the observation and the confirmed cause. In short:

- `DaemonLink.needs_start` compares the running daemon's version (for
  example `0.48.17-dev`, as a local-devenv build reports it) with
  `runtime_version`. That value is `runtime.json` `plugin_version`, for
  example `0.48.17`.
- `compare_daemon_versions` sorts a pre-release suffix after none, so the
  running daemon looks newer than the plugin.
- A reinstall of a new build at the same version therefore never replaces
  the daemon. The build-time and content-hash tie-break is never reached.

The user asked for a simple fix by delegation.

## Scope

- `agents-plugin/bin/ws-mcp-launcher.py`:
  - Make a new build at the same core version reach the existing build-time
    and content-hash tie-break. Do this whether the daemon reports a suffix
    the contract's `plugin_version` lacks, or the reverse.
  - Prefer the smallest change that keeps a genuinely newer core version
    deciding by version, as today.
  - One candidate: in `needs_start` only, treat versions with an equal
    numeric core as equal and fall through to the tie-break.
  - Another candidate: compare against the runtime binary's own version, if
    the launcher already has it cheaply. Do not add a process run of the
    binary on the hot path just for this.
- Keep the Go side consistent. Check how `daemon-start` in
  `agents-plugin-tool` decides whether to stop a running daemon (it may use
  `wsdashboard.CompareVersions` with the binary's own `-dev` version). If the
  Go side would refuse the replacement the launcher now asks for, report it
  instead of changing Go.
- Mirror the launcher change into `agents-plugin-wsflow/bin/ws-mcp-launcher.py`
  per `ai-docs/manuals/wsflow-mirroring.md`.
- Add launcher tests in the existing launcher test files
  (`agents-plugin/bin/test_ws_mcp_launcher_proxy.py` or similar) covering:
  - a `-dev` daemon versus a release contract with a newer, different-hash
    runtime: it must replace;
  - the same version and the same content hash: it must not replace;
  - an older core version: it must replace;
  - a newer core version: it must not replace.
- Move the idea ticket to `.done/` only if the ticket conventions allow
  closing an idea directly. Otherwise leave it, and name the commit in your
  report.

## Verification

Python only, run locally with low priority:
`nice -n 19 python3 -m unittest` (or the runner the existing launcher tests
use) on the launcher test files, for both packages if wsflow has its own.
Do not run go build or go test locally. A Go check, if needed, goes through
`~/.local/bin/ws-rgo -C agents-plugin-tool -m 15 -- test <pkg>`.

## Boundary

Commit on branch `fix/launcher-dev-daemon-replace` in this worktree. Return
to the lead without committing if the fix needs:

- a change to the version format,
- a change to the `runtime.json` contract, or
- a change on the Go side.
