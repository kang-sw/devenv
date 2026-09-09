---
title: Pi worker/explore children inherit a stale shell WS_MCP_BOOTSTRAP_BINARY and force-reinstall the wrong ws-mcp
spec:
  - pi-adapter-runtime
related:
  260907-feat-ws-pi-local-devenv-ws-mcp-build-bootstrap: the lead-side marker build overrides the variable for the lead's launcher only; children spawn with the unmodified process.env
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 95376d2d498939c7
sage-review-completeness-reviewed: 95376d2d498939c7
---

# Pi worker/explore children inherit a stale shell WS_MCP_BOOTSTRAP_BINARY and force-reinstall the wrong ws-mcp

## Background

### Observation (dogfood, 2026-09-07)

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

## Correction for ready promotion

The owner approved ready promotion on 2026-09-09. Child launch and resume must
neutralize inherited host-shell `WS_MCP_BOOTSTRAP_BINARY` and
`WS_MCP_BOOTSTRAP_URL` before the child launcher can consume them. Workers and exploration children
can reuse the validated runtime install; a fork may still select a fresh binary
through its own valid local-devenv marker. No child force-bootstraps an old
shell-selected binary into the cache slot merely by inheriting these values.

- Cover one-shot exploration, persistent workers/explore, forks and dormant
  resume. Sanitize a copied environment without mutating `process.env`.
- Account for the SDK merge boundary: RPC client options are merged over the
  parent environment. Merely omitting the two keys is insufficient. Override
  them with empty values at that boundary (or an equivalent verified mechanism
  that leaves them ineffective in the actual spawned process); a direct-spawn
  full environment can delete the keys.
- Preserve all other environment values, role/depth/approval/session wiring,
  and the lead's intentional local-devenv bootstrap selection. This change
  removes stale inherited overrides; it does not disable the child's own valid
  local-devenv discovery or the normal verified-cache/release path.
- Do not treat the separate release/tool-contract mismatch described below as
  solved by environment sanitization. No launcher/shared ws-mcp change is
  authored on this Pi track.

## Also observed

`agents-plugin-pi/runtime.json` (byte-synced from develop) lists
`config.resolve_agent`, which develop added after the v0.45.2 tag, so the
published v0.45.2 asset fails the launcher's tools superset check. Until a
release carries that tool, the env-less release path cannot succeed on this
track at all; only the marker build (lead) plus stamp reuse (children)
works. That is a develop release-cadence matter, noted here for context.

## Spec Impact

Update `pi-adapter-runtime` child-launch behavior to state that parent-shell
bootstrap binary/URL overrides cannot force a child to replace the selected
runtime. The normal runtime compatibility checks and lead bootstrap contract
remain unchanged. Planned behavior stays here until implementation.

## Phases

### Phase 1: Neutralize stale bootstrap overrides on every child launch

Apply one consistent policy at the direct-process and RPC-client environment
boundaries, including dormant resume. Keep the fix inside `agents-plugin-pi/`.
No new environment knob or tool-schema change is needed.

Verification: supply both stale parent overrides and an unrelated sentinel;
assert the actual direct-child environment and the effective RPC merged
environment have ineffective bootstrap overrides and retain the sentinel.
Cover fresh worker/fork/persistent-explore, one-shot collection and dormant
resume paths; prove parent process.env and intentional lead bootstrap values
are unchanged. A fixture launcher must reuse the selected compatible install
instead of replacing it with a stale binary. Real Pi dogfood is a post-build
acceptance check; absence of a newer public release is recorded separately.
