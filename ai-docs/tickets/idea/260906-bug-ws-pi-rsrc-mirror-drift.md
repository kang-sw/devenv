---
title: agents-plugin-pi/rsrc/ has drifted from its declared byte-identical source agents-plugin/rsrc/
related:
  260905-feat-ws-pi-harness-config-layer: found during Phase 4 review
---

# agents-plugin-pi/rsrc/ has drifted from its declared byte-identical source agents-plugin/rsrc/

## Background

`agents-plugin-pi/src/index.ts` declares the package-local `rsrc/` copies
(and `runtime.json`, `bin/ws-mcp-launcher.py`) to be hand-synced,
byte-identical copies of the same-named files under `agents-plugin/`. The
harness ticket's Phase 4 review found `agents-plugin-pi/runtime.json` stale
(0.43.4 pin against a 0.44.4 source), which the relay fixed and guarded with
a disk-reading identity test. The re-review then observed that
`agents-plugin-pi/rsrc/` is drifted too: nine playbooks
(`implementer-relay`, `lead-implement`, `lead-proceed`, `lead-review`,
`lead-ship`, `lead-tune`, `lead-workflow-manual`, `plan-populator-research`,
`plan-populator-survey`) plus `rsrc/manifest.json` differ from
`agents-plugin/rsrc/`. Only `runtime.json` is now test-guarded; the rsrc
mirror has no guard, so Pi sessions render playbooks that differ from what
the released ws package ships.

Open questions for triage: whether the drift is stale-copy (resync is the
fix) or intentional Pi overlay text that should live in `.pi.md` overlays
instead of diverging mirrors; and whether the identity test added for
`runtime.json` should be widened to the whole `rsrc/` tree (or replaced by
a sync step in the adapter's build/test) so the declaration cannot silently
break again.

## Phases

### Phase 1: Classify the drift and restore the byte-identical invariant

Diff each drifted file against `agents-plugin/rsrc/`; resync stale copies;
move any intentional Pi-specific text into overlay files per the harness
ticket's Phase 2 overlay selection; extend the identity test (or add a sync
check) to cover `rsrc/` and the launcher copy.
