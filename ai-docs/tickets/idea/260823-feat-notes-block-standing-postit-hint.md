---
title: "# Notes block: always render with a standing post-it hint"
related:
  260823-feat-note-write-oversize-relocate-nudge: sibling — the positive counterpart; that ticket discourages misfiled/oversize notes, this one teaches the correct short-resident post-it use
---

# # Notes block: always render with a standing post-it hint

## Background

The ambient `# Notes` block is skipped entirely when zero notes exist on any
layer (matching sibling injections' silent-when-empty contract). So the surface
is invisible exactly when an agent most needs to be told it exists.

This is the positive counterpart to `260823-feat-note-write-oversize-relocate-nudge`:
that ticket discourages the wrong use (large/misfiled durable knowledge → relocate
or erase); this ticket teaches the right use (short, always-in-context post-it
reminders) by giving `# Notes` a standing affordance, modeled on the `# Manuals`
block which always renders as a standing surface.

Goal: make the AI actively pin a note when the user says "let's remember this,"
and remove it when it is no longer needed — by keeping the affordance and its
attach/detach verbs continuously visible.

## Decisions

- **Always render `# Notes`**, including the empty state — deliberately breaking
  the sibling injections' silent-when-empty contract. Justified because `# Notes`
  is a user-facing affordance (like `# Manuals`), not a machine-computed warning
  (like `bootstrapStalenessWarning` / `docCoverageWarning`).

- **The post-it hint is standing — shown even when notes exist**, not empty-only.
  Rationale (decisive): the reader is the agent, not the human, and the agent is
  a *memento* — its working memory resets each turn, so it needs the standing
  reminder of what the surface is for and to prune it. An empty-only hint would
  assume a reader who remembers the surface between turns; the agent does not.
  - Rejected: empty-only full hint with no hint when populated. Assumes a
    persistent-memory reader; wrong model for the actual (agent) reader.

- **Keep the standing (non-empty) hint to one short line** to bound the
  per-`workflow_manual` weight; the fuller phrasing appears only in the empty
  state.

- **Hint names both attach and detach verbs** (`note.write` / `note.erase`) so
  the "remove when no longer needed" behavior is prompted, not just the write.
  Detach verb is `erase` — consistent with the sibling ticket's relocate/erase
  (not mute) remediation.

- **Confirmed starting phrasing:**
  - Empty state:
    > _No notes. Notes are your short, always-in-context post-it reminders — `note.write` to pin one, `note.erase` when it's no longer needed._
  - Non-empty (trailing, one line under the notes list):
    > _Post-it reminders: `note.write` to pin, `note.erase` when done._

## Constraints

- Standing weight: the non-empty hint rides every `workflow_manual` FRESH/CONTINUE
  render, so it stays a single line.
- Placement parity with today: the notes block is appended after `## Session
  State` (session-context, not a prepended standing warning); this change keeps
  that placement and only changes the empty-render and hint behavior.

## Prior Art

- Empty-skip and block assembly: `agents-plugin-tool/internal/wsnote/inject.go`
  (`Compute`), cap/wrapper `agents-plugin-tool/internal/mcp/note_announcement.go`,
  call sites `agents-plugin-tool/internal/mcp/workflow_manual.go` (FRESH-with-root
  and CONTINUE branches).
- Standing-surface model to mirror: the `# Manuals` block (always rendered).
- Spec surface to update on landing: `ai-docs/spec/mcp-tools.md` note-injection
  anchor (`#260810-note-injection`), which currently documents the
  skipped-when-empty behavior.

## Phases

### Phase 1: Always-render # Notes with standing post-it hint

Change the injection path so `# Notes` renders even with zero notes (empty-state
post-it hint) and appends the one-line standing hint when notes are present.
Update the `mcp-tools.md` note-injection anchor to document always-render plus
the empty vs non-empty hint. Verify: zero-note session shows the empty hint;
non-zero session shows the notes list followed by the one-line standing hint.
