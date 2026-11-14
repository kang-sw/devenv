---
title: "Whiteboard: explicit topic state, execution signals, and lead nudges"
---

# Whiteboard: explicit topic state, execution signals, and lead nudges

## Background

An open topic with no outstanding question can conceal unfinished work: the
owner cannot tell whether execution is happening, waiting, or forgotten.
Make the lead declare the work's state, retain an actionable owner signal
when work is not happening, and nudge the lead when the declared structure
or execution time no longer matches the work.

This is accepted backlog, not an implementation-ready contract. MAIN relayed
owner approval `688,689=a` on 2026-11-14 and authorized documentation-only
capture now. Owner approval 697 permits parallel implementation in separate
worktrees, superseding the earlier serial-start restriction; integration order
remains TASK3 -> TASK4 -> TASK5 -> 675. Phase 1 may land independently and early
after queue coordination. This documentation-only assignment starts no source
implementation.

## Decisions

- **Explicit work state rather than inference alone.** Every open topic has
  an explicit state with the obligations in Phase 1. The state vocabulary
  below describes approved behavior, not settled public command syntax or
  persisted enum strings.
- **Execution signal.** Approval of a concrete deliverable is its execution
  signal. Owner requests to build or change something must not be downgraded
  into checks or technical items that leave the requested work unexecuted.
- **One owner-facing ask.** Anything unfinished and not being worked keeps
  an open ask, at most one per topic. Close only on the owner's final answer.
- **Linked-ticket slot removal (707 supersedes 689).** Hide the slot from the
  board UI and playbook only. Preserve the existing accepted attribute's
  validation, writes, storage and query behavior, including values written
  through older commands. Owner 707 replaces the earlier instruction to
  silently ignore the attribute; it is not merely form acceptance. Do not
  delete stored links or change read/write semantics.
- **Timer scope (728).** Timers and topic lists belong to the lead's own
  board/reply address, not other leads' boards or a machine-wide topic pool.
- **Per-topic overdue debounce (729).** Each topic has its own 30-minute
  debounce. A newly overdue topic triggers an immediate nudge containing the
  full overdue-topic list, even when another topic was nudged recently.
- **Restart continuity (730).** Preserve deadlines and last-nudge times across
  daemon restarts. Recover with one catch-up overdue list, not a replay of
  every missed timer interval.
- **Crash delivery preference (731).** A possible duplicate after a crash is
  accepted rather than a missed nudge. This does not require an exactly-once
  delivery protocol.
- **Compatibility tier: T0.** Owner 707 resolves the linked-ticket conflict by
  limiting removal to UI/playbook exposure while preserving existing
  argument/store/query behavior. MAIN's intended tier for all three phases
  remains T0: shared text/UI changes plus additive optional state and existing
  mailbox delivery. Optional-state persistence still needs the conditional
  writer-preservation check below; this capture verifies no new runtime
  behavior and authorizes no T2, minor/schema bump or protocol change.

## Owner Authority

Initial owner approval (historical; the ignored-attribute interpretation of
689 is superseded by 707 below):

> 688,689=a

Owner answer relayed by MAIN at 2026-11-14 12:02:04, verbatim:

> 707=a

707 limits linked-ticket removal to UI/playbook exposure and preserves the
existing attribute's write/storage/query semantics. Owner approval 697 also
permits parallel implementation in separate worktrees without changing the
integration order. Neither update selects a state encoding or timer mechanism.

Owner answers relayed by MAIN at 2026-11-14 12:24:51, verbatim:

> 728,730,731=a

> 729=b

728 scopes timers and lists to the lead's own board/reply address. 729 chooses
per-topic 30-minute debounce: a newly overdue topic nudges immediately with the
full overdue list. 730 preserves deadlines and last-nudge times across daemon
restart and sends one catch-up overdue list. 731 accepts possible duplicate
nudges after a crash over missing a nudge. These answers settle the Phase 3
policy holds; they do not select public grammar, field shape, Text encoding,
state authority, or a scheduler implementation.

Owner idea, approximately 2026-11-14 11:30Z, referring to stalled-triage text
`서버가 자동으로 판정하게`:

> 이거 관련해서는 그냥 아이디어인데, 토픽에도 티켓 비슷한 상태를 추가해서, (문법 비용 측면에서 연관 티켓 슬롯을 그냥 없애버려도 된다고 생각해요, 사용자 입장에서는 봐도 거의 노이즈에 가까워서 ...) 명시적으로 상태를 지정하게 하고, 상태 전환을 로그에 기록하고(e.g. 작업 중) 예를 들어 "논의 중"인데 미결 항목이 없는 토픽에 대해 경고 nudge,(새 턴 프롬프트 주입?) // "구현 대기" 상태 = 만약 다른 구현 중 토픽 없는데 보류 중 항목 있으면 whiteboard 업데이트 시점 넛지 (어차피 보류 중인 건 사용자도 알 수 있으니까)  //  "구현 중" 상태 + 구현 상태 전환 시 예상 소요 시간 필수 -> 예상 소요시간 넘어간 태스크마다 새 턴 주입(이 때 예상 소요시간 넘어간 태스크 리스트 전체 열거) + 30분 디바운스 + 전역 1시간 태스크 리스트 반복 타이머.

Earlier approved owner note, `685=a`:

> 미결 질문이 없는 토픽은 구현되고 있는 상태여야 한다 ... 구현이 되지 않은 상태이거나 명시적으로 보류된 티켓들은 반드시 뭐가 됐든 미결 질문을 하나는 띄워서 사용자 신호를 받을 수 있는 상태여야 한다

MAIN's approved three-phase clarification supplies the behavioral contract
below. The quoted idea's parenthetical mechanism suggestion does not settle
new injection machinery; Phase 3 explicitly reuses mailbox push.

## Constraints

- Keep this ticket in `todo/`. Capture runs no fact-populator, Sage review,
  promotion, independent implementation review, runtime activation, or source
  changes. Readiness and implementation belong to the lead's later steps.
- Public grammar, exact enum strings, ETA/owner encoding and the technical
  persistence mechanism remain unselected. Timer scope, per-topic debounce,
  restart continuity and crash delivery preference are settled by 728-731;
  do not retain them as open policy holds or infer encoding from state names
  or illustrative log/extension text.
- The optional runtime field must accommodate existing topics without that
  field. The playbook's obligation to declare state is distinct from making
  a new field mandatory in old calls or old stores.
- Apply `ai-docs/manuals/skill-authoring.md` to planned playbook invariants:
  scope each rule to its named failure, keep judgment in prose and
  deterministic enforcement in tooling, and avoid copying schema contracts.
- Apply `ai-docs/manuals/shipped-surface-boundary.md`: shipped guidance must
  resolve in downstream projects without this repository's tickets, manuals,
  queue names, or owner coordination artifacts.
- Apply `ai-docs/manuals/wsflow-mirroring.md`: edit canonical shared resources,
  regenerate their manifest and byte-identical wsflow mirror, and verify
  product-mode rendering and package tests rather than hand-editing mirrors.
- Apply `ai-docs/manuals/ws-mcp.md` and
  `ai-docs/manuals/release-compatibility.md` to future MCP/store changes.
  Preserve old-call validation/write/query semantics, authoritative topic
  folders and history. Linked-ticket hiding is T0 under 707; inspect the
  eventual optional-state storage against mixed-version writer constraints.

## Prior Art and Compatibility Evidence

Source inspected at retained capture base `28d857622` and, for the linked-ticket
and folder-store evidence below, independently confirmed with read-only
`git show` against develop object
`9854ff350fa1b0eccfb5c71649c5190c405925b9`. These are source observations, not
runtime tests. Line references below address that develop object.

- `agents-plugin/rsrc/whiteboard/whiteboard.md` currently documents
  `wb:topic` with `tickets?`, item classes/statuses, owner quotes, close/reopen
  behavior, and automatic system events. It allows content-only topics and
  describes a check asking approval to proceed. Phase 1 must reconcile that
  guidance with the approved execution signal, rather than leave competing
  instructions. Shared state rules belong here and in the generated wsflow
  resource, not in repository-specific downstream text.
- Under `agents-plugin-tool/internal/wswhiteboard/`, `commands.go:27`
  accepts `tickets`; `topics.go:90-110` applies it, `116-129` validates and
  deduplicates stems, and `134-142` replaces or clears the stored list.
  A nonempty link list can make a new topic persist (`topics.go:83-104`).
  Owner 707 preserves these existing command effects.
- Under that same directory, `store.go:234-238` declares persisted
  `TopicDoc.Tickets`; `query_entries.go:382-384` copies it into topic query
  output and `query.go:349-350` emits it. `view.go:630-633` derives ticket
  badges and live-work styling; `assets/page.tmpl:44,110` renders them.
  At the retained base, `topic_tickets_test.go` also pins replacement,
  clearing, validation, query and rendering behavior. The earlier conflict
  between ignored writes and established write/read effects is resolved by
  707: retain command/storage/query effects and remove only UI/playbook
  exposure. These source observations support that boundary, not removal of persisted links or
  suppression of query output.
- At the develop object above, `folders.go:199-210,370-374` decodes topic
  metadata into typed structs; `601-622` reconstructs a topic and copies its
  known document fields, and `660` serializes its metadata through
  `folderJSON` (`351-356`). An unknown field in that rewritten metadata is
  not retained by this path. The compatibility manual's T0 definition
  (`ai-docs/manuals/release-compatibility.md:62-64` at the retained base)
  expressly requires unknown-field preservation for optional store fields.
  This matters if an older writer opens and rewrites new same-schema topic
  metadata, not merely because old playbook text calls the newer daemon:
  the ordinary same-minor model has one newer daemon serving older clients
  (manual lines 15-25). Exact state storage is not settled, so this is a
  concrete persistence-path constraint to check against the eventual design,
  not proof that every optional-state design needs a migration or cannot be
  T0. No writer-preservation implementation is chosen here.
- Follow-up source tracing at `fee98b018` (product paths unchanged from the
  develop object above) distinguishes immutable Entry records from rewritten
  TopicDoc metadata. `folders.go:402-444` loads typed entries, so an old reader
  drops unknown Entry members in memory. However, `138-158,613-629` compares
  typed before/after prefixes, both lacking those members; `648-659` serializes
  only newly appended entries at the committed byte offset. `790-839` appends
  without truncating or rebuilding committed lines. Pending append Data is
  opaque bytes (`72-76,281-287,727-737`), so old recovery also retains unknown
  members in those bytes. An optional structured payload on an existing-kind
  immutable system Entry is therefore a preservation candidate, unlike an
  unknown TopicDoc metadata field. First legacy conversion is a separate
  boundary: `289-333,648-655` serializes the entire typed legacy Entry list and
  cannot restore unknown members already dropped during legacy decode/clone.
  This is source evidence, not an executed mixed-version test or a selected
  state schema/authority. MAIN prefers investigating this candidate while
  retaining human-readable Text; no Text-embedded JSON protocol is approved.
- `entries.go` already has `EntrySystem` and `event`; system entries do not
  replace `LatestAgent`. Reuse this history model for automatic state
  transitions without confusing system logs with the lead's summary.
- `nudge.go` currently implements an update-response stale-topic heuristic:
  at least five open topics, an untouched topic with no open item for an
  hour, at most hourly, listing at most five names. `Apply` in `store.go`
  puts it in `Outcome.Info`; `whiteboard_tools.go` renders that as `info:`.
  This is prior art, not the approved structural or all-overdue-topic rule.
  No replacement/removal policy for that heuristic is settled by this ticket.
- `wsmailbox.PostSystem` in `internal/wsmailbox/replyid.go` queues system mail
  to a reply ID and leaves waking waits to its caller. Hosted lead delivery
  is described in the MCP runbook under lead-session Input and implemented
  through `internal/mcp/lead_mail.go`. These are reuse points, not proof that
  a topic ETA scheduler or its required wake behavior already exists.
- `261112-feat-ws-mixed-version-compat-guards` tracks broader compatibility
  enforcement. This capture neither implements it nor treats it as landed.
  Ticket discovery returned an origin timeout/cached-ownership advisory;
  local search results do not establish origin-wide absence of related work.

## Phases

### Phase 1: Shared state and execution rules; remove the linked-ticket slot

Update the shared playbook and wsflow mirror with these obligations for every
open topic:

- **Discussing:** holds an open ask.
- **Waiting:** held items remain visible and the unfinished, unworked topic
  retains its open ask.
- **Implementing:** requires ETA and owner when entering this state.
- **Awaiting confirmation:** holds one live-confirm check.

Keep at most one open ask per topic, close only on the owner's final answer,
and treat concrete-deliverable approval as execution authority rather than
another request for permission. Preserve the distinction between a real
choice and a finished-artifact confirmation. Remove the linked-ticket slot
from the UI and playbook only, preserving existing attribute validation,
write/storage/query behavior and stored values as required by 707, which
supersedes 689's ignored-attribute instruction.

This slice may land independently after queue coordination. These are lead
obligations, not claims of existing runtime enforcement. Optional machine
state, structural warnings and timed delivery belong to later phases;
728-731 do not add timer machinery or state syntax to this slice.

Verification expectations: inspect discussion, waiting, execution and
live-confirm scenarios; confirm the build/change request is not downgraded,
the final-answer closure rule is retained, and slot hiding preserves legacy
attribute validation, replacement/clearing, persistence and query output
without deleting links/history. Check shared/wsflow rendering and
mirror/package guards. The linked-ticket compatibility question is answered
by 707; readiness remains a separate lead-owned step.

### Phase 2: Optional topic state, transition history and structural warnings

After Phase 1, introduce an optional topic-state field set through a wb
command. Automatically append a board system line on a state transition,
including estimated versus actual duration where applicable. The approved
illustration is a transition from implementing to awaiting confirmation with
an estimate of 40 minutes and actual duration of 55 minutes; it does not
prescribe a wire format or a new event type.

Return structural warnings in `whiteboard.update`'s response for discussing
with zero open asks, and waiting while no topic is implementing. These are
response warnings, distinct from Phase 3's pushed turns.

Verification expectations: cover absent optional state on existing data/calls,
state changes and automatic history, ETA/owner requirements at implementation
transition, both warning conditions and their non-triggering counterparts.
Verify preservation through folder updates and mixed-version round-trips;
keep system transitions distinct from the latest agent summary. Evaluate the
optional immutable-system-Entry metadata candidate against old append/recovery
and legacy-conversion boundaries without treating it as a selected schema.
Timed nudges and restart/delivery policies remain Phase 3 implementation scope.

### Phase 3: All-overdue-topic mailbox nudges and cumulative ETA extensions

After Phase 2, use existing mailbox push to inject a lead turn when ETA is
exceeded, listing **all** overdue topics on the lead's own board/reply address
(728). Apply a 30-minute debounce **per topic**, not per board: a newly overdue
topic triggers an immediate nudge with the full overdue list even if another
topic was nudged recently (729). Repeat the board's global topic list hourly
only while any implementing or waiting topic exists on that board; other
leads' boards do not share this clock or list.

Preserve deadlines and last-nudge times across daemon restarts, delivering
one catch-up overdue list rather than replaying missed intervals (730).
Prefer retry with a possible duplicate after a crash over losing a nudge
(731); exactly-once delivery is not required. ETA extension requires a reason
and cumulative display: for example, two extensions changing the estimate
from 40 to 90 minutes, rather than silently resetting the estimate.

Verification expectations: use controlled time to cover multiple overdue
topics in one list, independent per-topic debounce, immediate full-list
notification for a newly overdue topic during another topic's debounce,
isolation between lead boards/reply addresses, the hourly eligibility
condition, no periodic list when neither implementing nor waiting topics
remain, and reasoned cumulative extensions. Cover restart preservation of
deadlines and last-nudge times, one catch-up list, and crash/retry behavior
that tolerates duplicates rather than silently losing a nudge. Exercise actual
mailbox-to-lead delivery, not only response text. The policy holds are answered;
the persistence/scheduler mechanism and exact public command/field forms still
require technical fact/design work before readiness. This capture implements
no scheduler or delivery protocol.

## Remaining Technical Preparation

- Settle public state/extension command grammar, exact enum and ETA/owner
  encoding, transition timing and validation, and closed/reopened-topic
  lifecycle behavior. These are technical contract choices, not reopened
  timer-scope or restart-policy questions.
- Establish the eventual optional-state authority and field shape through
  fact/design work. Compare existing immutable system-Entry metadata with
  alternative preservation-safe encodings; retain human-readable history.
  Verify absent-state compatibility and actual old/new writer round-trips,
  including pending recovery and the distinct legacy-conversion boundary.
  Optional metadata is not automatically T0 merely because it is optional.
- Design scheduler ownership, durable deadline/last-nudge bookkeeping and
  mailbox enqueue/retry ordering to implement 728-731. Existing folder and
  reply-registry transactions are separate (`folders.go:138-193`;
  `internal/wsmailbox/replyid.go:171-234`); source reuse alone does not establish
  crash-safe delivery or a running ETA scheduler. Use the accepted duplicate
  preference rather than seeking a new exactly-once policy decision.
- Specify and verify per-topic eligibility/bookkeeping when a full-list nudge
  includes other overdue topics, response-only structural warnings, cumulative
  extensions, restart catch-up, and actual existing mailbox push/wake delivery
  across supported lead hosts. These are implementation/verification gaps,
  not evidence of new runtime enforcement or authorization for a new protocol.

The formed technical proposal remains preparation, not a Decision selecting
Text encoding, an Entry payload schema, command syntax or a new state authority.
All three phases remain in this ticket; 697's parallel-worktree permission and
the existing integration order remain unchanged.
