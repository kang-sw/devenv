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
- **Alive-only nudge production (748).** Emit timed nudges only while the
  owning lead is alive. The scheduler must not enqueue to process-stopped or
  closed hosted leads: ordinary mailbox delivery can restart a stopped open
  conversation. Preserve deadlines/checkpoints while not alive and catch up
  when alive again. Unknown process/snapshot evidence suppresses production;
  a key, board, mail watcher or fresh heartbeat alone does not prove life.
- **Folder-copy source cessation (749).** Apply the existing source-cessation
  precondition to board and nudge-scheduler writing before a folder-copy
  takeover. The destination inherits deadlines, ignores copied notification
  receipts scoped to the source reply-id, and sends one catch-up when alive.
  This is not a new transfer command, lease or code-enforced authority switch.
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

Owner answer relayed by MAIN at 2026-11-14 13:05:55, verbatim:

> 748=a^"중지"를 말하나요, "닫기"를 말하나요? (아니면 추가할 "재움"?) 근데 생가해보니까 그냥 딱 살아있을 때에만 재촉 타이머가 도는 게 맞아 보이긴 하네요.

748 confirms alive-only nudge production, not a new sleep state. MAIN confirmed
at 13:09 that the scheduler must not enqueue to stopped or closed hosted leads;
the existing transport does not suppress their wake. MAIN accepted the cited
process-evidence gate: for hosted leads, require current owning open runner/row,
HostStopped false, positive associated harness HostPID and a live process; for
terminal Pi/Claude/Codex/manual leads, require valid own-key/session/root
association and a positive recorded HarnessPID with live-process evidence.
Unknown, missing or contradictory snapshot/PID evidence suppresses nudges.
Heartbeat-only inference is excluded. Existing PID checks do not authenticate
PID reuse, and process exit can race enqueue; document/test those limitations,
not a stronger delivery-time guarantee. Preserve deadlines/checkpoints while
not alive and catch up when alive again.

Owner answer relayed by MAIN at 2026-11-14 13:15:09, verbatim:

> 749=a

749 confirms source cessation first for folder-copy takeover, applying the
existing playbook obligation to the nudge scheduler. The destination inherits
deadlines, ignores copied source-reply-id receipts, and sends one catch-up when
alive. No new transfer command, lease or code-enforced switch is authorized.
The stopped/closed-lead and copied-source policy holds are answered; exact
command/schema and scheduler mechanisms remain technical preparation.

Owner idea, approximately 2026-11-14 11:30Z, referring to stalled-triage text
`서버가 자동으로 판정하게`:

> 이거 관련해서는 그냥 아이디어인데, 토픽에도 티켓 비슷한 상태를 추가해서, (문법 비용 측면에서 연관 티켓 슬롯을 그냥 없애버려도 된다고 생각해요, 사용자 입장에서는 봐도 거의 노이즈에 가까워서 ...) 명시적으로 상태를 지정하게 하고, 상태 전환을 로그에 기록하고(e.g. 작업 중) 예를 들어 "논의 중"인데 미결 항목이 없는 토픽에 대해 경고 nudge,(새 턴 프롬프트 주입?) // "구현 대기" 상태 = 만약 다른 구현 중 토픽 없는데 보류 중 항목 있으면 whiteboard 업데이트 시점 넛지 (어차피 보류 중인 건 사용자도 알 수 있으니까)  //  "구현 중" 상태 + 구현 상태 전환 시 예상 소요 시간 필수 -> 예상 소요시간 넘어간 태스크마다 새 턴 주입(이 때 예상 소요시간 넘어간 태스크 리스트 전체 열거) + 30분 디바운스 + 전역 1시간 태스크 리스트 반복 타이머.

Earlier approved owner note, `685=a`:

> 미결 질문이 없는 토픽은 구현되고 있는 상태여야 한다 ... 구현이 되지 않은 상태이거나 명시적으로 보류된 티켓들은 반드시 뭐가 됐든 미결 질문을 하나는 띄워서 사용자 신호를 받을 수 있는 상태여야 한다

MAIN's approved three-phase clarification supplies the behavioral contract
below. The quoted idea's parenthetical mechanism suggestion does not settle
new injection machinery; Phase 3 explicitly reuses mailbox push.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/skill-authoring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin-wsflow/rsrc/`, `agents-plugin-wsflow/skills/`, `agents-plugin-tool/internal/wsdoc/conventions/`)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin/hooks/`, `agents-plugin-wsflow/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)
- Keep this ticket in `todo/`. Capture runs no fact-populator, Sage review,
  promotion, independent implementation review, runtime activation, or source
  changes. Readiness and implementation belong to the lead's later steps.
- Public grammar, exact enum strings, ETA/owner encoding and the technical
  persistence mechanism remain unselected. Timer scope, per-topic debounce,
  restart continuity and crash delivery preference are settled by 728-731;
  alive-only production and copied-source cessation are settled by 748-749.
  Do not retain these as open policy holds or infer encoding from state names
  or illustrative log/extension text.
- Enforce the alive-only gate before nudge enqueue, not by assuming mailbox
  transport will suppress a wake. Process-stopped/closed hosted leads and
  unknown/missing/contradictory process evidence are ineligible. Preserve
  deadlines/checkpoints without advancing notification watermarks while
  ineligible; catch up when alive. Record PID-reuse and enqueue/stop race
  limits. Do not turn a dashboard heartbeat/stale-display heuristic into
  process proof or introduce a new host lifecycle API.
- Folder-copy takeover requires source board/scheduler cessation first. Keep
  destination deadline continuity, reply-id-scoped receipt handling and one
  catch-up; no new transfer command/lease or code-enforced switch is in scope.
  This remains distinct from existing same-key hosted/terminal handoff.
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

### Current static verification (2026-11-14)

All source observations above were rechecked at `4590981bf492403a217402839522a613d91bb7f9`
on `feat/topic-state-task5`. `git diff 9854ff350fa1b0eccfb5c71649c5190c405925b9 HEAD -- agents-plugin agents-plugin-tool agents-plugin-wsflow`
returned no product changes. The full external technical proposal, including its
addendum, was read as nonauthoritative preparation; its initial Text-protocol
recommendation and its now-settled timer-policy questions are not new authority.
No build, test, browser, probe, provider, or runtime verification was performed.

- **Grammar and caller authority:** `agents-plugin-tool/internal/wswhiteboard/commands.go#L22-L40`
  declares the existing topic group and command table, with no state or ETA
  command. `agents-plugin-tool/internal/mcp/server.go#L752-L757` gates board
  tools to top-level leads. `agents-plugin-tool/internal/wswhiteboard/store.go#L582-L675`
  clones, dispatches in order, and refuses the whole batch on any problem.
  Exact command names, enums, units/ranges, owner representation, repeated
  declarations, extension validation, transition timestamps, and query projection
  still need lead technical selection, not inference from the proposal's examples.
  The related refusal-cost work is not landed in this tree: its per-command
  mutation decision must not be assumed to be current Apply behavior.
- **Lifecycle and provenance:** `agents-plugin-tool/internal/wswhiteboard/store.go#L679-L720`
  withdraws open items on close and emits human-readable close events;
  `agents-plugin-tool/internal/wswhiteboard/entries.go#L62-L85` separates
  server events from accepted activity that reopens a topic. Adoption renames
  logical keys (`agents-plugin-tool/internal/wswhiteboard/topics.go#L207-L233`)
  while the folder mapping retains history identity
  (`agents-plugin-tool/internal/wswhiteboard/folders.go#L575-L590`). Whether a
  close ends an implementing run, what reopen restores, and how old-writer
  close/reopen events affect new state remain technical lifecycle choices;
  parsing the existing close Text is not already a machine-state contract.
- **Persistence candidate, not schema approval:** committed system-log bytes
  are retained by typed-prefix comparison followed by suffix-only publication
  (`agents-plugin-tool/internal/wswhiteboard/folders.go#L613-L660`,
  `agents-plugin-tool/internal/wswhiteboard/folders.go#L790-L839`). Pending
  recovery instead carries opaque append Data through the unchanged intent
  envelope (`agents-plugin-tool/internal/wswhiteboard/folders.go#L72-L91`,
  `agents-plugin-tool/internal/wswhiteboard/folders.go#L280-L287`,
  `agents-plugin-tool/internal/wswhiteboard/folders.go#L727-L737`). First legacy
  conversion clones typed State and serializes the entire entry list
  (`agents-plugin-tool/internal/wswhiteboard/folders.go#L309-L333`,
  `agents-plugin-tool/internal/wswhiteboard/folders.go#L648-L655`): unknown
  legacy members already lost in decoding are not recovered. These are three
  distinct boundaries. New readers must also retain any selected known payload
  through `State.clone` (`agents-plugin-tool/internal/wswhiteboard/store.go#L350-L361`).
  Payload shape/versioning, fold authority, absent/malformed/unsupported-state
  handling, query shape, and durable notification bookkeeping remain unselected;
  neither optional Entry metadata nor JSON inside Text is approved here.
- **Scheduler and delivery:** no topic ETA mechanism was found by
  `rg -n '\b(ETA|TopicState|eta_minutes|overdue|awaiting-confirmation)\b' agents-plugin-tool/internal/{wswhiteboard,wsdashboard,mcp} --glob '*.go' --glob '!**/*_test.go'`
  (only unrelated tracker overdue text matched). There are daemon scheduling
  precedents, not an ETA scheduler: `agents-plugin-tool/cmd/ws-mcp/mailbox_sweep.go#L15-L39`
  runs a queue sweep; `agents-plugin-tool/internal/wsdashboard/ack_check.go#L245-L332`
  manages answer-check timers and writes its watermark before inbox send. That
  answer-check ordering can lose a nudge and is not evidence for 731's preferred
  duplicate-over-loss behavior. `agents-plugin-tool/internal/mcp/pi_tools.go#L497-L519`
  supplies the existing PostSystem-then-Wake precedent;
  `agents-plugin-tool/internal/mcp/lead_mail.go#L80-L183` watches and claims hosted
  lead mail. Folder commit and reply-registry enqueue use separate locks and
  publication paths, not one transaction. Scheduler ownership/discovery, restart
  checkpoints, send/retry ordering, full-list debounce accounting, hourly clock
  anchoring, and simultaneous close/extension revalidation still need technical
  selection. Existing folder lifetime is not proof of a live recipient
  (`ai-docs/manuals/ws-mcp.md#L57-L81`); 748-749 now settle alive-only production
  and copied-source cessation, without selecting a new scheduler/host API.
  Existing mailbox delivery is the required reuse boundary, not authorization
  for a new injection protocol.
- **Alive gate and hosted lifecycle:** source tracing distinguishes settings
  stop from close: buttons post `/stop` and `/close`
  (`agents-plugin-tool/internal/wsdashboard/assets/dashboard.js#L8217-L8235`,
  `#L8268-L8283`); Stop ends only the process
  (`internal/wsagent/runner.go#L857-L865`, `#L3038-L3050`), while an open lead's
  mail watcher survives (`#L470-L476`, `#L1525-L1538`). PostSystem/Wake can
  therefore reach `mailChanged -> kick -> startProcess` and restart it
  (`#L1361-L1389`, `#L1799-L1826`). Close ends the current lifetime and its mail
  watch, but explicit guarded hosted Reopen exists
  (`internal/wsagent/handoff.go#L231-L285`); do not equate close with process
  stop or claim current hosted close is irreversible. The separate UI-copy
  discrepancy is captured by `261114-bug-ws-hosted-close-copy-conflicts-with-reopen`.
  Hosted positive-process evidence is HostStopped false plus positive HostPID
  and ProcessAlive (`internal/wsdashboard/pageview.go#L515-L522`), with owning
  runner/row association. Terminal HarnessPID is launcher-derived
  (`agents-plugin/bin/ws-mcp-launcher.py#L1864-L1866`;
  `internal/mcp/hosted_login.go#L37-L63`) and recorded in registration
  (`internal/mcp/dashboard_sessions.go#L332-L338`). Registration.Live is only a
  heartbeat display rule (`internal/wsdashboard/registry.go#L202-L214`);
  idle heartbeat cessation and MCP-loop EndedAt do not alone prove harness
  exit (`internal/mcp/dashboard_sessions.go#L419-L475`). ProcessAlive is not
  PID-start identity authentication (`internal/wsstate/process_alive.go#L3-L15`,
  `process_alive_unix.go#L7-L10`). These observations support the approved
  alive gate and its limits, not an executed end-to-end verification.
- **Folder-copy authority:** the existing playbook requires source cessation,
  destination backup/copy, then HTML regeneration
  (`agents-plugin/rsrc/whiteboard/whiteboard.md#L25-L29`). Each caller still
  resolves its board from its own root/key-derived reply-id
  (`agents-plugin-tool/internal/mcp/whiteboard_store.go#L16-L20`), under its
  directory's folder lock (`internal/wswhiteboard/folders.go#L167-L193`). Copy
  does not revoke the source key or redirect later source writes. Thus 749
  applies an existing caller obligation to scheduler cessation; it is not
  evidence of a mechanically enforced full switch. Existing same-key handoff
  instead changes registration ownership under lock
  (`internal/wsagent/handoff.go#L13-L27`, `#L108-L132`, `#L180-L229`). Paths
  abbreviated as `internal/` in these two bullets are under `agents-plugin-tool/`.
- **Verification seams:** existing tests include `TestTopicTicketsRoundTrip`,
  `TestTopicTicketsAreQuietMetadata`, and `TestTopicTicketsSurviveTheFolderStore`
  in `agents-plugin-tool/internal/wswhiteboard/topic_tickets_test.go`;
  `TestFolderImmutableVersionsAndLog`, `TestFolderRecoveryEveryBoundary`, and
  `TestFolderRecoveryRepeatedPartialAppend` in
  `agents-plugin-tool/internal/wswhiteboard/folders_test.go#L256-L447`;
  and `TestLeadMailboxDeliversClaimedMailOnce` in
  `agents-plugin-tool/internal/mcp/lead_mail_test.go#L106-L220`.
  They provide reuse seams, not mixed-version state or overdue-delivery coverage.
  The future matrix must distinguish old text/new daemon, new command/old daemon
  refusal, old writer/new committed log, old recovery/new pending bytes, and
  legacy conversion. Controlled-clock eligibility/debounce/restart/crash fixtures
  and actual supported-host queue-to-turn checks remain unexecuted requirements.
  Phase 1 also needs playbook reconciliation at
  `agents-plugin/rsrc/whiteboard/whiteboard.md#L108-L116` and
  `agents-plugin/rsrc/whiteboard/whiteboard.md#L243-L247`, plus the linked-ticket
  UI assertions revised without weakening command/store/query assertions.

## Prior Decisions

- 261113-feat-ws-whiteboard-topic-panels-and-log (2026-11-13, commit 972766ab): "Commit exact bytes and append offsets under one folder lock; replay interrupted writes idempotently before any consumer reads authority." — bearing: constrains
- 261112-feat-ws-whiteboard-topic-tickets-sticky-head (2026-11-13, commit c69946c0): "261112-feat-ws-whiteboard-topic-tickets-sticky-head: closed" — bearing: constrains
- 261110-feat-ws-dashboard-headless-lead-sessions (2026-11-11, commit be127052): "A hosted lead has no named inbox (its spawn drops the identity), so only its reply-id queue is ever drained." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/rsrc/whiteboard/whiteboard.md, wsflow mirror, agents-plugin-tool/internal/wswhiteboard/, internal/mcp/lead_mail.go, internal/wsmailbox/replyid.go |
| scope.surface | cross-module | shared playbooks/UI, whiteboard command/query/store contracts, daemon scheduling and mailbox delivery |
| scope.new_public_symbol | unknown | optional command/field is required, but no exported Go symbol or public spelling has been selected |
| scope.new_type_contract | yes | Phase 2 optional state and Phase 3 reasoned cumulative ETA extensions require new command/data contracts; exact shape remains unselected |
| scope.test_surface | existing | agents-plugin-tool/internal/wswhiteboard/topic_tickets_test.go, folders_test.go, entries_test.go, nudge_test.go; internal/mcp/lead_mail_test.go; new scheduler fixture location unselected |
| complexity.reuse_points | confirmed | read EntrySystem/event/LatestAgent, FolderStore append/replay, Outcome.Info, PostSystem and LeadMailbox Watch/Drain |
| complexity.side_effect_risk | high | durable state and timers enqueue unsolicited lead turns across restart and concurrent topic changes |
| risk.correctness | high | transition/lifecycle authority, mixed-writer preservation, debounce accounting and crash-ordering implementation remain unselected; alive-only/source-cessation policies are settled |
| risk.fit | high | state must fit immutable history and existing caller/mailbox boundaries without adopting a Text protocol or competing authority implicitly |
| risk.test | high | existing fixtures do not establish new-state skew preservation or controlled-time queue-to-turn behavior across hosts |
| risk.security_or_contract | high | new public grammar and optional persistent state need T0 proof; mailbox delivery must retain lead identity and existing authority boundaries |

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
728-731 and 748-749 do not add timer machinery or state syntax to this slice.

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
leads' boards do not share this clock or list. Gate both overdue and hourly
nudge production on the owning lead being alive (748), using the approved
process-evidence boundary above. In particular, do not enqueue to a stopped
or closed hosted lead: existing ordinary mail can restart a stopped open
conversation. Unknown/missing/contradictory process evidence suppresses enqueue;
heartbeat/key/board presence alone is insufficient. Preserve deadlines and
notification checkpoints while not alive, then send one catch-up when alive.
Document PID-reuse and enqueue/stop race limits rather than promise that
transport enforces delivery-time liveness.

For folder-copy takeover, require source board/scheduler cessation before the
copy under the existing playbook obligation (749). The destination inherits
deadlines, ignores copied receipts belonging to the source reply-id, and sends
one catch-up when alive. Do not add a transfer command/lease or code-enforced
switch; do not confuse this copy with same-key hosted/terminal handoff.

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
that tolerates duplicates rather than silently losing a nudge. Verify no
scheduler enqueue for stopped/closed hosted leads, dead terminal harnesses,
or unknown/missing/contradictory process evidence; live idle processes remain
eligible without relying on heartbeat freshness. Cover automatic idle-stop,
resume/reopen catch-up, expired-key non-resurrection, PID-reuse limitations and
enqueue/stop races. Exercise source cessation, destination deadline continuity,
source-reply-id receipt exclusion and one live-destination catch-up after folder
copy, separately from same-key handoff. Exercise actual mailbox-to-lead delivery,
not only response text. The policy holds, including 748-749, are answered;
the persistence/scheduler mechanism and exact public command/field forms still
require technical fact/design work before readiness. This capture implements
no scheduler or delivery protocol.

## Remaining Technical Preparation

- Settle public state/extension command grammar, exact enum and ETA/owner
  encoding, transition timing and validation, and closed/reopened-topic
  lifecycle behavior. These are technical contract choices, not reopened
  timer-scope, restart, alive-only or copied-source policy questions.
- Establish the eventual optional-state authority and field shape through
  fact/design work. Compare existing immutable system-Entry metadata with
  alternative preservation-safe encodings; retain human-readable history.
  Verify absent-state compatibility and actual old/new writer round-trips,
  including pending recovery and the distinct legacy-conversion boundary.
  Optional metadata is not automatically T0 merely because it is optional.
- Design scheduler ownership, durable deadline/last-nudge bookkeeping and
  mailbox enqueue/retry ordering to implement 728-731 and 748-749. Keep the
  approved alive gate and source-cessation obligation without selecting the
  formed proposal's command/Entry/receipt APIs by implication. Existing folder and
  reply-registry transactions are separate (`folders.go:138-193`;
  `internal/wsmailbox/replyid.go:171-234`); source reuse alone does not establish
  crash-safe delivery or a running ETA scheduler. Use the accepted duplicate
  preference rather than seeking a new exactly-once policy decision.
- Specify and verify per-topic eligibility/bookkeeping when a full-list nudge
  includes other overdue topics, response-only structural warnings, cumulative
  extensions, restart/live-resume/copy catch-up, and actual existing mailbox
  push/wake delivery across supported lead hosts. Verify approved process gates
  and document PID-reuse/enqueue-stop races; ended-lead and copied-source policy
  holds are not open. These are implementation/verification gaps,
  not evidence of new runtime enforcement or authorization for a new protocol.

The formed technical proposal remains preparation, not a Decision selecting
Text encoding, an Entry payload schema, command syntax or a new state authority.
All three phases remain in this ticket; 697's parallel-worktree permission and
the existing integration order remain unchanged.
