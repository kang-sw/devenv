---
title: "Re-sync the Pi track's ws-mcp binding, tool contract, and rsrc onto settled epic/refound"
parent: 260911-epic-ws-pi-refound-resync-harness-peer
related:
  260906-bug-ws-pi-rsrc-mirror-drift: the byte-identical mirror plus its identity guard (agents-plugin-pi/test/version-check.test.ts) that this resync must leave green
---

# Re-sync the Pi track's ws-mcp binding, tool contract, and rsrc onto settled epic/refound

## Background

`agents-plugin-pi/.local-devenv-runtime` pins `source_root` to the frozen
`ws-mcp-compat-a937b8dc` worktree (an `epic/refound` mid-point commit) so the Pi
build did not track the churning root worktree while `epic/refound` moved.
`epic/refound` has settled and is bound for `develop`. This ticket re-points the
Pi build at the develop root worktree, brings the Pi tool contract and mirrored
rsrc up to the settled surface, and retires the compat worktree.

## Decisions

- **Marker is path-based, not branch-pinned.** `read_local_devenv_contract`
  (`agents-plugin/bin/ws-mcp-launcher.py`) resolves only `source_root` /
  `tool_dir` / `go` absolute paths and builds the `tool_dir` working tree — no
  ref/branch/sha pin. Re-pointing `source_root` at `/home/swkang/devenv` makes
  the Pi build track whatever the root worktree checks out (develop-bound
  content). The marker is gitignored, worktree-local, and reversible, so the
  re-point may proceed independently of the merge; the tracked-file sync sources
  from `develop` after the merge (epic Cross-Child Decisions 1, 2).
- **runtime.json is a pure 7-tool removal.** refound removed
  `mental_models.list` / `mental_models.query` / `mental_models.status` /
  `references.trace` / `spec_index.verify` / `spec_stem.generate` / `specs.query`
  from `agents-plugin/runtime.json`; no additions or renames.
  `agents-plugin-pi/runtime.json` does not exist upstream — it is Pi-track-local
  and byte-synced from `agents-plugin/runtime.json`, so its update is derivative
  and follows the shared one.
- **rsrc mirror follows 260906.** Absorb refound's shared `agents-plugin/rsrc/`
  into `track/pi-agent`, then re-run the `260906` mirror guard so
  `agents-plugin-pi/rsrc/` matches byte-for-byte. No Pi-specific edits ride the
  resync (that is the overlay child's job, authored upstream).
- **Retire the compat worktree** once the marker points at root and the sync is
  green. Rejected: keeping a frozen compat pin — its only purpose was avoiding
  refound churn, which is over.

## Constraints

- **Pi-track-local authorship (AGENTS.md clause 1).** This ticket edits only
  Pi-track artifacts (the marker, `agents-plugin-pi/runtime.json`, the
  `agents-plugin-pi/rsrc` mirror) and absorbs develop content; it authors no
  shared ws-mcp source.
- **Ready gate:** do not promote to `ready/` until `epic/refound` has merged to
  `develop` (epic Cross-Child Decision 1).
- Removed-tool references in Pi-authored surfaces (adapter guides, bridge tests)
  not fixed by the rsrc absorb must be reconciled by hand; the 7 tools are
  removed, not renamed, so references are deleted or adopt refound's replacement
  pattern (native reads for the retired spec/mental-model discovery).

## Phases

### Phase 1: Re-point the marker and absorb the settled ws-mcp

Re-point `agents-plugin-pi/.local-devenv-runtime` `source_root` / `tool_dir`
from `ws-mcp-compat-a937b8dc` to `/home/swkang/devenv` and confirm the Pi build
starts against the develop-root ws-mcp. Absorb develop (post-refound) shared
`rsrc/` into `track/pi-agent`; byte-sync `agents-plugin/runtime.json` ->
`agents-plugin-pi/runtime.json` (the 7 removed tools); reconcile any
removed-tool references in Pi-authored surfaces. Re-run the `260906` mirror
identity guard and retire the compat worktree.

Verification: a Pi session starts against the develop-root binary and its tool
list reflects the reduced surface (no `specs.query` / `mental_models.*` /
`references.trace` / `spec_*`); `diff -rq agents-plugin/rsrc
agents-plugin-pi/rsrc` is empty; the `agents-plugin-pi` test suite is green
including the mirror identity guard; no Pi-authored file references the 7
removed tools.
