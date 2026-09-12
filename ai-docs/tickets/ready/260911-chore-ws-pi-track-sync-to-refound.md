---
title: "Re-sync the Pi track's ws-mcp binding, tool contract, and rsrc onto settled epic/refound"
parent: 260911-epic-ws-pi-refound-resync-harness-peer
related:
  260906-bug-ws-pi-rsrc-mirror-drift: the byte-identical mirror plus its identity guard (agents-plugin-pi/test/version-check.test.ts) that this resync must leave green
spec:
  - 260903-pi-bridge-version-pin
  - 260907-pi-local-devenv-build-bootstrap
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 7bcf986f8285d001
sage-review-completeness-reviewed: 7bcf986f8285d001
---

# Re-sync the Pi track's ws-mcp binding, tool contract, and rsrc onto settled epic/refound

## Background

`agents-plugin-pi/.local-devenv-runtime` originally pinned `source_root` to the
frozen `ws-mcp-compat-a937b8dc` worktree (an `epic/refound` mid-point commit) so
the Pi build did not track the churning root worktree while `epic/refound`
moved. During later question-queue dogfood, the worktree-local marker was
temporarily repointed at this Pi-track worktree. `epic/refound` has now landed
on `develop` as merge commit `77409c56`. This ticket re-points the Pi build at
the actual develop root worktree, brings the Pi tool contract and mirrored rsrc
up to the settled surface, and verifies the now-absent compat worktree remains
retired.

## Decisions

- **Marker is path-based, not branch-pinned.** `read_local_devenv_contract`
  (`agents-plugin/bin/ws-mcp-launcher.py`) resolves only `source_root` /
  `tool_dir` / `go` absolute paths and builds the `tool_dir` working tree — no
  ref/branch/sha pin. Re-pointing `source_root` at the develop worktree reported
  by `git worktree list` makes the Pi build track whatever that worktree checks
  out. On the current machine that path is `/Users/kang-sw/devenv`; it belongs
  only in the gitignored marker and is not a portable tracked default. The marker
  is worktree-local and reversible; the tracked-file sync sources from `develop`
  after merge `77409c56` (epic Cross-Child Decisions 1, 2).
- **runtime.json follows the complete refound contract.** Refound removed
  `mental_models.list` / `mental_models.query` / `mental_models.status` /
  `references.trace` / `spec_index.verify` / `spec_stem.generate` / `specs.query`,
  added `git.merge`, and removed 23 command entries from
  `agents-plugin/runtime.json`. `agents-plugin-pi/runtime.json` does not exist
  upstream — it is Pi-track-local and byte-synced from the complete
  `agents-plugin/runtime.json`, so its update is derivative rather than a
  hand-picked tool-only delta.
- **rsrc mirror follows 260906.** Absorb refound's shared `agents-plugin/rsrc/`
  into `track/pi-agent`, then re-run the `260906` mirror guard so
  `agents-plugin-pi/rsrc/` matches byte-for-byte. No Pi-specific edits ride the
  resync (that is the overlay child's job, authored upstream).
- **Keep the compat worktree retired.** It is already absent from
  `git worktree list`; confirm it remains absent after the marker points at the
  develop root and the sync is green. Rejected: restoring a frozen compat pin —
  its only purpose was avoiding refound churn, which is over.

## Constraints

- **Pi-track-local authorship (AGENTS.md clause 1).** This ticket edits only
  Pi-track artifacts (the marker, `agents-plugin-pi/runtime.json`, the
  `agents-plugin-pi/rsrc` mirror) and absorbs develop content; it authors no
  shared ws-mcp source.
- **Ready gate satisfied:** `epic/refound` merged to `develop` as `77409c56`
  (epic Cross-Child Decision 1); `/Users/kang-sw/devenv` currently resolves to
  `develop` at `90a9ef2e`.
- Runtime-contract references in Pi-authored surfaces (adapter guides, bridge
  tests) not fixed by the rsrc absorb must be reconciled by hand. References to
  the seven removed tools are deleted or adopt refound's replacement pattern
  (native reads for retired spec/mental-model discovery); `git.merge` and the
  reduced command inventory follow the shared runtime contract without a Pi-specific
  reinterpretation.

## Phases

### Phase 1: Re-point the marker and absorb the settled ws-mcp

Re-point `agents-plugin-pi/.local-devenv-runtime` `source_root` / `tool_dir`
from the temporary Pi-track dogfood target to the develop worktree resolved by
`git worktree list` (currently `/Users/kang-sw/devenv`) and confirm the Pi build
starts against the develop-root ws-mcp. Absorb develop (post-refound) shared
`rsrc/` into `track/pi-agent`; byte-sync `agents-plugin/runtime.json` ->
`agents-plugin-pi/runtime.json` as the complete refound contract; reconcile
runtime-contract references in Pi-authored surfaces. Re-run the `260906` mirror
identity guard and confirm the compat worktree remains absent.

Verification: a Pi session starts against the develop-root binary and its tool
list reflects the refound surface (`git.merge` present; no `specs.query` /
`mental_models.*` / `references.trace` / `spec_*`); the 23 removed commands are
absent; `diff -rq agents-plugin/rsrc agents-plugin-pi/rsrc` is empty; the
`agents-plugin-pi` test suite is green including the mirror identity guard; no
Pi-authored file retains stale references to the removed runtime entries.
