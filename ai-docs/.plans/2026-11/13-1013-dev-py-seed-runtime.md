# dev.py: seed the remote-built runtime into the local-devenv plugin dirs

## Problem

`python scripts/dev.py rerun daemon` builds ws-mcp on w5090 (Windows
cross-build), stages it, and replaces the dashboard daemon. But the plugin
launcher of each local-devenv package (a package dir holding a valid
`.local-devenv-runtime` marker; since 3d1fe8d93 that includes the source
checkout's `agents-plugin-wsflow/` that Claude Code runs directly) keeps its
own runtime under `<plugin_dir>/.runtime/<os>-<arch>/` with a
`.compatibility.json` stamp carrying the local source fingerprint. On the next
session start that stamp misses, so the launcher rebuilds the same source
locally (observed 2026-11-13: after a Claude restart the launcher rebuilt and
replaced the daemon dev.py had just started). Local compute is the scarce
resource on this host; the owner wants builds on w5090.

## Outcome

- After a successful build, `rerun daemon` also seeds every local-devenv
  package dir of this checkout (`agents-plugin/`, `agents-plugin-wsflow/`,
  each only when the launcher's own contract check accepts its marker):
  copy the built exe to the path the launcher would use for that package
  (`runtime_dir` = `WS_MCP_RUNTIME_DIR` or `<plugin_dir>/.runtime/<platform>`,
  name from `runtime_binary_name`), then write the stamp with the launcher's
  own `write_compatibility_stamp(..., source_fingerprint=
  local_devenv_source_fingerprint(plugin_dir, os_name))`. Reuse the launcher
  functions through the module dev.py already imports; no second copy of
  naming, stamp, or fingerprint logic.
- A seed target whose exe is locked (a running launcher's runtime on
  Windows) is replaced the way the launcher replaces its own runtime if it
  has such a path; otherwise report that package as skipped with the reason
  and continue (the daemon replacement still counts as success).
- The fingerprint is taken from the working tree the build was synced from
  (the same checkout), after the build; if the tree changed during the build
  the next launcher start simply rebuilds, which is acceptable.
- Print one line per package: seeded / skipped (reason) / not local-devenv.
- An optional `--no-seed` flag skips seeding.

## Out of scope

- Launcher or Go changes; other dev.py commands.

## Verification

- A test (pytest, alongside the launcher tests or under `scripts/`, matching
  where dev.py tests live if any) with a temp plugin dir + marker: after
  seeding, the launcher's `compatibility_stamp_current(binary, contract,
  contract_path, runtime_dir, fingerprint)` returns True; a dir without a
  marker is not touched.
- Do not run `dev.py rerun daemon` from the worktree (its plugin dirs carry
  no marker, and it would replace the shared daemon); the lead runs the live
  check from the main checkout after merge.
