---
title: Note layer discipline - worktree-first writes, worker read-only notes, bounded ambient Notes block
---

# Note layer discipline - worktree-first writes, worker read-only notes, bounded ambient Notes block

## Background

`note.write` takes a required `layer` enum with no default and no preference
signal (`agents-plugin-tool/internal/mcp/server.go` note.write schema); the only
steering is the workflow manual's "Use the narrowest layer that reaches its
readers" (`agents-plugin/rsrc/lead-workflow-manual/lead-workflow-manual.md`,
`### Notes / durable memory`). The only write-time check is the >=300-byte
oversize nudge (`internal/mcp/note_tools.go`, `noteOversizeChallengeFor`).

Notes accumulate unbounded. On the owner's machine (2026-10-30) one project's
clone store held 121 notes / 182 KB and another 17.6 KB clone + 12.9 KB
worktree. The ambient `# Notes` block (`internal/wsnote/inject.go`, `Compute`)
inlines the top `notesInjectionCap = 20` (`internal/mcp/note_announcement.go`)
and reduces everything else to one count line, so elided notes show not even a
key and are effectively undiscoverable.

Any session scope can write notes today: `roleAllowsTool`
(`server.go`, `func roleAllowsTool`) lets delegate and leaf keys call every
`note.*` tool.

## Decisions

- **Clone/machine writes require a reason.** `note.write` with `layer:
  "clone"` or `layer: "machine"` requires a non-empty `reason` string argument
  stating why the note must reach beyond this worktree; the call is rejected
  without it. The reason is not stored on the record and is not echoed into the
  Notes block. Worktree and repo writes take no reason.
  Rejected: a post-write soft nudge only (does not force the decision at write
  time); storing the reason (grows the very notes this ticket bounds).
- **Worktree-first wording.** The `note.write` and `note.query` schema layer
  descriptions present `worktree` first, and the `clone`/`machine` entries
  state their use criterion: write there only when a session in another
  worktree (clone) or another project (machine) would fail without the fact.
  The workflow manual's `### Notes / durable memory` sentence is aligned with
  the same rule.
- **Workers are note read-only.** For delegate and leaf scoped session keys,
  `note.query` stays allowed; `note.write`, `note.erase`, `note.mute`, and
  `note.unmute` are denied through the existing role gate. The denial for
  these four tools carries a note-specific message telling the caller to put
  the note content in its final report so the lead can write it, instead of
  the generic `tool not available in current ws MCP profile` text. Mute and
  unmute count as writes because they change state every session sees.
  Rejected: a new elevation tool (workers already report to the lead).
- **Machine notes are pinned.** In the ambient `# Notes` block, visible
  machine-layer notes always render first and are exempt from every cap and
  from the counts below. Muted machine notes stay hidden, as muted notes do
  today.
- **Bounded non-machine display.** Non-machine notes (worktree, clone, repo),
  ordered by the existing `CompareRecords` comparator:
  - the first 20 render with their body, as today;
  - the next 30 render key-only, each key layer-tagged, on one line;
  - anything beyond 50 renders as a `+N hidden` line, on every render.
- **Hidden-notes cleanup nudge.** When the `+N hidden` line is present, the
  block also carries a strong cleanup warning: the hidden count with its
  per-layer breakdown, and the remediation (`note.query` the hidden notes,
  `note.erase` the stale ones, move still-needed detail to a ticket or manual;
  not mute). The warning is throttled to at most once per day per clone, with
  a throttle file in the git common dir, following the worktree pool nudge
  (`internal/mcp/worktree_nudge.go`, `worktreeNudgeThrottleFile` /
  `worktreeNudgeInterval`). Rejected: once per session key per 24h (repeats on
  every new session; the condition is a store property, and one session's
  cleanup resolves it).
- **Stale-note guidance.** The block's closing line (`inject.go`, the
  "_Post-it reminders, ..._" footer) is replaced by one line that says to
  erase stale notes immediately on sight with `note.erase`. The empty-state
  text is unchanged. Rejected: a new sentence in the manual body (three
  mirrored copies; the footer renders next to the notes on every call).

## Constraints

- Out of scope: a total-size (10 KB) warning — deferred by the owner.
- Out of scope: `agents-plugin/rsrc/executor-wrapup.md` (and its wsflow/pi
  copies) still instructs a `note.write(layer: "repo")`, but no playbook loads
  it (only `rsrc/manifest.json` and `playbook_tools_test.go` reference it);
  its cleanup is a separate decision.
- Manuals: `ai-docs/manuals/shipped-surface-boundary.md` (every MCP string
  here ships), `ai-docs/manuals/ws-mcp.md` (`internal/mcp/`),
  `ai-docs/manuals/skill-authoring.md` and
  `ai-docs/manuals/wsflow-mirroring.md` (the manual sentence and its
  wsflow/pi mirrors).

## Phases

### Phase 1: Note layer discipline

Implement every decision above in `agents-plugin-tool/internal/mcp/` and
`internal/wsnote/`, plus the manual sentence and its mirrors.

Verification: Go tests covering the `reason` rejection per layer, the
delegate/leaf denial (write/erase/mute/unmute denied, query allowed, message
text), machine pinning and cap exemption including muted machine notes, the
20/30/`+N hidden` boundaries, the once-per-day-per-clone throttle, and the new
footer; the existing note and workflow_manual test suites pass.
