---
title: wsflow rsrc mirror regen missed after shipped rsrc edits
sage-review: skipped
completed: 2026-06-29
---

# wsflow rsrc mirror regen missed after shipped rsrc edits

## Problem

During `260625-feat-lead-tune-schema-backed-knob-catalog` implementation,
regenerating `agents-plugin-wsflow/rsrc/` from canonical `agents-plugin/rsrc/`
pulled in several pre-existing canonical rsrc changes outside the touched
`lead-tune` playbook. That means the generated wsflow rsrc mirror had already
drifted before this feature edit.

The current change regenerates the mirror and makes the worktree consistent
again, but the process gap remains: a shipped rsrc edit can update
`agents-plugin/rsrc/` and `agents-plugin/rsrc/manifest.json` while forgetting
the byte-identical wsflow mirror.

## Phases

### Phase 1: Add after-rsrc-edit checklist to wsflow-mirroring.md

Update `ai-docs/ref/wsflow-mirroring.md` to add an explicit "After any rsrc
edit" checklist that names both required generated artifacts in order:
1. `WSRSRC_REGEN=1 go test ./internal/wsrsrc/... -count=1 -run TestGenerateRealManifest`
2. `WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror`

Place the checklist in the "Rsrc Tree Provisioning" section of that doc, or
add a new "After-edit Checklist" subsection immediately before the
"Generated-sameness carve-out" paragraph.

Completion: the checklist is present in the doc; names both artifacts; both
commands include `-count=1`.

## Spec Impact

Documentation-only change to `ai-docs/ref/wsflow-mirroring.md`.
No caller-visible behavior change. Contract-first spec: no.


## Resolution (2026-06-29)

Added after-edit checklist to ai-docs/ref/wsflow-mirroring.md Rsrc Tree Provisioning section naming both required regen steps in order (manifest.json, then wsflow mirror), both with mandatory -count=1 flag explanation.
