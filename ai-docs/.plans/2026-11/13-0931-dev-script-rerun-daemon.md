# Dev script: `dev.bat rerun daemon` replaces the dashboard daemon with a source build

## Problem

During local dogfooding, getting the shared dashboard daemon onto the current
checkout's Go source needs a session start or an MCP reconnect whose launcher
happens to source-build a newer binary. That is indirect and hard to predict.

## Outcome

- A repository-local developer script, `scripts/dev.py`, with one command:
  `rerun daemon`. A root `dev.bat` forwards its arguments to it (`python
  scripts\dev.py %*`), so `dev.bat rerun daemon` works from the repo root.
  Not shipped: nothing under `agents-plugin*/` changes.
- `rerun daemon`:
  1. builds `agents-plugin-tool/cmd/ws-mcp` with `go build` into a scratch
     location;
  2. stages it the way the launcher does before it runs `daemon-start`
     (content-hash-named copy under the dashboard daemon's `bin/` directory;
     reuse the launcher's or the runtime's staging logic or its exact naming
     rule rather than inventing a second scheme), so the running exe is never
     the build output and a later rebuild never hits a locked file;
  3. runs `<staged> daemon-start --build-time <now in Unix ms>` with the
     environment a launcher would pass (cache root, namespace-independent),
     so the existing newer-build-wins rule drains and replaces the running
     daemon; existing session launchers reconnect on their own;
  4. prints the old and new `/health` version/build_time (or the
     daemon-start report line) and exits non-zero on any failure.
- The cache root is resolved the same way the launcher/runtime resolve it
  (`~/.cache/ws@kang-sw-devenv` on this host); no hardcoded user path.
- No kill of any process by image name; replacement goes only through
  `daemon-start`'s drain path (or `/shutdown` with the endpoint token if
  daemon-start alone cannot replace an equal build).

## Out of scope

- Other subcommands (plugin reinstall, tests). Add when needed.
- A POSIX `dev.sh` wrapper (the Python script runs directly there).
- Any change to the launcher or the Go runtime (another delegate is editing
  `ws-mcp-launcher.py` and `internal/mcp` concurrently).

## Verification

- `python scripts/dev.py rerun daemon` on this Windows host: the daemon's
  `/health` build_time changes to the new build's and its pid changes; the
  calling Claude session's wsflow MCP keeps working afterwards (a tool call
  succeeds).
- Running it twice in a row succeeds (no locked-file failure).
