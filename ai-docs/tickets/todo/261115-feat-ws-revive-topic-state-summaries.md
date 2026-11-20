---
title: "revive 토픽 요약에 로그 서술 대신 구조화된 작업 상태 표시"
related:
  261114-feat-ws-whiteboard-topic-state-and-lead-nudges: "landed explicit state, owner and cumulative ETA authority; preserve its lifecycle"
  261114-feat-ws-topic-latest-panel-recap-summary: "separate summary-projection request; do not extend its log/panel behavior into revive injection"
---

# Revive topic summaries show structured work state instead of log prose

## Background

MAIN's 2026-11-15 02:01:51 message relays owner approval (873)=a:
revive should restore the work status of every open topic, including those
without agent logs. Current session-state rendering instead injects the
latest agent log and omits topics without one. MAIN's binding 02:02:31
addendum confirms removal of that log prose from revive summaries.

## Decisions

- Each open topic line has this approved shape:
  `stem (+ label) · state · owner/ETA remaining-or-overdue (implementing) · open ask IDs`.
  State uses the existing declared values `discussing`, `waiting`,
  `implementing`, `awaiting-confirmation`. Display open ask IDs as `(nn)`.
  Include open topics whether or not they have agent logs; exclude closed
  topics. The placeholders describe fields, not newly assigned item IDs.
- Owner-approved removal, relayed by MAIN at 02:02:31, verbatim:
  "마지막 로그 한 줄을 빼는 게 어떨까 … 노이즈일 가능성이 더 높다 … 쿼리로 쉽게 얻을 수 있는 텍스트".
  English translation: remove the last log line because it is more likely
  noise and its text is easily obtained through a query. Do not inject latest
  agent log prose. Obtain logs through existing `whiteboard.query` on demand;
  this selects no new query or API. The addendum supersedes only preservation
  of the latest agent log in the original capture brief.
- **VERIFIED:** MAIN's 2026-11-15 02:11:58 technical calls select
  `state: none` for absent state and `state: unknown (<raw value>)` for
  malformed/unsupported state. Keep those cases distinct; never guess a state
  from logs, asks or activity.
- **VERIFIED:** The same MAIN calls select open IDs of policy/check items
  only, on the same basis as `OpenAskCount` (board question count). Structural
  warnings stay separate and are not listed as open asks.
- Reuse existing structured state/run/ETA and item authority. The summary is
  output text, not a new state store, lifecycle, status inference mechanism or
  scheduler.
- Compatibility tier: **T0 output text only**. Preserve tool arguments,
  query behavior, persistence, schema and protocol. Keep session identity,
  fail-loud recovery, manual rules, agenda/todo rendering and the shared
  `workflow_manual`/`workflow_state` Session State projection intact.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`). Shipped summary text must be downstream-neutral.
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`).
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`).
- This capture changes only the todo ticket. No source, readiness posture,
  persisted store change or new route/API is selected. TASK3 stays first;
  root owns release and MAIN handoff. Local source inspection and ticket/diff guardrails
  are the only verification authorized for this lane.

## Prior Art and Evidence

The following are static observations at `40f96ea17`, not executed tests.
Paths are under `agents-plugin-tool/`:

- **VERIFIED:** `internal/mcp/workflow_manual.go:156-181`,
  `renderWhiteboardSummaries`, calls `wswhiteboard.Query(board, nil)`, skips
  `t.Closed || t.LatestAgent == nil`, then prints stem (or existing Name)
  plus `LatestAgent.Text`, resolving item references and indenting multiline
  logs. Thus topics without agent logs are currently missing.
- **VERIFIED:** `internal/wswhiteboard/query.go`, `QueryTopic`, exposes
  Name/Stem/Label, Closed, State, StateDiagnostic and Items. Query discovery
  in `query_entries.go`, `queryEntries`, enumerates topics from items and
  topic documents, not just agent entries. `makeQueryTopic` supplies declared
  state/diagnostics and label independently of LatestAgent. Discovery populates
  counts but not per-topic Items; do not mistake it for an open-ID list.
- **VERIFIED:** `internal/wswhiteboard/topic_state.go`, `TopicStates`, folds
  immutable system history in numeric entry order. `TopicRun` owns Owner,
  StartedAt, ETAMinutes and derived DeadlineAt. `stateDeadline` derives the
  deadline from original start plus total minutes. Non-implementing states
  retain no active Run. Ordinary absence and malformed/unsupported diagnostics
  are distinct from the four declared values; no default state is supplied.
- **VERIFIED:** `internal/wswhiteboard/topic_eta.go`, `applyTopicETA`, keeps
  the original run/start/owner while increasing cumulative total minutes.
  A revive summary must not reset elapsed time when an ETA is extended.
- **VERIFIED:** `internal/wswhiteboard/store.go`, Item and StatusOpen,
  hold item ID, topic, class and open status. `OpenAskCount` counts policy and
  check classes, while `topic_state.go`, `structuralWarnings`, treats every
  StatusOpen item as an ask. These were distinct existing uses at the inspection
  pin; MAIN's later 02:11:58 call selects the `OpenAskCount` basis for the new
  summary, not the structural-warning basis.
- **VERIFIED:** `internal/mcp/workflow_manual_recaps_test.go` currently pins
  latest-agent inclusion, exclusion of body/system-only topics, item-reference
  substitution and state/manual suffix identity. Those first expectations
  must change for the approved output, not the identity invariant.
- **INFERRED:** The structured authority observed above at `40f96ea17` can
  support the new summary without a schema change. MAIN's later 02:11:58 calls
  settle absence/diagnostic presentation and open-ask classes as recorded in
  Decisions; those choices are confirmed contract, not inferred mappings.

`261114-feat-ws-whiteboard-topic-state-and-lead-nudges` supplies the landed
state/ETA lifecycle. `261114-feat-ws-topic-latest-panel-recap-summary` separately
requests newer agent/panel recaps on several surfaces; this owner's later
revive-specific removal must not be silently reversed by that work. Query and
board summaries are not changed by this ticket.

## Phases

### Phase 1: Render structured open-topic work summaries during revive

Replace latest-log injection in `renderWhiteboardSummaries` with the approved
per-topic line. Include topics without logs and display implementing owner
and ETA remaining/overdue from the existing cumulative run authority, plus
open policy/check ask IDs only. Display absent state as `state: none` and
malformed/unsupported state as `state: unknown (<raw value>)`. Keep structural
warnings separate from open asks. Do not infer state from text or add a
lifecycle, store field, timer or query/API.

Future tests alongside the existing MCP renderer tests must cover all four
declared states; implementing remaining/overdue ETA before and after a
cumulative extension; stems and labels; open policy/check ask IDs, exclusion
of settled items and other classes, and separation of structural warnings;
closed-topic exclusion; and open topics with agent logs, without
logs, with body only or with system entries only. Assert latest agent log
prose is absent, including multiline/imported log text, and that topics
without logs remain included. Assert `state: none` for absent state and
`state: unknown (<raw value>)` for malformed/unsupported state, without guessing
one of the declared states. Preserve nil/empty-board behavior, heading placement
before Todos, agenda/todos and session-key/manual
rules, and byte identity between `workflow_state` and the `workflow_manual`
Session State suffix. Existing `whiteboard.query` must still retrieve logs
on demand. This capture runs no executable tests.
