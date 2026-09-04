---
title: "MCP read-surface verb-vocabulary unification (deterministic scripted rename)"
parent: 260903-epic-mcp-tool-surface-affordance-reduction
related:
  260903-refactor-mcp-read-surface-collapse: sibling layer ③ — collapse depends on this layer's `query` survivor naming; for tickets/specs rename and collapse coincide
sage-review-design: completed
sage-review-completeness: required
sage-review-design-reviewed: ae7feeadf1d08c7b
---

# MCP read-surface verb-vocabulary unification (deterministic scripted rename)

Layer ④ of `260903-epic-mcp-tool-surface-affordance-reduction`. See the epic for
the full canonical verb table, cost model, and cross-child invariants; this
ticket carries only ④'s specifics.

## Scope

Rename the read/query surface to a small canonical verb set so a single
"read/query" intent stops being spread across `list`/`status`/`read`/`find`/
`search`/`info`/`print`, and so `status` stops naming both a corpus lookup and a
live-state read.

Canonical verbs (from the epic):

- `query` — search / resolve / enumerate a searchable corpus.
- `read` — fetch one item's body by id/key.
- `list` — enumerate a short bounded set (kept; NOT every list becomes query).
- `status` — live runtime / vcs / process state only.

Illustrative rename map (finalize here): `tickets.find→tickets.query`,
`specs.find→specs.query`, `note.search→note.query`,
`mental_models.find→mental_models.query`, `runtime.info→runtime.status`,
candidate `playbook.print→playbook.read`.

## Method — deterministic, not hand-edited

Once the old→new name map is frozen, apply it by **script** across: Go tool
registration + dispatch switch, `runtime.json`, MCP/workflow specs, playbook
tokens, and tests. Verify by diff + test run. Only a thin prose-cleanup tail
(surrounding sentences like "find the ticket…") needs a human pass. Hand-editing
name-by-name is out of method.

## Deprecation posture

One-shot hard cut, inherited from the epic. Old names have no persisted
cross-version consumer — Go dispatch, `runtime.json`, specs, playbook tokens,
workflow manual, and the wsflow mirror all ship in-package and are swept by this
ticket's own script — so the rename is atomic, not a compat break: no
alias/transition window. The script removes every old-name reference in the same
pass.

## Ordering / dependencies

- Runs **first** in the epic's execution order (locks canonical names for ③/②).
- Collision with ① (`enter.*` affordance rewrite): ④'s script edits *tokens* in
  `lead-proceed`/`lead-implement`; ① owns their *authoring* pass. Default order
  ④-script → ①-authoring so those two skills are not authored twice.

## Open questions

- Final verb map, including the `runtime.info→status` and `playbook.print→read`
  candidates (are they read/live-state or something else?).
- Whether the gated `exec.*`/`mercenary.*` families are swept in the same script
  pass (they adopt the vocabulary but are not the epic's landing gate).

## Spec Impact

Not applicable at `idea/`. Adoption will touch the MCP tools spec
(`mcp-tools.md`) tool-name contract; scope at promotion.
