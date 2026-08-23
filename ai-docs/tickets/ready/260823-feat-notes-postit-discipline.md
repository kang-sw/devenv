---
title: "Notes post-it discipline — oversize write nudge + always-render standing hint"
related:
  260823-feat-note-write-oversize-relocate-nudge: absorbed source — merged into this ticket, dropped on landing
  260823-feat-notes-block-standing-postit-hint: absorbed source — merged into this ticket, dropped on landing
sage-review-design: completed
sage-review-completeness: completed
---

# Notes post-it discipline — oversize write nudge + always-render standing hint

## Background

Agents over-rely on `ws/note.write`: content that belongs in a ticket, spec, or
mental-model accumulates as durable notes. This is a **misfiling** problem, not a
raw context-weight problem — the ambient `# Notes` block already caps item count
at 20. The "no better home" discipline currently lives only as passive prose in
the workflow manual and does not change behavior.

This ticket reshapes the notes surface toward one coherent identity — **short,
always-in-context post-it reminders** — with two complementary facets of a single
behavior slice:

- a **write-side nudge** that discourages the wrong use (large/misfiled durable
  knowledge), and
- a **read-side standing affordance** that teaches the right use (pin a short
  reminder, remove it when done).

This ticket absorbs and replaces two idea tickets from the same discussion:
`260823-feat-note-write-oversize-relocate-nudge` and
`260823-feat-notes-block-standing-postit-hint`; both source tickets are moved to
`.dropped/` as part of landing this merged ticket.

## Decisions

### Write-side: oversize-note nudge (facet A)

- **Stateless trigger at `note.write`.** When `value` length is `>= ~300`
  characters (a trivially-tunable named constant, like the existing 20-item
  injection cap), append a short challenge to the tool response. No latch, no
  per-session state, no `workflow_manual` change.
  - Rejected: a one-shot latch armed at every `workflow_manual` load. The length
    gate already makes oversize writes sparse (~0–3/session), so per-write
    challenge is already quiet; the latch adds per-session-key armed-flag
    plumbing to shave ~3 → ~1. Reconsider only if dogfooding proves the stateless
    version noisy.
  - Rejected: firing on *every* write regardless of size — the noisy behavior the
    length gate exists to avoid.
- **Non-blocking, post-write.** The note is still written; the challenge rides the
  response. A pre-write hard gate is heavier than a nudge warrants and invites
  re-submission to get past it.
- **Remediation verbs are relocate / erase — NOT mute.** `note.mute` is a
  visibility flag only (data retained until `note.erase`, still visible to
  `note.search`); offered as remediation it becomes the minimum-resistance escape
  and builds a write-only graveyard (content neither in context nor in its proper
  home). By the keep-test, a valid note is never a mute candidate.
- **Batch writes.** `note.write` accepts an array of notes; the challenge is
  appended once per call when any note's `value` crosses the threshold, not once
  per oversized note.
- **Confirmed phrasing (one to two lines):**
  > Large note — keep only if volatile AND homeless AND must-always-stay-in-context;
  > otherwise move it to a ticket/spec/mental-model, or erase. Not mute.

### Read-side: always-render standing post-it hint (facet B)

- **Always render `# Notes`,** including the empty state — deliberately breaking
  the sibling injections' silent-when-empty contract. Justified because `# Notes`
  is a user-facing affordance (model it on the always-rendered `# Manuals`
  block), not a machine-computed warning like `bootstrapStalenessWarning` /
  `docCoverageWarning`.
- **The post-it hint is standing — shown even when notes exist,** not empty-only.
  Decisive rationale: the reader is the agent, not the human, and the agent is a
  *memento* — its working memory resets each turn, so the affordance and its
  prune verb must stay continuously visible. An empty-only hint assumes a
  persistent-memory reader that does not exist.
  - Rejected: empty-only full hint with no hint when populated (wrong reader
    model).
- **Standing (non-empty) hint kept to one line** to bound per-`workflow_manual`
  weight; the fuller phrasing appears only in the empty state.
- **Hint names both attach and detach verbs** (`note.write` / `note.erase`) so
  "remove when no longer needed" is prompted, not just the write. Detach verb is
  `erase`, consistent with facet A's relocate/erase (not mute) remediation.
- **Three render states, not two.** The injection has three cases (see
  `#260810-note-injection`): no notes on any layer, all-muted (notes exist but
  zero visible), and has-visible. Key the empty-state post-it hint off the
  **existing empty-skip predicate (no notes on any layer)** — never off
  zero-visible — so an all-muted session never emits a false "No notes." while
  muted notes exist. The all-muted case keeps rendering its current heading +
  muted-count line and counts as "notes present" for the one-line standing hint.
- **Confirmed phrasing:**
  - Empty state:
    > _No notes. Notes are your short, always-in-context post-it reminders — `note.write` to pin one, `note.erase` when it's no longer needed._
  - Non-empty (trailing, one line under the notes list):
    > _Post-it reminders: `note.write` to pin, `note.erase` when done._

## Constraints

- No note-`value` length is recorded anywhere today; facet A's size signal is new
  instrumentation (a `len(value)` check in the write handler), not a conditional
  on existing data.
- Facet B keeps today's placement: the notes block is appended after `## Session
  State` (session-context, not a prepended standing warning). Only the
  empty-render and hint behavior change.
- Both facets stay weight-conscious: facet A's challenge appears only in the
  `note.write` response (never a standing `workflow_manual` injection); facet B's
  non-empty hint is a single line.

## Prior Art

- `note.write` handler / note subsystem: `agents-plugin-tool/internal/mcp/note_tools.go`
  (`handleNoteWrite`), storage under `agents-plugin-tool/internal/wsnote/`.
- Ambient `# Notes` injection (20-item cap, priority-desc sort, muted-excluded,
  skipped-when-empty): `agents-plugin-tool/internal/wsnote/inject.go` (`Compute`),
  `agents-plugin-tool/internal/mcp/note_announcement.go`; call sites in
  `agents-plugin-tool/internal/mcp/workflow_manual.go` (FRESH-with-root and
  CONTINUE branches).
- Standing-surface model to mirror for facet B: the `# Manuals` block.
- Prepend-warning pattern (reference, NOT used here): `bootstrapStalenessWarning`
  / `docCoverageWarning` in `workflow_manual.go`.

## Spec Impact

Target spec area: `ai-docs/spec/mcp-tools.md`, note subsystem anchors.

- Note-tools anchor (`#260810-note-tools`): document facet A — `note.write`
  appends the relocate/erase challenge to its response when `len(value)` meets or
  exceeds the tunable threshold; the write itself is unconditional; threshold is a
  named tunable constant.
- Note-injection anchor (`#260810-note-injection`): update the current
  skipped-when-empty contract to always-render, and document all three render
  states — no-notes (empty-state post-it hint, keyed off the existing
  no-notes-on-any-layer predicate), all-muted (existing heading + muted-count
  line, plus the standing hint), and has-visible (notes list + one-line standing
  hint). Note the deliberate divergence from the sibling injections'
  silent-when-empty contract and the reason (user-facing affordance).

## Phases

### Phase 1: Notes post-it discipline (write nudge + always-render standing hint)

Deliver both facets as one reviewable slice (they are independent but small,
share the note subsystem, and update one spec area — no sequential dependency, so
one phase):

- Facet A: add a `len(value) >= threshold` check to the `note.write` handler that
  appends the confirmed relocate/erase challenge to the tool response; the note is
  written regardless. Threshold is a named, trivially-tunable constant (start
  `~300`).
- Facet B: change the injection path so `# Notes` renders even with zero notes
  (empty-state post-it hint) and appends the one-line standing hint when notes are
  present.
- Update the `mcp-tools.md` note-tools and note-injection anchors per `## Spec
  Impact`.

Verification:
- Oversize (`>= threshold`) `note.write` returns the challenge; a sub-threshold
  write does not; the note is written in both cases.
- Zero-note session renders the empty-state post-it hint; a session with visible
  notes renders the notes list followed by the one-line standing hint.
- All-muted session (notes exist, zero visible) still renders the heading +
  muted-count line and the standing hint — never the "No notes." empty hint.
- `ai-docs/spec/mcp-tools.md` note-tools and note-injection anchors are updated
  per `## Spec Impact`.
- The two absorbed source idea tickets are moved to `.dropped/`.
- `go test ./...` and `go vet ./...` in `agents-plugin-tool/` pass.
