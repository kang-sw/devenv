---
title: Pi worker/explore children inherit a stale shell WS_MCP_BOOTSTRAP_BINARY and force-reinstall the wrong ws-mcp
related:
  260907-feat-ws-pi-local-devenv-ws-mcp-build-bootstrap: the lead-side marker build overrides the variable for the lead's launcher only; children spawn with the unmodified process.env
---

# Pi worker/explore children inherit a stale shell WS_MCP_BOOTSTRAP_BINARY and force-reinstall the wrong ws-mcp

## Observation (dogfood, 2026-09-07)

Pi was started from a shell that still exported
`WS_MCP_BOOTSTRAP_BINARY=~/.cache/ws-mcp-pi-0.44.4` (the old hand-run
dogfood recipe). The lead session started fine: the bridge's marker build
overrides the variable on the launcher child's `spawn` env, so the lead's
launcher installed the fresh 0.45.2 dev build and wrote the compatibility
stamp. At 21:59 the lead called `explore`; the child process inherits
`process.env` unchanged, so its launcher saw the stale variable, took the
forced-bootstrap path (clear stamp, `copy2` the 0.44.4 file into the
`ws-mcp-0.45.2-<hash>` cache slot), failed `incompatible ws-mcp runtime
after repair`, and left the slot holding 0.44.4 content with the Sep 6
mtime. Every later child launch failed the same way, and the lead's own
next restart would have too. The same mechanism explains the identical
stale-slot state found earlier the same day.

## Likely fix

The spawner should not let a host-shell `WS_MCP_BOOTSTRAP_BINARY` /
`WS_MCP_BOOTSTRAP_URL` reach a child's launcher. Options: strip both
variables from the child spawn env unconditionally (children are meant to
reuse the lead's stamped install), or, while the local-devenv marker is
active, forward the lead's own bootstrap value so children converge on the
same binary. Stripping is simpler and matches the lead-only design; a
worker that finds no stamp still takes the ordinary release path.

## Also observed

`agents-plugin-pi/runtime.json` (byte-synced from develop) lists
`config.resolve_agent`, which develop added after the v0.45.2 tag, so the
published v0.45.2 asset fails the launcher's tools superset check. Until a
release carries that tool, the env-less release path cannot succeed on this
track at all; only the marker build (lead) plus stamp reuse (children)
works. That is a develop release-cadence matter, noted here for context.
