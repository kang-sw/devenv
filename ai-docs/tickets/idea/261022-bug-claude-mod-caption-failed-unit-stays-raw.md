---
title: Claude mod captions - raw rows that never caption, and Edit diffs a click cannot reopen
related:
  261022-feat-claude-mod-tool-call-captions: the feature these issues are in
  261022-bug-claude-mod-caption-prerun-row: the confirmed empty-caption cause, split out and fixed there
---

# Claude mod captions - raw rows that never caption, and Edit diffs a click cannot reopen

## Background

Dogfood observations with `claude.display_summary_enabled` on, still
unconfirmed. The empty Write/Edit captions ("input also empty", `0.0 KB`) were
confirmed as a pre-run render and split into
`261022-bug-claude-mod-caption-prerun-row`.

### Raw rows

Some collapsed rows stay raw (`Called plugin:ws:ws`, `Called plugin:ws:ws 3
times`, `Update` with its diff) while other rows of the same tools caption
fine, so the tools are hooked and the failure is per unit. A raw `Update` row
was also seen to turn into a captioned `Edit` later, so some raw rows are only
pending. Three kinds to tell apart:

- Pending: queued (1.5 s tick) or awaiting the model; swaps in later.
- Permanently failed: `request(unit)` marks the unit `requested` once; an
  unanswered request (`ask` returns undefined, logged only as
  `request not answered: <reason>`) or a reply item missing fields (skipped with
  `continue`) is never asked again until a reload. Requests run one at a time
  (`isFlushing`) under a 30 s `timeoutMs` (`captions.tsx:78`), which per the mod
  API bounds "the whole call"; whether engine-side queueing counts against it
  is undocumented. `aborted` also covers a dispatch abort, so the reason alone
  does not separate a timeout from an abort.
- Captioned but not redrawn: `redraw()` may not reach a row the engine does not
  invalidate (e.g. scrolled out). Hypothesis only.

Observed caption latencies on haiku in one session: 3.8 s to 14.5 s; no
`request not answered` line in that session's debug log.

Reproduced 2026-10-22 (debug.log, probe mod): a single-call ws MCP group
(`Called plugin:ws:ws`) stayed raw after a user interrupt at 12:49:33. Ruled
out: request failure (every `$.model.complete` answered, none unanswered) and
redraw (the reply at 12:50:25.69 was followed by ws `ui.render` passes at
.71). The four candidate groups rendered inactive, calls-only, every call with
an `output`. Remaining: the unit's reply item was skipped, or the unit was
marked `requested` at an earlier render and never asked again. Telling these
apart needs a debug log inside the ws mod (requested unit keys, skipped reply
items, unanswered reason with elapsed time); the probe mod cannot see it.

### Edit diff after a caption

A captioned Edit draws no diff: the `ToolResult` hook (`register.ts:871-875`)
draws an empty box for a captioned, non-errored call. `ctrl+o` brings the diff
back; a mouse click does not, and only on Edit rows (clicking other captioned
rows works). Open: whether a click sets `isExpanded` on the `ToolResult` hook
for Edit; a debug log of that hook's props on a click would show it. This is
information loss, so it ranks above the cosmetic cases. Also decide whether
file-edit calls should keep their diff while captioned.

## Open

- Capture a raw row that never swaps in with the debug lines that landed with
  `261022-bug-claude-mod-caption-prerun-row` (`ws captions: queued`, `left
  raw, ...`, `reply item skipped, ...`, `request not answered: ... after N
  ms`).
- Decide the raw-row fix: one bounded retry, a visible failure marker, or both.
  Keep the render-loop guard: a failed unit never retries per render.
- Probe `ToolResult` props on a click of a captioned Edit.

## Phases

### Phase 1: Confirm the causes, then fix

Confirm each cause above with a debug log before choosing its fix.
