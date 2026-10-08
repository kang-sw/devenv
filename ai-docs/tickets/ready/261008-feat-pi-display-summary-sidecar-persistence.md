---
title: "Persist Pi display summaries in a lead-conversation sidecar with lifecycle handling"
related:
  261007-feat-pi-display-summary: extends the existing memory-only display summaries
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: f9fc153241649568
sage-review-completeness-reviewed: f9fc153241649568
---

# Persist Pi display summaries in a lead-conversation sidecar with lifecycle handling

## Background

Display summaries currently live in an in-memory map in
`agents-plugin-pi/src/display-summary.ts`; session initialization clears that
store in `agents-plugin-pi/src/display-summary-session.ts`. Reloading or reopening loses the
summaries even though the lead conversation is retained
(`agents-plugin-pi/src/display-summary.ts#L65-L80`,
`agents-plugin-pi/src/display-summary-session.ts#L66-L87`).

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
- Target the current Pi 1.0.4 host contract; pinned 0.84.4 compatibility and
  dependency upgrades are outside this ticket, following commit 7c282769.
- When the originating conversation first becomes saved, backfill accepted
  in-memory summaries. Rejected: saving only subsequent batches loses early
  display state on reopening.
- An existing usable child sidecar is authoritative, including an empty file
  or a partially valid file. Do not fill missing rows from the parent.
  Rejected: hole-filling can resurrect parent state on repeated child opens.
- Inheritance requires verifiable native source provenance and an available
  source. Otherwise retain the child's own valid summaries or raw fallback.
  Equal session IDs or similar names do not establish source provenance;
  distinct conversation paths never share a writable sidecar.
- Native-fork inheritance includes parent summaries already accepted at the
  cutover, even with appends queued. Do not wait for unfinished model requests.
  Rejected: disk-only inheritance makes write scheduling change inherited state.
- On orderly shutdown/reload/session replacement, drain accepted append work
  with I/O failure fallback, without waiting for unfinished model requests.
  Do not recreate a sidecar after its conversation is observed absent. Keep
  acceptance, restoration and queued writes bound to their originating session;
  newer live acceptance wins over older restored values. Rejected: best-effort
  teardown unnecessarily loses accepted summaries. Existing crash, power-loss
  and multi-process exclusions remain unchanged.
- Leave a wholly foreign-version/foreign-session file untouched and disable
  persistence for that file while retaining live summaries. Rejected: replacing,
  quarantining or mixing into an unrecognized file risks other data. Ignore
  invalid records in a recognized file. Orphan cleanup deletes only recognizable
  supported sidecars with consistent ownership metadata and an absent matching
  conversation.
- Accept complete valid final JSON without a terminating newline and add the
  missing line boundary before appending. Ignore malformed final data without
  deleting preceding records; boundary repair may retain invalid data for the
  loader to ignore. Rejected: mandatory trailing-newline validity discards a
  complete record, and destructive truncation is unnecessary.
- Skip symlinked conversations/sidecars for persistence, repair, inheritance
  and orphan deletion. Rejected: following targets undermines directory-scoped
  mutation and can affect unrelated files.
- Validate row eligibility against all retained stored conversation history,
  including pre-compaction and abandoned-branch rows, not only the active
  branch. Preserve existing renderer IDs; native inheritance still filters to
  entries retained in the child. Rejected: active-branch pruning loses saved
  state when switching branches. Apply the adapter's summarizable-row policy:
  the rendered Previous conversation block is excluded, per the user's separate
  hotfix request; persistence does not re-enable its summary replacement.

## Record Format

File: `<leadSessionFile>.ws-display-summaries.log`.
One JSON object per line, terminated by a newline:

```json
{"version":1,"sessionId":"<Pi session header ID>","id":"<renderer row ID>","summary":{"toolIntention":"<intention>","toolResult":"<result>","optionalContext":"<omit when absent>"}}
```

## Prior Decisions

- 261007-feat-pi-display-summary (2026-10-08, commit f73c52e6): "Known Minor left open: rows queued while a summary request is in flight wait for the next turn_end/agent_end" — bearing: constrains
- 7c282769 (2026-10-08, commit): "The current Pi 1.0.4 facade supports this path; the pinned 0.84.4 facade lacks streamSimple and is not claimed supported by this hotfix." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/display-summary.ts and agents-plugin-pi/src/display-summary-session.ts; persistence and lifecycle tests alongside the existing test suites |
| scope.surface | cross-module | summarizer/store, session lifecycle wiring, persisted renderer row IDs, and Pi SessionManager file/header/branch contracts |
| scope.new_public_symbol | no | no new user-facing command or configuration key requested; persistence is adapter-owned |
| scope.new_type_contract | yes | version 1 JSONL record with sessionId, id, and summary; exact format in Record Format |
| scope.test_surface | existing | agents-plugin-pi/test/display-summary.test.ts, agents-plugin-pi/test/display-summary-session.test.ts, agents-plugin-pi/test/display-summary-render.test.ts; no new test file is required by the phase |
| complexity.reuse_points | confirmed | createDisplaySummaryStore and parseSummaryResponse in agents-plugin-pi/src/display-summary.ts; registerDisplaySummarySession in agents-plugin-pi/src/display-summary-session.ts; summaryIdOf in agents-plugin-pi/src/summary-id.ts; compaction-history.ts keys its renderer by entry.id |
| complexity.side_effect_risk | high | appends and repairs filesystem data, deletes orphan sidecars, and seeds native children across runtime replacement |
| risk.correctness | high | asynchronous config/provider/load/write completion must preserve originating session ownership; reset permits old-generation successful summaries while dispose rejects late responses (display-summary.ts#L428-L500, #L532-L542) |
| risk.fit | moderate | installed Pi 1.0.4 and pinned 0.84.4 differ; allocated session paths need not exist yet; native forks preserve copied entry IDs and expose parentSession, unlike ws fork process roles |
| risk.test | high | deterministic races must cover paused configuration, late responses, load versus live acceptance, queued appends, malformed tails, deletion, and fork source availability; current session tests use fake contexts without SessionManager |
| risk.security_or_contract | high | new durable summaries and directory-scoped deletion require ownership safeguards; foreign-session/version records and copied-row eligibility must not leak or destroy other data |

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
exclusion. Also cover initial-save backfill, authoritative empty/partial child
files, missing native provenance, accepted-but-unwritten fork state, orderly
queue draining, deleted conversations, foreign-file write suppression, complete
unterminated records, symlink exclusion, restoration/live-acceptance races,
paused configuration and late provider responses, retained-history eligibility,
and exclusion of the Previous conversation block. Preserve the completed
feature's delayed-flush behavior. Configured promotion reviews remain required
before this idea becomes an execution target.
