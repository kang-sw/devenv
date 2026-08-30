---
title: "note.write oversize-note relocate/erase nudge"
dropped: 2026-08-23
---

# note.write oversize-note relocate/erase nudge

## Background

Agents over-rely on `ws/note.write`: content that belongs in a ticket, spec, or
mental-model accumulates as durable notes instead. This is a **misfiling**
problem (not a raw context-weight problem — the ambient `# Notes` block already
caps item count at 20). The existing "no better home" discipline lives only as
passive prose in the workflow manual and does not change write-side behavior.

Goal: a lightweight nudge that fires at the moment a note is written, reminding
the agent to route misfiled content to its proper home before it settles into
durable memory.

## Decisions

- **Stateless trigger, at `note.write`.** When `note.write` receives a `value`
  whose length is `>= ~300` characters (a trivially-tunable starting knob, like
  the existing 20-item injection cap), append a short challenge to the tool
  response. No latch, no per-session state, no `workflow_manual` change.
  - Rejected: arming a one-shot latch at every `workflow_manual` load (fire once
    per session/compaction cycle). It resolves a noise concern that the length
    gate already suppresses — oversize writes are sparse (~0–3/session), so
    per-write challenge is already quiet. Adds per-session-key armed-flag
    plumbing for a shave from ~3 to ~1. Deferred to a follow-up **only if**
    dogfooding shows the stateless version is actually noisy.
  - Rejected: firing on *every* `note.write` regardless of size — that is the
    noisy behavior the length gate exists to avoid.

- **Non-blocking, post-write challenge.** The note is still written; the
  challenge rides the response as a reminder. A pre-write hard gate is heavier
  than a nudge warrants and invites re-submission to get past it.

- **Remediation verbs are relocate / erase — NOT mute.** `note.mute` is a
  visibility flag only (data retained until `note.erase`, still visible to
  `note.search`). If mute is offered as remediation it becomes the
  minimum-resistance escape: the agent mutes to silence the warning without doing
  the valuable relocation, producing a write-only graveyard (content neither in
  context nor in its proper home). Also, by the keep-test below, a *valid* note
  is never a mute candidate (it must always stay in context); an *invalid* note
  should be relocated or erased. Mute is for the separate "durable, correctly
  homed here, but not every-session" case, which this nudge does not target.

- **Challenge phrasing: one to two lines, routing-oriented.** Confirmed starting
  text:
  > Large note — keep only if volatile AND homeless AND must-always-stay-in-context;
  > otherwise move it to a ticket/spec/mental-model, or erase. Not mute.

## Constraints

- No note-`value` length is currently recorded anywhere; the size signal is new
  instrumentation (a `len(value)` check in the write handler), not a conditional
  on existing data.
- Keep the nudge weight-free on the read side: it appears only in the
  `note.write` response, never as a standing injection into `workflow_manual`
  output (the notes block is deliberately *appended* as session-context, not
  *prepended* as a standing warning).

## Prior Art

- `note.write` handler and note subsystem: `agents-plugin-tool/internal/mcp/note_tools.go`
  (`handleNoteWrite`), storage under `agents-plugin-tool/internal/wsnote/`.
- Ambient `# Notes` injection (20-item cap, priority-desc sort, muted-excluded):
  `agents-plugin-tool/internal/wsnote/inject.go`,
  `agents-plugin-tool/internal/mcp/note_announcement.go`.
- Existing per-load nudge pattern for reference (prepend warnings computed once
  at `workflow_manual` FRESH/CONTINUE): `bootstrapStalenessWarning` /
  `docCoverageWarning` in `agents-plugin-tool/internal/mcp/workflow_manual.go`.
- Spec surfaces to update on landing: `ai-docs/spec/mcp-tools.md` note-tools
  anchor (`#260810-note-tools`).

## Out of scope (deferred)

- **Existing-backlog hygiene.** This nudge only affects newly written notes; it
  does not revisit already-accumulated misfiled notes. A separate conditional
  `workflow_manual` audit (prepend a one-line report only when total ambient note
  *size* crosses a budget) would cover that, and is deliberately deferred.
- The latch variant (see rejected alternative above).

## Phases

### Phase 1: Oversize-note relocate/erase nudge on note.write

Add a `len(value) >= threshold` check to the `note.write` handler that appends
the confirmed one-to-two-line challenge to the tool response. Threshold is a
named, trivially-tunable constant (start `~300`). No `workflow_manual` change, no
session state. Update the `mcp-tools.md` note-tools anchor to document the nudge
and threshold. Verify: an oversize write returns the challenge; a sub-threshold
write does not; the note is written either way.


## Resolution (2026-08-23)

Absorbed into 260823-feat-notes-postit-discipline (facet A). Merged with its sibling into one ready ticket at the user's request.
