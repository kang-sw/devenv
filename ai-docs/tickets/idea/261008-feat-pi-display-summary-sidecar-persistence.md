---
title: "Persist Pi display summaries in a lead-conversation sidecar with lifecycle handling"
related:
  261007-feat-pi-display-summary: extends the existing memory-only display summaries
---

# Persist Pi display summaries in a lead-conversation sidecar with lifecycle handling

## Background

Display summaries currently live in an in-memory map in
`agents-plugin-pi/src/display-summary.ts`; session initialization clears that
store in `src/display-summary-session.ts`. Reloading or reopening loses the
summaries even though the lead conversation is retained.

The user requested capturing persistent summary storage and loading as a
separate ticket rather than extending the display hotfix. The requested storage
is a sidecar associated with the lead conversation. The user explicitly
clarified that EOL means end-of-lifecycle handling, not line-ending syntax.

## Decisions

- Capture persistence separately; do not implement it in the current hotfix.
- Store and load display summaries through a lead-conversation sidecar.
- Include lifecycle handling for that sidecar, rather than treating initial
  save/load alone as the whole feature.
- Use append-only key/value JSONL rather than rewriting the full accumulated
  summary map. File: `<leadSessionFile>.ws-display-summaries.log`; the content
  is JSONL, but the suffix avoids Pi's `.jsonl` conversation discovery.
  Rejected: whole-map JSON snapshots rewrite all accumulated summaries after
  each batch; the user prefers incremental accumulation.
- Each record contains `version: 1`, `sessionId` (the Pi session header ID),
  `id` (the existing renderer row ID), and `summary` (required strings
  `toolIntention` and `toolResult`, plus optional string `optionalContext`).
  Load records in order; the last valid value for an ID wins.
- Preserve sidecars on quit, reload, session switches, and compaction. At lead
  startup clean orphan summary sidecars directly within the active session
  file's directory when their conversation file is absent. Do not scan globally
  or recursively. Rejected: manual-only cleanup leaves orphan files behind;
  immediate deletion interception has no corresponding extension API event.
- Serialize accepted-batch appends per active lead session; pending writes
  remain bound to their originating session, never a replacement session.
  Ignore an incomplete final record on load and recover the line boundary
  before the next append without losing preceding valid records.
  Rejected: concurrent per-row writes complicate record boundaries and ordering.
- Validate supported record version, matching Pi session ID, row ID, and summary
  strings instead of blindly replaying data. Missing/invalid records leave the
  corresponding rows raw. I/O failures preserve live in-memory summaries and
  original-row fallback. Sessions without a saved conversation file remain
  memory-only; do not create a durable sidecar for setup-only unsaved sessions.
- Persist display summaries only, not provider request history, invalidators,
  or the summarizer's random request-session UUID. No power-loss durability or
  multi-process locking guarantee is introduced by the serialized append rule.
- Inherit retained-row summaries only for Pi native conversation forks/copies,
  not ws fork subagents. Prefer an existing valid child sidecar; otherwise seed
  only summaries for copied rows, using the child Pi session ID. Thereafter
  parent and child sidecars are independent. Rejected: starting native forks
  without inherited summaries discards useful copied display state; a shared
  writable parent file or ongoing parent fallback couples their lifecycles.

## Record Format

File: `<leadSessionFile>.ws-display-summaries.log`.
One JSON object per line, terminated by a newline:

```json
{"version":1,"sessionId":"<Pi session header ID>","id":"<renderer row ID>","summary":{"toolIntention":"<intention>","toolResult":"<result>","optionalContext":"<omit when absent>"}}
```

## Phases

### Phase 1: Sidecar persistence, restoration, and lifecycle

Add incremental summary persistence alongside the lead conversation using the
confirmed record format and exact renderer row IDs. Load saved summaries on
startup/reopen/reload so retained rows can render without regenerating their
summaries. Keep accepted-batch writes ordered and session-bound, and preserve
original-row rendering on absent, invalid, or unreadable cache data.

Handle the confirmed lifecycle: retain cache across normal session transitions
and compaction; clean only matching orphan sidecars in the active session
directory on lead startup; initialize independent native-conversation fork
sidecars from retained rows without enabling persistence for fork subagents.

Verification must cover incremental append rather than full-file rewriting;
last-valid-record replay; reload/reopen restoration using persisted row IDs;
interrupted-tail recovery and invalid/foreign-version/foreign-session records;
I/O fallback and writes pending during session replacement; saved versus
unsaved sessions; scoped orphan cleanup preserving live conversation sidecars;
and native fork inheritance, independent subsequent writes, and subagent
exclusion. Fact population and the configured promotion reviews are still
required before this idea becomes an execution target.
