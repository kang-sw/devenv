---
title: agents-plugin-pi/rsrc/ has drifted from its declared byte-identical source agents-plugin/rsrc/
related:
  260905-feat-ws-pi-harness-config-layer: found during Phase 4 review; that ticket's runtime.json identity test is the pattern widened here
  260903-research-ws-pi-adapter-npm-distribution: names a pre-publish hand-sync check for the same three copies; whether the test-time guard here also serves publishing is an open packaging question this ticket does not settle
spec:
  - pi-adapter-runtime
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 605d288bde056022
sage-review-completeness-reviewed: 605d288bde056022
---

# agents-plugin-pi/rsrc/ has drifted from its declared byte-identical source agents-plugin/rsrc/

## Background

`agents-plugin-pi/src/index.ts` declares the package-local `rsrc/`,
`runtime.json`, and `bin/ws-mcp-launcher.py` to be hand-synced,
byte-identical copies of the same-named files under `agents-plugin/`
(`{#260903-pi-adapter-package-topology}`). The harness ticket's Phase 4
review found `runtime.json` stale and guarded it with a disk-reading
identity test (`agents-plugin-pi/test/version-check.test.ts`); the
re-review then observed that `rsrc/` is drifted too.

Classification, 2026-09-06 (`diff -rq agents-plugin/rsrc agents-plugin-pi/rsrc`):

- Nine playbooks differ: `implementer-relay`, `lead-implement`,
  `lead-proceed`, `lead-review`, `lead-ship`, `lead-tune`,
  `lead-workflow-manual`, `plan-populator-research`,
  `plan-populator-survey`. Every Pi copy was last touched by the one-time
  bulk copy-in commit `9aea2744` (2026-09-03) and the ws source has moved on
  since (the lead-review range scenario, the lead-ship release gate, the
  lead-implement relay cadence, and so on). No hunk carries Pi-specific text;
  all drift is stale copy.
- `rsrc/manifest.json` differs only in the hash entries of those nine files;
  it is derived, not independently drifted.
- `bin/ws-mcp-launcher.py` is currently byte-identical (the earlier claim
  that it differed does not reproduce); it is unguarded, so it can re-drift
  silently.

The drift is live: the launcher sets `WS_RSRC_ROOT` to the adapter package's
own `rsrc/` and ws-mcp's loader honors that root with no fallback, so every
Pi session renders the stale playbooks. No `.pi.md` overlay exists anywhere
and the loader is called with an empty harness for Pi sessions, so there is
no intentional overlay text to preserve; the harness ticket's Phase 2 only
makes overlays selectable.

## Decisions

- **Resync, do not fork.** Copy the nine playbooks and `manifest.json`
  verbatim from `agents-plugin/rsrc/`. Pi-specific wording, when it is ever
  needed, goes into `.pi.md` overlays through the harness ticket's
  mechanism, never into a diverging mirror.
- **Guard the whole mirror.** Extend the adapter's identity test to walk
  `agents-plugin/rsrc/` recursively and assert every file exists
  byte-identical under `agents-plugin-pi/rsrc/` and that the Pi tree has no
  file the source lacks, and add the same assertion for
  `bin/ws-mcp-launcher.py`. The test reads both trees from disk with Node
  builtins, like the existing `runtime.json` case, so it fails on the next
  desync with the file name in the message. The comparison is a pure
  two-roots comparator so the negative cases run against tmpdir fixtures;
  only the positive case reads the committed trees.
- **Residual exposure.** The guard fires when the adapter suite runs, not at
  the moment an upstream playbook is edited, so a `develop`-side edit can
  land green while the mirror goes stale until the next `agents-plugin-pi`
  test run; the Pi track owner runs that suite when syncing from `develop`.
- **Overlays are mirrored, not authored in the mirror.** Because the guard
  rejects any file the source tree lacks, a future `.pi.md` overlay is
  authored under `agents-plugin/rsrc/` and mirrored like every other file.
- **Rejected: a Go regen test in `agents-plugin-tool/`.** The wsflow package
  guards its mirror from Go, but the adapter must not add obligations to
  the ws-mcp source tree; the guard lives in the adapter's own test suite.
- **Rejected: a build-time copy step.** `rsrc/` is committed and reviewed;
  a generated tree would hide upstream playbook changes from the Pi track's
  review. Left as a later option if hand-sync proves too noisy.

## Spec Impact

`pi-adapter-runtime` `{#260903-pi-adapter-package-topology}`: the three
hand-synced copies are guarded by an identity test over the whole `rsrc/`
tree, `runtime.json`, and the launcher; "no automated sync tooling" is
qualified to "no automated sync, but automated drift detection".

## Constraints

- Adapter-only change in `agents-plugin-pi/`; `agents-plugin/rsrc/` and
  `agents-plugin-tool/` are not edited.
- The resync is a verbatim copy; no content edits ride along.

## Phases

### Phase 1: Resync the mirror and guard it

Copy the nine drifted playbooks and `manifest.json` verbatim from
`agents-plugin/rsrc/`; widen the identity test to the recursive `rsrc/`
tree plus the launcher; amend the spec passage under Spec Impact. Tests: the
widened identity test passes on the resynced tree; a deliberately modified
copy fails naming the file; an extra file under the Pi tree fails.
Verification: `diff -rq` between the two `rsrc/` trees is empty after the
copy; `npm test` in `agents-plugin-pi/` passes. Live check (owner-run): a
new Pi session's `lead-review` playbook shows the range scenario.
