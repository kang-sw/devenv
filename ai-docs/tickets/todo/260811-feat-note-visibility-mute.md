---
title: Note visibility — mute a note out of injection while keeping it searchable
parent: 260807-epic-mechanical-project-memory
related:
  260807-feat-note-memory-layers: extends — adds a `visible` attribute and mute/unmute verbs to the shipped note.* surface and its workflow_manual injection
  260810-feat-repo-tracked-note-layer: coordinates — visibility is a layer-agnostic note attribute; the tracked repo layer inherits it, and whichever of the two lands second extends the shared record shape
sage-review-design: completed
---

# Note visibility — mute a note out of injection while keeping it searchable

## Background

The shipped note-memory surface (`260807-feat-note-memory-layers`) force-injects
every note into `workflow_manual`'s `# Notes` block, bounded only by a
priority-ordered cap: the highest-`priority` records fill `notesInjectionCap`
slots and the remainder is elided behind a visible "N lower-priority notes
elided" line, retrievable via `note.search`. Priority is the *only* lever over
what an agent sees every session.

Priority is the wrong lever for one common intent: a note that should stay on
record but should *not* be injected right now — a resolved reminder, a
context-specific note that is noise outside its worktree phase, a machine record
kept for reference but rarely relevant. Lowering its priority to bury it below
the cap both fights the priority ordering of unrelated notes and still lets it
consume budget until enough higher-priority notes exist to push it under. What
is wanted is an explicit visibility toggle orthogonal to priority: hide this
note from injection, keep it stored and searchable, restore it on demand.

This was confirmed in discussion (2026-08-11) as a first-class companion to the
tracked repo layer: tracked Session Notes are actively used downstream and want
active pruning, and a visibility toggle is the pruning verb that does not
destroy the note.

## Decisions

Confirmed in discussion (2026-08-11):

- **Visibility is a general `note.*` attribute across all layers**, not a
  per-layer feature. It applies uniformly to `machine`, `worktree`, and the
  future tracked `repo` layer (`260810`). Chosen over a layer-specific hide
  mechanism so there is one visibility model to reason about.
- **`visible` is a boolean field on the record, default `true`, orthogonal to
  `priority`.** The stored record shape gains it:
  `[key, value, priority, written_at, visible]`. Priority still orders what is
  injected; visibility gates whether the note is a candidate for injection at
  all. The two axes do not interact — a muted high-priority note is simply not
  injected.
- **`note.write` does not touch `visible`; only mute/unmute do.** This is the
  orthogonality decision applied to the write path, and it is a load-bearing
  contract commitment (not a planning detail): `note.write` neither accepts a
  `visible` argument nor changes it. On an existing key it **preserves** the
  stored `visible`; on a new key it initializes `visible: true`. `value`,
  `priority`, and `written_at` are still fully overwritten/re-stamped exactly as
  today — the "full overwrite" applies to the note's *content*, while `visible`
  is display metadata owned solely by `note.mute` / `note.unmute`. **Consequence:
  updating a muted note's content does NOT un-mute it** — the mute persists until
  an explicit `note.unmute`. Chosen over the "full overwrite resets `visible` to
  default" reading because the epic's two-orthogonal-axes model and the confirmed
  "visibility is a general attribute orthogonal to priority" decision both require
  the axes not to bleed into each other; a silent un-mute on a content edit is
  exactly that bleed.
- **Muting is a set-state verb, not a write.** `note.mute(layer, [keys])` /
  `note.unmute(layer, [keys])` set `visible` to false / true and are
  **idempotent**: muting an already-muted key is a no-op, and — critically —
  muting/unmuting **does not re-stamp `written_at`**. This is the load-bearing
  distinction from `note.write`, which re-stamps on every call; visibility is
  metadata about display, not a new write of the note's content.
- **Muted notes leave the injection entirely, including the cap budget.** A
  muted note is excluded from the `# Notes` block *and* does not consume a
  `notesInjectionCap` slot — muting a note frees room for a lower-priority
  visible note to surface. (Contrast: an over-cap note still exists in the
  injection's accounting, just elided.)
- **A distinct elision line for muted notes.** Muted notes are surfaced by their
  own visible count line — `N muted — note.search to view` — kept separate from
  the existing "N lower-priority notes elided" over-cap line, because the two
  states have different causes (author intent vs. budget pressure) and different
  remedies (`note.unmute` vs. raising priority / cap). When both apply, both
  lines render.
- **Muted notes stay searchable.** `note.search` returns muted records unchanged
  (visibility gates injection only, never retrieval), so `note.search` is the
  advertised path back to a muted note and `note.unmute` restores it to
  injection.

## Open naming

- **`mute` / `unmute` are provisional.** They read well against "the note is
  still there, just silenced," but the pair is bikesheddable (`hide`/`show`, or a
  differently-named set-state pair). Settle the verb spelling during planning
  before the spec text is written; the *contract* above (idempotent set-state, no
  `written_at` re-stamp, cap-excluded, distinct elision line, searchable) is what
  is fixed. A `note.write(..., visible:)` spelling is **excluded**: it folds
  visibility into `note.write` (which the "`note.write` does not touch `visible`"
  decision forbids) and inherits `note.write`'s `written_at` re-stamp (which the
  no-re-stamp contract forbids).

## Search precision (planning-settleable)

- The "N muted — note.search to view" pointer relies on `note.search` returning
  muted records; it does (visibility gates injection only). `note.search` today
  takes only `glob` / `from` / `then`, so a caller following the pointer gets
  muted and visible records intermixed and distinguishes them by the `visible`
  field — mirroring the existing over-cap elision line's imprecision, so
  acceptable as-is. Planning may optionally add a `visible`/`muted` filter to
  `note.search` so the pointer is mute-scoped; not required for the contract.

## Coordination with the repo layer (`260810`)

Visibility touches the shared record shape and the shared injection path, both of
which `260810` also extends (it adds the third layer). The two are otherwise
independent and can land in either order:

- If this ticket lands first, it adds `visible` to the two shipped layers'
  record and injection; `260810` inherits the field when it adds the `repo`
  layer.
- If `260810` lands first, this ticket extends `visible` across all three layers
  at once.

Whichever lands second extends the shared shape rather than redefining it. No
hard dependency in either direction; note the coupling so the second
implementer does not treat the record shape as frozen.

## Spec Impact

No new stem — this extends the note.* contract authored by
`260807-feat-note-memory-layers`.

- **`spec/mcp-tools.md`** `## Note Tools {#260810-note-tools}`: add the `visible`
  field to the documented record shape
  (`[key, value, priority, written_at, visible]`, default `true`) and the
  `note.mute` / `note.unmute` verbs — idempotent set-state, no `written_at`
  re-stamp. Also amend the `note.write` contract: it does not accept or modify
  `visible`, preserving the stored value on an existing key and initializing new
  keys to `true`, so a content overwrite never changes visibility. Caller-visible
  change: two new note verbs, a new record field, and a clarified `note.write`
  that leaves `visible` untouched.
- **`spec/mcp-tools.md`** `### Note Injection {#260810-note-injection}`: document
  that muted notes are excluded from the injected `# Notes` block and from the
  `notesInjectionCap` budget, and that they surface as a distinct
  "N muted — note.search to view" line separate from the over-cap elision line.
  Caller-visible change: `workflow_manual` output gains a muted-count line and
  drops muted notes from the injected set.

## Phases

### Phase 1: `visible` field + mute/unmute verbs + injection exclusion

Deliver against the two shipped non-tracked layers (`machine`, `worktree`); the
`repo` layer inherits the attribute when `260810` lands.

- Extend the `internal/wsnote` record with a `visible` boolean, default `true`,
  and migrate existing records as visible (absent field reads as `true`).
- Make `note.write` preserve the stored `visible` on an existing key and set
  `true` on a new key — it neither accepts nor mutates `visible` (so a content
  overwrite never un-mutes). Cover with a write-over-a-muted-key test asserting
  the mute survives.
- Add `note.mute(layer, [keys])` / `note.unmute(layer, [keys])` MCP tools
  (final verb spelling settled in planning): idempotent set-state on `visible`,
  reusing the existing flock + temp-file + atomic-rename RMW pattern; **no
  `written_at` re-stamp** (verify with the injectable clock the note-memory
  tests already use).
- In the `workflow_manual` `# Notes` computation: filter muted notes out before
  the priority-ordered cap so they consume no slot, and emit the distinct
  "N muted — note.search to view" count line when any layer has muted notes,
  independent of the existing over-cap elision line. The all-muted edge: the
  existing empty-block skip fires only when there are **no notes at all** — if any
  layer has muted notes the block still renders (heading + muted-count line) even
  when zero visible notes remain, so muting does not hide the fact that muted
  notes exist.
- `note.search` returns muted records unchanged.

Verification: a write → mute round trip drops the note from a subsequent
`workflow_manual` injection while `note.search` still returns it and
`written_at` is unchanged across the mute; unmute restores it to injection;
muting an over-cap-adjacent note frees a slot for a previously-elided visible
note; the muted-count line and the over-cap elision line render independently
and together; muting an already-muted key is a no-op.
