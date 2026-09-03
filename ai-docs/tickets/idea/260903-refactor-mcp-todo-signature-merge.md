---
title: "MCP mechanical over-split → signature merge (todo insert-trio → add; note visibility)"
parent: 260903-epic-mcp-tool-surface-affordance-reduction
---

# MCP mechanical over-split → signature merge (todo insert-trio → add; note visibility)

Layer ② of `260903-epic-mcp-tool-surface-affordance-reduction`, and the one
**judgment-heavy** workstream: it designs a novel merged signature and reconciles
output/anchor semantics, unlike the deterministic rename (④) and the clean
read-collapse (③).

## Scope

Fold over-split tool families to the operation callers actually compose.

Primary win — `todo.*` (9 tools):

- `todo.append`, `todo.insert_before`, `todo.insert_after` differ only by
  position anchor → merge into one `todo.add(position: end|before|after,
  ref_key?)`. **3 → 1.**
- Keep the distinct verbs: `check` (set status), `erase`, `clear`, `reorder`,
  `list`, `read`. Net family `9 → 7` (`read`↔`list` fold is possible but the two
  differ in granularity — leave for now).

Candidate — `note.*` (5 tools): `note.mute`/`unmute` are a visibility toggle
pair → `note.set_visible(visible: bool)`. **5 → 4.** Lower value; confirm at
design.

## Why this is the judgment layer

Unlike ③ (survivor already equals the removed tools) and ④ (fixed name map), a
signature merge invents a new parameterization: the `position` discriminant for
`add`, its `ref_key` requirement rules (required for before/after, absent for
end), and the merged tool's output/echo shape must be designed and reconciled,
not derived.

## Ordering / dependencies

- Runs **last** in the epic's execution order (after ④ rename and ③ collapse),
  since it carries the design cost and benefits from a settled surrounding
  surface.
- Boundary: naming/shape hygiene stays elsewhere; this ticket owns the one
  place the epic changes a call *shape* (not just a name).

## Open questions

- Final `todo.add` signature: enum vs separate `before_key`/`after_key`; error
  contract when `ref_key` is missing/invalid; whether `end` needs `ref_key` at
  all.
- Whether `todo.read` folds into `todo.list` (granularity difference argues no).
- `note.set_visible` inclusion — worth the churn, or drop the note candidate?
- Deprecation posture (hard cut vs alias) for the removed leaf tools.

## Spec Impact

Not applicable at `idea/`. Adoption will touch the todo (and possibly note) tool
contract in the MCP tools spec; scope at promotion.
