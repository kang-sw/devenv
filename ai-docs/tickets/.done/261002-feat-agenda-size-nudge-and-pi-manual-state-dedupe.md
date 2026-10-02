---
title: "Agenda size nudge, and Pi session state only through deduped live workflow_manual calls"
related:
  260625-feat-ws-session-state-machine: defined agenda as session-level mode context reminded at workflow-manual load; this ticket adds the size nudge on that surface
  260904-feat-ws-pi-lead-bootstrap-system-prompt: defined the system-prompt manual snapshot and the workflow_manual -> session-state mapping; this ticket narrows the snapshot to the static part
  260908-feat-ws-pi-playbook-read-dedupe-in-window: the stateless active-context dedupe contract this ticket reuses for workflow_manual
sage-review-design: skipped
sage-review-completeness: completed
sage-review-completeness-reviewed: 43e24c47ebd03846
completed: 2026-10-02
---

# Agenda size nudge, and Pi session state only through deduped live workflow_manual calls

## Background

A long-running Pi lead session (GPT) used `agenda.set` heavily as a scratchpad
and never cleared it; the rendered agenda reached roughly 3000 lines. Nothing
prompts cleanup: no shipped playbook mentions agenda, the only skill mention
is lead-revive's description ("restore agenda/todo state",
`agents-plugin/skills/lead-revive/SKILL.md#L3`), which says nothing about
cleanup, no code path calls `agenda.clear`, and `renderSessionState` pretty-prints every blob in
full.

On Pi the cost is multiplied. The lead's system prompt carries the full
`workflow_manual` response captured at `session_start` (`manualSnapshotRef`),
including everything from `## Session Key` onward (Session State with every
agenda blob, then Notes). The system prompt is not compacted, and that copy
goes stale after the first state mutation. When the model later calls
`workflow_manual` (entry skills and lead-revive tell it to), the bridge strips
the static body but returns the state tail again, so the same agenda is present
twice and compaction cannot reclaim the system-prompt copy.

Measured on this machine's 72 stored session records (compact JSON bytes): the
typed resolver blobs alone reach `implement` max 1582 B (median 1199) and
`proceed` max 976 B (median 850); sessions holding only those two sit at about
2.1-2.5 KB.

## Decisions

### D1. Agenda size nudge in ws-mcp, every time over the threshold

- When the session's total agenda size is at or above **4096 bytes**, append a
  cleanup note to (a) the `## Session State` render (`renderSessionState`, so
  `workflow_manual`, `workflow_state`, and lead-revive all carry it) and (b)
  the `agenda.set` response.
- The note is emitted on every render or set while the total stays at or above
  the threshold; no per-session "already nudged" state.
- Typed resolver blobs (`implement`, `proceed`) count toward the total like any
  other blob.
- Applies to every harness (Claude, Codex, Pi); it lives in ws-mcp.
- Size measure: the sum of stored blob value byte lengths (key names
  excluded), not the pretty-printed render (the threshold is calibrated on
  stored bytes; render size varies with indentation). `agenda.set` computes
  the total after the new blob is stored.
- Note literal:

  ```text
  note: agenda totals <N> bytes (threshold 4096); clear stale blobs with agenda.clear (key, or all: true).
  ```

- Placement: in `## Session State` immediately after the agenda blobs (before
  `### Todos`); in the `agenda.set` response as a second line after
  `agenda set: <key>`.
- Rejected: nudging once per threshold crossing (needs stored state, and the
  failure mode is a model that ignores a single nudge); a 3000-byte threshold
  (resolver-only sessions reach ~2.5 KB, so it fires on sessions with nothing
  stale and trains the model to ignore it); excluding resolver keys from the
  total (hard-codes the resolver key list into nudge logic).

### D2. Pi system prompt keeps only the static manual part

- The Pi lead/fork system-prompt ws block drops the manual snapshot's tail from
  the `## Session Key` line onward (Session Key, Session State, Notes). The
  static manual body and everything ws-mcp prepends ahead of it stay.
- Notes leave the system prompt together with session state: they are equally
  stale after a `note.write`, and they then reach the model only through live
  `workflow_manual` calls, where compaction and D3 dedupe apply.
- The fixed snapshot header line (`lead-bootstrap.ts`, currently "Session-start
  snapshot of your ws workflow manual (fetched once; not refreshed
  mid-session — call ws__workflow_manual for your live current session
  state):") is replaced by this conditional nudge, worded so it does not read
  as a per-turn instruction:

  ```text
  Your session state (agenda, todos, notes) is not in this prompt. When no `workflow_manual` result is visible in your context — a fresh session, or after compaction — call `workflow_manual` before acting.
  ```

- Rejected: keeping the state tail in the snapshot (stale, uncompactable,
  duplicated by live calls); replacing the whole manual with a one-line
  `workflow_manual(<key>)` pointer (loses the compaction-surviving manual body
  that `260904-feat-ws-pi-lead-bootstrap-system-prompt` deliberately chose, and
  would push the static body through tool results on every call).

### D3. Pi workflow_manual responses dedupe against the active context

- A Pi `workflow_manual` call whose returned text is byte-identical to a prior
  `workflow_manual` result still visible in Pi's active context
  (`sessionManager.buildContextEntries()`) returns a short pointer instead of
  the full text; any byte difference returns the full text. No diff rendering.
- Reuse the stateless contract of
  `260908-feat-ws-pi-playbook-read-dedupe-in-window`
  (`playbook-read-dedupe.ts`): active-context only, fresh backend read and byte
  equality as the source of truth, no adapter cache, compaction naturally
  re-enables the full text.
- The compared text is the full text the Pi tool returns (after the mapping
  line and advisories), keyed by the resolved session key, through
  `dedupeRead` with a new read family, inheriting its pointer semantics.
- An advisory that varies between calls while session state does not makes
  the texts differ, so the full text is returned; that is the accepted
  fail-safe direction, not a defect.
- With D2 in place the only comparison target is prior visible tool results;
  there is no comparison against the system prompt.
- Rejected: rendering a diff of the state (risk of the model misreading partial
  state); comparing against the system-prompt snapshot (removed by D2).

## Constraints

- No playbook or skill text changes; the nudge lives in ws-mcp responses and
  the Pi adapter.
- D2 changes only what the system prompt carries; the existing
  `workflow_manual` mapping (`cutStaticBody` / `WORKFLOW_STATE_MAPPING_LINE`)
  keeps its behavior.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)

## Prior Decisions

- 260904-feat-ws-pi-lead-bootstrap-system-prompt (2026-09-04, Decisions): "### 1. Workflow manual + Pi lead guide live in the lead's system prompt" / bridge prepends "Workflow manual is in your system prompt; this is your current session state." — bearing: constrains
- 3a83b29c (2026-09-04, commit): "Replaces the previous 'nothing dynamic in the system prompt' posture with a one-shot session-start snapshot per the user's direction; ... answered by the snapshot marker line plus the entry-point workflow_manual cadence" — bearing: constrains
- 260906-bug-ws-pi-workflow-manual-static-body-cut-never-matches (2026-09-09, Result): "`cutStaticBody` rewritten ... to a whole-line anchor cut: start anchor = the session-start snapshot's first non-empty line (`# Workflow Manual`), end anchor = the literal `## Session Key` heading" — bearing: constrains
- 260908-feat-ws-pi-playbook-read-dedupe-in-window (2026-09-08, Decisions): "Scope. `ws__playbook_read` and `ws-skill` only. ... the workflow-manual snapshot (`lead-bootstrap.ts`, already served from the system prompt) are untouched. ws-mcp is untouched (adapter-only...)" — bearing: supports
- 260908-feat-ws-pi-playbook-read-dedupe-in-window (2026-09-10, Result): "malformed, stale, ambiguous, and entirely missing pointer provenance falls back to the fresh body" — bearing: constrains
- 260702-feat-workflow-manual-state-only-view (2026-07-02, commit 73e83c5e): "workflow_manual re-dumps the ~150-line manual on every call; a lead recovering after compaction ... often only needs the Session State (todos/agenda)" — bearing: supports
- 260824-review-release-gate-policy-p1 (2026-08-30, commit bd89c44a): "No existing workflow_manual nudge is session-scoped-once (all recompute unconditionally on every call)" — bearing: supports
- 260625-feat-ws-session-state-machine (2026-06-26, Result): "`renderSessionState`. Fresh keeps the gated self-bootstrap line; continue strips it and appends a restored `## Session State` (agenda remind + todo ...)" — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/mcp/workflow_manual.go, agents-plugin-tool/internal/mcp/session_state.go, agents-plugin-pi/src/lead-bootstrap.ts, agents-plugin-pi/src/bridge.ts, agents-plugin-pi/src/playbook-read-dedupe.ts |
| scope.surface | cross-module | ws-mcp response text for workflow_manual, workflow_state, agenda.set plus Pi system prompt and bridge workflow_manual dispatch; exported SESSION_START_SNAPSHOT_MARKER and ReadFamily change |
| scope.new_public_symbol | yes | likely a new exported marker constant replacing SESSION_START_SNAPSHOT_MARKER and a workflow_manual dedupe key helper in playbook-read-dedupe.ts |
| scope.new_type_contract | yes | ReadFamily union in agents-plugin-pi/src/playbook-read-dedupe.ts#L3 widened with a workflow_manual family, and its provenance validator at L50 |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/session_state_test.go, agents-plugin-pi/test/lead-bootstrap.test.ts, agents-plugin-pi/test/bridge.test.ts, agents-plugin-pi/test/playbook-read-dedupe.test.ts |
| complexity.reuse_points | confirmed | dedupeRead in agents-plugin-pi/src/playbook-read-dedupe.ts#L92 and buildContextEntries usage at agents-plugin-pi/src/bridge.ts#L904 |
| complexity.side_effect_risk | moderate | renderSessionState output is shared by workflow_manual and workflow_state under a byte-identity invariant workflow_manual.go#L189-L197, and Pi cutStaticBody anchors on the Session Key line |
| risk.correctness | moderate | snapshot truncation at the Session Key line and dedupe byte-equality across mapping line and advisories must not drop live state |
| risk.fit | moderate | D3 extends a dedupe scope 260908 explicitly excluded the manual snapshot from; D2 narrows the 260904 snapshot design |
| risk.test | moderate | Pi dedupe tests need active-context and compaction fixtures; Go nudge tests need threshold boundary cases |
| risk.security_or_contract | moderate | changes caller-visible response text of workflow_manual, workflow_state, agenda.set on every harness |

## Phases

### Phase 1: Agenda nudge, Pi snapshot narrowing, Pi workflow_manual dedupe

Implement D1, D2, and D3.

Verification:

- Go tests: the nudge is absent below 4096 bytes and present at and above it,
  on both the `## Session State` render and the `agenda.set` response; it
  repeats across consecutive renders while over the threshold; typed resolver
  blobs (`implement`, `proceed`) count toward the total.
- Pi tests: the system-prompt ws block contains the static manual body and the
  D2 nudge line, and none of `## Session Key`, `## Session State`, the Notes
  block, or the old session-start snapshot header text.
- Pi tests: a second `workflow_manual` call with an unchanged backend response
  while the first result is visible returns the pointer; a changed response
  returns the full text; a first result no longer in the active context (e.g.
  after compaction) yields the full text.
- Existing Go and Pi suites pass.

### Result (fd7978470) - 2026-10-02

- D1 (5ffaea726): `agendaSizeNote` in `workflow_manual.go` sums stored blob
  value bytes (keys excluded) and, at or above 4096, emits the ticket's literal
  note after the agenda blobs and before `### Todos` in `renderSessionState`
  (so `workflow_manual`, `workflow_state`, and lead-revive carry it).
  `agenda.set` computes the note inside the same locked write
  (`setAgendaNote`; `setAgenda` stays as a thin wrapper) and returns it as a
  second line. Stateless: repeats every render/set while over the threshold.
- D2 (1362fbf3e): `buildWsBlock` now uses `staticManualPart`, which cuts the
  snapshot at its first whole `## Session Key` line (same end anchor as
  `cutStaticBody`). `SESSION_START_SNAPSHOT_MARKER` is replaced by
  `SESSION_STATE_POINTER_LINE` carrying the ticket's nudge text verbatim.
  `cutStaticBody` and `WORKFLOW_STATE_MAPPING_LINE` are unchanged.
- D3 (fd7978470): new `"workflow_manual"` `ReadFamily` in
  `playbook-read-dedupe.ts`, keyed by `workflowManualKey(resolved session
  key)`. The compared text joins every text item (`workflowManualResultText`)
  on both sides, so advisory items count. Prior calls' raw arguments resolve
  through `resolvedSessionKeyArg` (omitted, empty, or sentinel goes to the
  bridge's own key), which is passed to `dedupeRead` as an option.
  `dedupeWorkflowManualContent` applies on both the mapped and the verbatim
  dispatch paths. The full -> pointer -> full cadence of `dedupeRead` is
  inherited.
- Verification: `go test ./...` (agents-plugin-tool) all pass, with new
  threshold-boundary, repeat-render, resolver-blob, and `agenda.set` handler
  tests. `npm test` (agents-plugin-pi) gave 1934 pass and 2 fail. The
  `web-package` packed-install test fails identically on the base commit
  (environment). The `claude-lifecycle` TERM/KILL test is load-flaky and passed
  3/3 when run alone. New Pi tests cover the system-prompt block against the
  real manual fixture, a production-bridge pointer/changed/foreign-key/compacted
  sequence, and `SessionManager` compaction for the new family.
- Review (lite): no Critical or Important findings. Two minor findings were
  left open: the inherited pointer prose "Do not read it again" reads oddly for
  a state check, and the raw-dispatch dedupe wiring has no direct integration
  test.
