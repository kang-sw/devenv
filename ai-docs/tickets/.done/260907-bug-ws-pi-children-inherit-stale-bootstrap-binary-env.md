---
title: Pi worker/explore children inherit a stale shell WS_MCP_BOOTSTRAP_BINARY and force-reinstall the wrong ws-mcp
spec:
  - pi-adapter-runtime
related:
  260907-feat-ws-pi-local-devenv-ws-mcp-build-bootstrap: marker bootstrap remains launcher-scoped; this ticket neutralizes stale shell overrides at child boundaries
plans:
  phase-1: 2026-09/09-2217-260907-bug-ws-pi-children-inherit-stale-bootstrap-binary-env
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 95376d2d498939c7
sage-review-completeness-reviewed: 95376d2d498939c7
completed: 2026-09-09
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

## Also observed (historical, 2026-09-07)

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

### Result (649ca5cf) - 2026-09-09

Implemented in `bf811a0b` with launcher-regression follow-up `649ca5cf`.
Direct child environments delete the inherited bootstrap binary/URL overrides;
RPC options explicitly empty them so the SDK's parent-environment merge cannot
restore stale values. The shared boundaries cover fresh worker/fork/persistent
exploration, one-shot collection and dormant resume without mutating the parent
environment or disabling a fork's own marker-driven bootstrap.

Verification:

- Focused spawner and local-devenv suites: **323/323 passed**.
- Real launcher fixture: raw stale overrides force the expected failure;
  sanitized child environment reuses the selected compatible runtime unchanged.
- Isolated live startup: installed Pi **0.85.1** started through its real
  `RpcClient` with the adapter extension and a copied local **0.45.2** runtime.
  The copy independently matched the bundled version, protocol, tools and
  commands. With stale parent binary/URL inputs, the direct builder removed
  both and the effective RPC environment carried empty overrides. `getState()`
  succeeded, the launcher wrote its compatibility stamp, the runtime content
  remained unchanged and no launch-error breadcrumb appeared. All mutable
  Pi/WS state was temporary and removed after clean process shutdown; no model
  prompt, download or owner-cache mutation was used. This verifies real startup
  and bootstrap reuse, not interactive conversation UX or a new public release.
- Full package run: **1276 passed, 130 failed**. The unchanged environmental
  baseline comprises 129 cases/import failures requiring the missing hardcoded
  Linux global Pi SDK fixture and one `lead-bootstrap.test.ts` expectation that
  the host lead exposes `ws-ask`. The latter reproduced standalone with
  `expected ws-ask on the host lead's reshaped surface`; current filtering of
  `ws-ask`/`ws-resolve` is intentional. No build/typecheck script or project
  tsconfig exists. These failures are not reported as a passing full suite.

Review: correctness clean; two Important findings were [fixed] in the single
relay/documentation pass: add the real launcher fixture and document child
bootstrap isolation. No Critical or unresolved implementation findings remain.
Spec `260907-pi-local-devenv-build-bootstrap` was updated in `3507ec9c`; the
spec index passed. No separate mental-model entry was needed because the
invariant is covered by the spec.

Deviations: no implementation scope change. The separate historical release
tool-contract mismatch is not claimed resolved, and the existing full-suite
fixture/tool-surface failures remain outside this ticket.


## Resolution (2026-09-09)

Child bootstrap environment isolation is implemented, independently reviewed, and verified with helper, launcher-fixture and isolated real-Pi startup evidence. Preserve the separately recorded full-suite baseline failures and historical release mismatch as limitations, not unresolved scope of this fix.
