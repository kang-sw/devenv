---
title: "MCP mechanical over-split → signature merge (todo insert-trio → add)"
parent: 260903-epic-mcp-tool-surface-affordance-reduction
sage-review-design: completed
sage-review-design-reviewed: 9d10d44f552ffc22
sage-review-completeness: required
---

# MCP mechanical over-split → signature merge (todo insert-trio → add)

## Background

Layer ② of `260903-epic-mcp-tool-surface-affordance-reduction`, and the one
**judgment-heavy** workstream: it designs a novel merged signature and reconciles
output/anchor semantics, unlike the deterministic rename (④) and the clean
read-collapse (③).

`todo.append`, `todo.insert_before`, `todo.insert_after` differ only by position
anchor. Verified in `agents-plugin-tool/internal/mcp/session_state.go`: the two
insert tools already share one handler/core (`handleTodoInsert`/`todoInsert`,
differing only by an `after bool`); `todo.append`/`todoAppend` is separate solely
because it takes no anchor. `key`/`title`/`instruction` semantics are identical
across all three; the anchor is a flat top-level `ref_key` resolved by item key
(not index); all three return the same compact text confirmation
(`"todo appended/inserted: <key>"`). Merge the three into one
`todo.add(position, ref_key?)`. **3 → 1** (family `9 → 7`).

Kept distinct: `check`, `erase`, `clear`, `reorder`, `list`, `read`. The
`read`↔`list` fold is left (they differ in granularity — one item's JSON vs the
list render). The `note.mute`/`unmute` candidate was **dropped** (user-confirmed
2026-09-04): small win, and the pair reads more clearly than a boolean setter.
Scope is the `todo` merge only.

## Decisions — todo.add signature (frozen)

```jsonc
todo.add {
  session_key, key, title,           // same contract as the three tools today
  instruction?,                      // nullable, same contract
  position?: "end" | "before" | "after",   // enum, default "end"
  ref_key?                           // anchor item key; before/after only
}
required: [session_key, key, title]  // position defaults to end; ref_key conditional
```

- **`position` enum + single `ref_key`**, not separate `before_key`/`after_key`.
  `position` has three states one of which (`end`) carries no anchor, so an enum
  is the natural discriminant; a single `ref_key` matches the insert tools'
  existing flat `ref_key` and avoids an "exactly one of two keys" validation.
  Reserve the nested `{before|after}` object style for `todo.reorder`.
- **`position` defaults to `end`** — the common append case stays a drop-in with
  no new required field versus today's `todo.append`.
- **`ref_key` required iff `position ∈ {before, after}`; must be omitted for
  `end`.**
- **Error contract (fail-loud, reusing existing messages):**
  - `position` not in enum → `"todo.add: position must be one of end, before,
    after"`.
  - `position ∈ {before, after}` and `ref_key` missing/empty → `"todo.add:
    ref_key is required when position is <before|after>"`.
  - `position = end` **and** `ref_key` supplied → **reject**: `"todo.add: ref_key
    must be omitted when position is end"` (fail-loud over silently ignoring a
    stray anchor, consistent with `reorder`'s exactly-one posture).
  - bad `ref_key` format / unknown `ref_key` / duplicate `key` / bad
    `instruction` type → reuse `todoInsert`/`normalizeTodoKey`/`todoAppend`
    messages verbatim.
- **Output: one compact confirmation** `"todo added: <normalizedKey>\n"`,
  regardless of position — consistent with `erase`/`clear`/`reorder`. No list
  re-render (that is `todo.check`'s established role).
- **Implementation is a schema/dispatch merge, not new list logic:**
  `handleTodoAdd` branches on `position` → `todoAppend(...)` for `end`, else
  `todoInsert(..., after = position=="after")`.

## Ordering / dependencies

- Runs **last** in the epic's execution order (after ④ rename and ③ collapse),
  since it carries the design cost and benefits from a settled surrounding
  surface.
- Boundary: naming/shape hygiene stays elsewhere; this ticket owns the one place
  the epic changes a call *shape* (not just a name).
- Deprecation posture: one-shot hard cut inherited from the epic — no alias for
  the removed leaf tools (`todo.append`/`todo.insert_before`/`todo.insert_after`).

## Phases

### Phase 1: Merge the insert-trio into todo.add

Add `todo.add` with the frozen signature and error contract above; `handleTodoAdd`
dispatches to the existing `todoAppend`/`todoInsert` cores (no new
list-manipulation logic). Remove `todo.append`, `todo.insert_before`,
`todo.insert_after` from registration + dispatch. Update `runtime.json`,
`mcp-tools.md`, the playbook token call sites that name the old tools
(`agents-plugin/rsrc/lead-write-ticket/`, `lead-forge-spec/`,
`lead-forge-mental-model/`, and the wsflow mirrors via the mirror script), the
`tickets.checklist` description that mentions "a single todo.append instruction",
and tests.

Acceptance: the three old tools are gone in-package (a `references.trace` / grep
sweep is clean); `todo.add` reproduces each old placement with byte-identical
mutation + confirmation output; every error branch fails loud with the specified
message; the full test suite is green.

## Spec Impact

Edits to an existing anchor only — no new spec stem, no heading `{#slug}` change:

- `mcp-tools.md` `{#260625-session-state-tools}` — replace the creation-mutation
  entries (`todo.append`, `todo.insert_before`, `todo.insert_after`) with a
  single `todo.add(position, ref_key?)` entry documenting the enum, the
  conditional `ref_key`, the error contract, and the compact confirmation output.
