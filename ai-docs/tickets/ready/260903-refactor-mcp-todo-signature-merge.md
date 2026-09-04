---
title: "MCP mechanical over-split → signature merge (todo insert-trio → add)"
parent: 260903-epic-mcp-tool-surface-affordance-reduction
sage-review-design: completed
sage-review-design-reviewed: b20e3dad1923284f
sage-review-completeness: completed
sage-review-completeness-reviewed: b20e3dad1923284f
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
`lead-forge-mental-model/`, and the wsflow mirror — regenerated via the mirror
generator in `agents-plugin-tool/internal/wsrsrc/`, not hand-edited), the
`tickets.checklist` description that mentions "a single todo.append instruction",
and tests. The grep sweep covers spec **prose** too, not just tool entries — the
`{#260625-session-state-tools}` anchor also names the tool in a sentence ("no
skill-side todo.append loop is needed…") that goes stale after the merge.

Acceptance: the three old tools are gone in-package (a `references.trace` /
`grep -r "todo\.(append|insert_before|insert_after)"` sweep across code, specs,
and playbooks is clean); `todo.add` reproduces each old placement with a
byte-identical *mutation*, while the confirmation string is the new unified
`todo added: <key>` (deliberately replacing the old per-position
`todo appended:`/`todo inserted:` strings — tests assert the new string); every
error branch fails loud with the specified message; the full Go test suite is
green (`go test ./...` in `agents-plugin-tool/`).

### Result (645664e1) - 2026-09-04

Collapsed `todo.append`/`todo.insert_before`/`todo.insert_after` into a single
`todo.add(position: end|before|after = "end", ref_key?)`. `handleTodoAdd`
branches on `position` to the existing `todoAppend`/`todoInsert` cores (no new
list logic); the three old tools are removed from registration + dispatch and
the schema collapses from three entries to one. Confirmation unified to
`todo added: <key>`. `ref_key` is required iff `position ∈ {before, after}` and
rejected for `end` — enforced via the comma-ok idiom (`value, ok :=
args["ref_key"]`) so a *supplied-empty* `ref_key` is still rejected for `end`,
distinct from absent. Updated `runtime.json` (both packages), `mcp-tools.md`
`{#260625-session-state-tools}` including the stale "todo.append loop" prose,
the playbook token call sites plus the regenerated wsflow mirror, and the
`tickets.checklist` description.

Verification: `go test ./... -count=1` green (13 packages); the
`todo\.(append|insert_before|insert_after)` grep sweep is clean across code,
specs, and playbooks; runtime.json drift tests green in both the Go and wsflow
suites; spec index ok.

Review (partitioned, correctness opus / test sonnet): correctness clean with 2
Minor recorded and no action — JSON `null` `ref_key` is not special-cased (it is
rejected fail-loud for `end` rather than treated as absent), and a non-string
`position` retains the `"end"` default rather than tripping the enum error; both
are malformed-input-only and outside the frozen contract. Test returned 1
Important — no coverage of supplied-empty `ref_key` for `end` — dispositioned
[fixed] via relay #1: the implementer added four assertions pinning
supplied-empty rejection (explicit `end` + implicit) and absent-acceptance
(explicit `end` + implicit) at commit 98548123.

Deferred: CLI subcommand verb alignment stays out of scope
(`260904-refactor-cli-subcommand-verb-alignment`).

## Spec Impact

Edits to an existing anchor only — no new spec stem, no heading `{#slug}` change:

- `mcp-tools.md` `{#260625-session-state-tools}` — replace the creation-mutation
  entries (`todo.append`, `todo.insert_before`, `todo.insert_after`) with a
  single `todo.add(position, ref_key?)` entry documenting the enum, the
  conditional `ref_key`, the error contract, and the compact confirmation output.
