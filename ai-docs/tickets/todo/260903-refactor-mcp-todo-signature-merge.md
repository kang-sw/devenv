---
title: "MCP mechanical over-split → signature merge (todo insert-trio → add)"
parent: 260903-epic-mcp-tool-surface-affordance-reduction
sage-review-design: completed
sage-review-design-reviewed: 9d10d44f552ffc22
sage-review-completeness: required
---

# MCP mechanical over-split → signature merge (todo insert-trio → add)

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

Note candidate — **dropped (user-confirmed 2026-09-04).** `note.mute`/`unmute`
(→ `note.set_visible(bool)`, 5 → 4) was considered but dropped: the win is small
and the `mute`/`unmute` pair reads more clearly than a boolean setter, so the
churn is not worth it. This ticket's scope is the `todo` merge only.

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
- Deprecation posture: one-shot hard cut inherited from the epic — no alias for
  the removed leaf tools (`todo.insert_before`/`todo.insert_after`).

## Open questions

- Final `todo.add` signature: enum vs separate `before_key`/`after_key`; error
  contract when `ref_key` is missing/invalid; whether `end` needs `ref_key` at
  all.
- Whether `todo.read` folds into `todo.list` (granularity difference argues no).

## Spec Impact

Not applicable at `idea/`. Adoption will touch the todo tool contract in the MCP
tools spec; scope at promotion.
