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

This ticket remains in `todo/` pending the lead's review/readiness steps. MAIN
relayed owner approval `688,689=a` on 2026-11-14; the lead has now auto-confirmed
the technical gap resolution recorded below. This settlement changes only the
ticket, not the product. Owner approval 697 permits parallel implementation in
separate worktrees, superseding the earlier serial-start restriction; integration order
remains TASK3 -> TASK4 -> TASK5 -> 675. Phase 1 may land independently and early
after queue coordination. This documentation-only assignment starts no source
implementation.

## Decisions

- **Explicit work state rather than inference alone.** Every open topic has
  an explicit state with the obligations in Phase 1. Phase 2's machine-state
  declarations are optional for legacy data/calls; the exact additive grammar
  and encoding are settled in Technical Contract below.
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
  writer-preservation verification below; this settlement verifies no new
  runtime behavior and authorizes no T2, minor/schema bump or protocol-number
  change.
- **Technical gap resolution: auto-confirmed by the lead.** Select the minimum
  command/state/receipt/scheduler contract below, including numeric global-entry
  ordering, the bounded legacy-close adapter and low-level lock discipline.
  Existing immutable system-entry bytes preserve optional metadata across old
  folder writers; enqueue-before-watermark implements the accepted preference
  for duplicates over loss without another protocol. Keep ordinary human Text.
  Reject JSON inside Text, TopicDoc/projection authority, new Entry kinds,
  sidecars, an outbox/exactly-once protocol and transfer leases: they either
  lose preservation, obscure the existing human history or add an unnecessary
  competing mechanism. This is technical gap resolution under the existing
  owner policies, not new policy approval or evidence of executed verification.

## Technical Contract

### Commands, state and validation

- Add scoped empty `wb:state` inside `wb:topic`, with `value` and optional
  `owner`, `eta_minutes`. Exact values: `discussing`, `waiting`, `implementing`,
  `awaiting-confirmation`. Add scoped empty `wb:eta` with required `minutes`
  and `reason` in Phase 3, not Phase 2. No public closed/unset state or inferred
  default. Old calls and stores without state remain valid.
- ETA is a positive decimal integer total wall-clock budget in minutes from
  the original implementing start, not a duration from extension time. Range:
  1 through `floor(MaxInt64 / time.Minute)`; reject signs, fractions, zero,
  overflow and any start/deadline outside representable UTC RFC3339Nano
  timestamp bounds. Check duration conversion, timestamp addition and
  encode/parse round-trip for start/deadline. Server time supplies timestamps;
  callers cannot set start/deadline. No pause/resume arithmetic.
- Trim `owner` and `reason`; require valid UTF-8 and no control characters.
  Owner is 1-256 bytes after trimming, describing the responsible executor,
  not an account/lease/key/mailbox destination. Reason is 1-2048 bytes. Nudges
  still target the owning lead's reply-id.
- `wb:state` and `wb:eta` must name a known open topic in the working batch.
  Existing accepted item/content activity can first create/reopen it; state
  commands alone neither create empty topics nor reopen closed ones. Preserve
  ordered, whole-batch atomic refusal until separately authorized changes land.
- Entering implementing from absent/another state requires owner and ETA,
  creates a run identified by its state entry ID and stamps start once.
  Repeating the same implementing declaration with the same owner and current
  ETA is a no-op, not a start/debounce reset. Differing owner/ETA is refused;
  ETA changes use `wb:eta`, executor replacement requires explicit exit/re-entry.
  Other state commands reject owner/ETA attributes and repeating their current
  state is a no-op. Re-entry starts a fresh run, retaining old history.
- `wb:eta` requires currently implementing, a strictly larger cumulative total
  and nonempty reason. Retain original start, owner and initial estimate;
  increment extension count and append an extension record. Same-total ETA is
  refused. Leaving implementing records actual elapsed milliseconds alongside
  original/current estimates. Preserve each extension's individual reason.
- After an accepted batch, emit response-only structural warnings listing all
  discussing topics with zero open items and waiting topics when no open
  implementing topic exists on this board. Count every `StatusOpen` item,
  including checks, as an ask. Absent state triggers neither warning. Use
  deterministic names alongside existing Outcome.Info; do not refuse those
  structures, create asks or replace the existing stale-topic heuristic.
  At-most-one-ask/final-owner-answer obligations remain lead judgment rules.

### Immutable history authority and lifecycle

- Add optional raw JSON members `topic_state` and, in Phase 3, `board_notice`
  on the existing immutable system Entry. Keep normal human-readable Text and
  existing Entry.Kind. Consume machine payloads only on system Entries with
  Imported false and ItemID zero; these are producer conventions, not
  authenticated local-file provenance. Retain raw bytes/unknown members through
  new readers and State.clone; parse separately. No TopicDoc/session metadata or
  query projection becomes competing state authority; no schema/protocol bump.
- `topic_state` v1 has `v: 1`, `op: set|extend|reset`. A set carries `value` and
  a run snapshot when entering/leaving implementing. Snapshot keys: `id`
  (start entry ID), `started_at`, `owner`, `initial_eta_minutes`, `eta_minutes`,
  `extension_count`. Extend carries the updated run snapshot and `reason`;
  leaving implementing/reset retains the completed run snapshot when known
  and carries `elapsed_ms`; a transition records `from` when known. Entry.At is
  event time. The extension entry ID is the ETA revision; before extension,
  revision equals run ID. Containing topic and numeric stream order determine
  authority, not caller-supplied names or timestamp ordering.
- Query adds optional derived topic `state: {value, entry, run?}`, with run
  snapshot plus derived `deadline_at`; entry identifies the latest state/ETA
  entry. Unknown fold omits state and adds `state_diagnostic: {entry, code}`,
  code `malformed` or `unsupported`. Print additive text metadata lines; add
  no selector and preserve existing selectors, bodies, tickets and latest-agent
  semantics. System state/receipt entries never replace LatestAgent.
- Complete existing legacy-to-folder conversion before emitting new metadata.
  Never emit it into rollback snapshots or dual-write legacy data. Legacy
  absence remains absent, without backfill. Old typed legacy conversion cannot
  recover unknown legacy members already dropped; committed folder prefixes
  and pending opaque append bytes are the distinct preservation boundaries.
- Replay all retained topic histories, including currently closed ones, sorted
  by numeric canonical board-global `eN`, not topic order, lexical ID or time.
  The global EntrySeq and board lock serialize allocation; adoption preserves
  IDs/history. Equal timestamps do not tie-break authority. Pre-cutover recap
  allocation does not establish historical chronology or machine-state epochs.
- Close ends the run, deactivates that topic's timers and clears its declared
  state; reopening restores neither state/run nor withdrawn asks. New close
  attaches reset metadata to the existing close system line. For real old
  writers, recognize only valid non-imported system entries with ItemID zero
  whose whole Text begins exactly `Topic closed: `, after an active declared
  baseline. Prefer structured reset when present. Existing accepted commands
  cannot choose raw Entry kind/provenance; log/panel/item text with this phrase
  is not a reset. Imported is an unauthenticated bool, not a signature: local
  authoritative-file edits can forge both this literal and structured state.
  Bound the adapter to inspected historical producers; do not parse arbitrary
  authored/imported summaries or silently extend the proof to other writers.
- Missing metadata is ordinary absence. Malformed known-v1 payload produces
  diagnostic/unknown current state, never an older deadline fallback; suppress
  that topic's state-based warnings/timers. An explicit valid state command
  can establish a new baseline after malformed v1. Unknown version/operation
  preserves bytes and reports unsupported state; refuse new state/ETA mutation
  until supported code handles it, while old content/item/close commands remain
  accepted. Close/reset ends activity without interpreting an unknown run;
  a valid later baseline supersedes older diagnostics. Invalid outer log JSON
  keeps the existing corruption error. Diagnostics do not fabricate asks.

### Scheduler, receipts and accounting

- One daemon-owned scheduler per cache root, independent of pages, dashboard
  enablement and MCP connections. Discover parentless lead key records in both
  existing layouts and locate existing boards by recorded root/key reply-id.
  Do not mint/touch/resurrect keys, create absent boards, run git for discovery
  or treat arbitrary folders/registry remnants as lead authority. Reconcile
  at startup and every 30 seconds; successful board updates poke precise timers
  for deadlines, debounce and hourly due times. Generation is cache invalidation,
  not authority. Missing key association suspends discovery without data loss.
- Apply the approved 748 process gate immediately before enqueue: owning open
  hosted runner/row, HostStopped false, positive associated live harness HostPID;
  or valid own terminal key/session/root association with positive live
  HarnessPID. Suppress stopped/closed/unknown/missing/contradictory evidence,
  not idle-but-live processes. Heartbeat/key/watcher presence alone is not
  proof. Preserve deadlines/checkpoints while ineligible; catch up when alive.
  Document PID-reuse and enqueue/stop races; queued mail retains existing
  transport semantics, not a new delivery-time liveness guarantee.
- `board_notice` v1 receipt keys: `v: 1`, `reply_id`, `sent_at`, `reasons`
  (array of overdue and/or hourly), optional `hourly_epoch` (activation entry
  ID), and `covered` array of `{topic_dir, run_id, eta_entry}` for all overdue
  topics included. Use stable folder identity. Append one readable system
  receipt in the first eligible topic in stable folder order, fold receipts
  across all retained histories, and ignore source-reply-id receipts after
  copy. Raw receipt data is retained, not rewritten as a metadata sidecar.
- Under the existing FolderStore transaction lock, reload/recover authority,
  revalidate close/extension/eligibility and form one message. Order:
  low-level PostSystem succeeds -> nonblocking Wake -> append receipt to the
  supplied work-State -> folder commit. Failure to enqueue writes no receipt.
  Crash/failure after enqueue but before committed receipt retries and may
  duplicate; uncertain folder intent recovers before eligibility is recomputed.
  Never copy watermark-before-send ordering. This is scheduler enqueue retry,
  not exactly-once delivery, unlimited queue retention or power-loss assurance.
- Resolve recipient identity/mailbox path and session facts before the folder
  transaction; bounded under-lock revalidation uses direct identity/registration
  reads. No sessionStore.mu, key/session mutation wrapper, nested FolderStore
  open/update, high-level session/board/mailbox API, render/refresh callback or
  delivery wait under the folder lock. PostSystem releases the reply lock on
  return; Wake only makes nonblocking channel sends. Mutate the supplied
  work-State for receipts, never reenter the store to append them.
- Overdue means open known implementing and now at/after original start plus
  current total ETA. For each run/ETA revision, trigger immediately if never
  covered, otherwise at least 30 minutes after last coverage. Any trigger sends
  the full overdue list with owner, original/current estimates, elapsed duration
  and extension count/reasons, without truncation. Stamp ALL included overdue
  topics at enqueue time, not only triggers; do not stamp non-overdue mentions.
  Extension retains start but changes revision: a future deadline waits; a still
  overdue revision triggers immediately, independently of other topics.
- Hourly eligibility is any open implementing/waiting topic. Fold numeric
  board-global entries and bounded closes to identify maximal eligible epochs;
  the zero-to-nonzero activation entry ID/time anchors the epoch. First due is
  activation + one hour, then last successful hourly receipt + one hour.
  Overdue-only sends do not move the hourly clock. Zero eligible topics ends
  the epoch; a later one starts fresh. Hourly list includes every open topic's
  declared/absent state and the full overdue section. If both timers are due,
  send one combined message/receipt and account for both.
- Restart/resume/copy recovery sends at most one combined catch-up per board
  when alive and due, then schedules forward from committed notification time;
  never replay missed intervals. Backward clock movement delays rather than
  invents negative duration; clamp displayed elapsed at zero and diagnose
  future stored timestamps. Source cessation precedes copy under 749; destination
  inherits deadlines and ignores source-reply-id receipts. Same-key native
  handoff changes transport ownership, not board/timer identity. No transfer
  lease, command or code-enforced copy switch.

## Owner Authority

Initial owner approval (historical; the ignored-attribute interpretation of
689 is superseded by 707 below):

> 688,689=a

Owner answer relayed by MAIN at 2026-11-14 12:02:04, verbatim:

> 707=a

707 limits linked-ticket removal to UI/playbook exposure and preserves the
existing attribute's write/storage/query semantics. Owner approval 697 also
permits parallel implementation in separate worktrees without changing the
integration order. Neither policy update itself selected a state encoding or
timer mechanism; the later lead technical selection is recorded above.

Owner answers relayed by MAIN at 2026-11-14 12:24:51, verbatim:

> 728,730,731=a

> 729=b

728 scopes timers and lists to the lead's own board/reply address. 729 chooses
per-topic 30-minute debounce: a newly overdue topic nudges immediately with the
full overdue list. 730 preserves deadlines and last-nudge times across daemon
restart and sends one catch-up overdue list. 731 accepts possible duplicate
nudges after a crash over missing a nudge. Those policy answers themselves did
not select grammar, field shape or a scheduler implementation; the subsequent
lead technical settlement selects those mechanics without changing the policies.

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
The stopped/closed-lead and copied-source policy holds are answered. The lead's
subsequent technical auto-confirmation selects Technical Contract above; these
owner answers themselves did not select a wire field or scheduler API.

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
- Keep this ticket in `todo/` for this documentation-only technical settlement.
  This task runs no Sage/reviewer dispatch, promotion, runtime activation or
  source changes. The lead's review/readiness steps follow this commit.
- Technical Contract records the lead's auto-confirmed grammar, enums,
  ETA/owner encoding, immutable Entry authority and scheduler mechanics.
  Policies 707/728-731/748-749 and all three phases remain unchanged. Do not
  retain answered gaps as holds or treat selected mechanisms as implemented.
  No additional owner approval is required for this technical settlement.
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
  selected optional Entry storage against mixed-version writer constraints.

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
  (manual lines 15-25). This excludes TopicDoc metadata authority; the selected
  immutable Entry approach avoids rewriting committed unknown members.
  It does not prove every optional-state design needs a migration or cannot be
  T0; executable historical-writer verification is still required.
- Follow-up source tracing at `fee98b018` (product paths unchanged from the
  develop object above) distinguishes immutable Entry records from rewritten
  TopicDoc metadata. `folders.go:402-444` loads typed entries, so an old reader
  drops unknown Entry members in memory. However, `138-158,613-629` compares
  typed before/after prefixes, both lacking those members; `648-659` serializes
  only newly appended entries at the committed byte offset. `790-839` appends
  without truncating or rebuilding committed lines. Pending append Data is
  opaque bytes (`72-76,281-287,727-737`), so old recovery also retains unknown
  members in those bytes. An optional structured payload on an existing-kind
  immutable system Entry therefore has a byte-preservation path supporting the
  selected approach, unlike an unknown TopicDoc metadata field. First legacy
  conversion is a separate boundary: `289-333,648-655` serializes the entire typed legacy Entry list and
  cannot restore unknown members already dropped during legacy decode/clone.
  This is source evidence, not an executed mixed-version test. The lead now
  selects raw optional metadata on existing immutable system Entries, retaining
  human-readable Text; JSON encoded inside Text is rejected.
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
  enforcement. This settlement neither implements it nor treats it as landed.
  Ticket discovery returned an origin timeout/cached-ownership advisory;
  local search results do not establish origin-wide absence of related work.

### Current static verification (2026-11-14)

All source observations above were rechecked at `4590981bf492403a217402839522a613d91bb7f9`
on `feat/topic-state-task5`. `git diff 9854ff350fa1b0eccfb5c71649c5190c405925b9 HEAD -- agents-plugin agents-plugin-tool agents-plugin-wsflow`
returned no product changes. Later source-only tracing confirmed the global
entry order, bounded close-producer recognition, low-level lock direction and
alive/copy boundaries below. The lead has now selected the formed contract
with those corrections and the settled owner policies; Technical Contract in
this ticket is the selected record. The earlier Text-protocol and board-wide
debounce proposals are superseded. No build, test, browser, probe, provider,
or runtime verification was performed.

- **Grammar and caller authority:** `agents-plugin-tool/internal/wswhiteboard/commands.go#L22-L40`
  declares the existing topic group and command table, with no state or ETA
  command. `agents-plugin-tool/internal/mcp/server.go#L752-L757` gates board
  tools to top-level leads. `agents-plugin-tool/internal/wswhiteboard/store.go#L582-L675`
  clones, dispatches in order, and refuses the whole batch on any problem.
  The selected additive grammar, bounds, owner representation, idempotence,
  extensions, timestamps and query projection are in Technical Contract;
  the current product does not implement them.
  The related refusal-cost work is not landed in this tree: its per-command
  mutation decision must not be assumed to be current Apply behavior.
- **Lifecycle and provenance:** `agents-plugin-tool/internal/wswhiteboard/store.go#L679-L720`
  withdraws open items on close and emits human-readable close events;
  `agents-plugin-tool/internal/wswhiteboard/entries.go#L62-L85` separates
  server events from accepted activity that reopens a topic. Adoption renames
  logical keys (`agents-plugin-tool/internal/wswhiteboard/topics.go#L207-L233`)
  while the folder mapping retains history identity
  (`agents-plugin-tool/internal/wswhiteboard/folders.go#L575-L590`). The selected
  new lifecycle ends runs on close and does not restore state on reopen.
  The old-close literal is a bounded compatibility adapter selected here,
  not a previously existing general Text machine protocol. Imported is only
  an unauthenticated bool (`entries.go#L25-L39`); accepted log/panel/item paths
  cannot forge an actual close producer record (`entries.go#L133-L155`;
  `store.go#L714-L719`, `#L865`, `#L916-L919`, `#L1074-L1149`).
- **Selected persistence, source proof still needs execution:** committed system-log bytes
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
  Raw state/receipt payloads, fold authority, version diagnostics and query
  shape are now selected above; this source evidence does not count as tested
  preservation. Numeric global allocation comes from `entries.go#L62-L67`,
  with durable EntrySeq and validation under one board lock
  (`store.go#L250-L252`; `folders.go#L138-L193`, `#L557-L573`, `#L630-L642`).
  Read validates global uniqueness (`folders.go#L402-L425`); replay must merge
  all histories numerically, including closed topics, rather than rely on time.
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
  publication paths, not one transaction. The selected scheduler uses existing
  key/board discovery, persisted Entry receipts and enqueue-before-watermark
  retry, with the accounting/epoch rules above; no implementation is present.
  Under-lock code must stay low-level: PostSystem's closure only appends queue
  data (`internal/wsmailbox/replyid.go#L150-L234`) and Wake only broadcasts
  nonblocking channels (`internal/wswake/wake.go#L59-L76`). Ordinary session
  mutation takes sessionStore.mu before the board lock
  (`internal/mcp/session_state.go#L613-L651`), so reversing that order or nesting
  a board open/update inside the scheduler callback is excluded. Existing folder
  lifetime is not proof of a live recipient (`ai-docs/manuals/ws-mcp.md#L57-L81`); 748-749 now settle alive-only production
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
| scope.new_public_symbol | yes | additive wb:state and wb:eta commands, optional raw topic_state/board_notice Entry metadata and derived query state are selected |
| scope.new_type_contract | yes | Technical Contract defines exact state/extension grammar, raw v1 state/receipt payloads, lifecycle and diagnostics |
| scope.test_surface | existing | agents-plugin-tool/internal/wswhiteboard/topic_tickets_test.go, folders_test.go, entries_test.go, nudge_test.go; internal/mcp/lead_mail_test.go; extend these seams and add controlled-clock scheduler fixtures |
| complexity.reuse_points | confirmed | read EntrySystem/event/LatestAgent, FolderStore append/replay, Outcome.Info, PostSystem and LeadMailbox Watch/Drain |
| complexity.side_effect_risk | high | durable state and timers enqueue unsolicited lead turns across restart and concurrent topic changes |
| risk.correctness | high | selected lifecycle, mixed-writer preservation, all-topic receipt accounting, crash ordering and alive gates require implementation/fault verification |
| risk.fit | high | selected immutable raw Entry history must retain the existing caller/mailbox boundaries without competing authority or protocol |
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

After Phase 1, implement the selected `wb:state` grammar, four enums,
ETA/owner validation, idempotence, known-open-topic rules, close/reopen lifecycle
and derived query projection in Technical Contract. Complete folder cutover
before emitting raw optional `topic_state` v1 on existing immutable system
Entries. Automatically append a human-readable system line for each actual
transition, including estimate versus actual on leaving implementing; system
history never replaces LatestAgent. Fold numeric board-global entry order over
all retained histories, including closed topics, with the selected bounded
old-close adapter and absence/version/diagnostic rules. Do not introduce a
new Entry kind, TopicDoc authority or Text-encoded JSON protocol.

Return selected response-only structural warnings in `whiteboard.update` for
discussing with zero open items and waiting with no open implementing topic.
Do not refuse those structures or synthesize asks. Preserve the existing
stale-topic response heuristic. Phase 3 alone adds `wb:eta`, notification
receipts, scheduler clocks and pushed delivery.

Verification expectations: exercise exact grammar and duration/timestamp bounds,
owner limits, atomic refusal, identical-declaration no-op, absent legacy state,
close/reopen/re-entry and adoption. Verify numeric e9/e10 and equal-time order,
closed-history epoch evidence, malformed/unsupported payloads and raw-member
retention through clone, pending recovery and actual historical folder writers.
Separately cover legacy conversion and unchanged rollback snapshots. Test
prefix-looking authored log/panel/item/answer content against the bounded
close adapter and document the unauthenticated local-file boundary. Verify
query additions preserve existing selectors/ticket output and system events
never replace LatestAgent. Cover both structural warnings and non-triggering
counterparts; runtime work remains deferred until execution is authorized.

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
(731); exactly-once delivery is not required. Implement the selected `wb:eta`
grammar and cumulative elapsed ETA: two extensions from 40 to 90 minutes retain
original start/estimate, both reasons and the extension count, never silently
resetting the estimate. Use the selected daemon discovery/30-second recovery
scan/precise timers, raw immutable `board_notice` receipts, supplied-State
low-level enqueue-before-watermark sequence, all-included-topic accounting,
independent hourly epochs and combined one-catch-up behavior. No high-level
session/board/mailbox call or sessionStore.mu acquisition under the folder lock.

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
not only response text. Add concurrent normal-update/scheduler/drain/sweep
fixtures to check the selected lock direction, no nested folder acquisition,
mailbox timeout, enqueue failure, post-enqueue crash, receipt commit failure
and wake-before-commit. Test full-list receipt stamps for all included topics,
per-run/revision debounce, independent hourly anchoring and a combined message
when both clocks are due. Verify all existing supported delivery paths without
claiming automatic idle-turn injection on hosts/configurations that lack it.
Policy and technical selections are complete; implementation and executable
verification remain pending. This ticket update activates no scheduler.

## Readiness and Verification Boundary

The lead auto-confirmed the technical gaps above after reading the complete
formed contract and its order/provenance/lock corrections. No unresolved owner
policy or public-contract choice remains in this record. The ticket stays in
`todo/` for the lead's review/readiness steps; this update dispatches no reviewer,
promotes nothing and implements no product behavior. All three phases, 697's
parallel-worktree permission and the integration order remain unchanged.

The remaining work is implementation and evidence, not another schema-choice
hold. Preserve this minimum future matrix:

| Boundary | Required evidence |
|---|---|
| Old text/calls on new daemon | Whole-batch semantics and absent state remain valid; 707 ticket validation/replacement/clear/store/query effects unchanged while UI/playbook exposure disappears |
| New commands on historical daemon | Unknown commands refuse without mutation; Phase 1 text invokes no unlanded syntax |
| Actual historical schema-2 writer | New committed log prefixes survive content/item/render/adoption/close/reopen updates byte-for-byte; new-reader fold retains structured state/receipts |
| Historical recovery of new pending intent | Exact opaque append bytes, including partial suffix, complete once; metadata and checkpoints survive |
| Legacy conversion | New state emitted only after cutover; rollback snapshot unchanged; no false unknown-legacy-member preservation claim |
| New reader and lifecycle | Raw clone retention, diagnostics, bounded producer recognition, numeric/equal-time order and closed histories; no stale deadline restoration or LatestAgent replacement |
| Scheduler time/fault/concurrency | Per-topic/revision/full-list accounting, hourly epochs, alive gate, restart/resume/copy one-catch-up, enqueue-before-receipt duplicate preference and no lock inversion |
| Hosted Claude and Pi | Actual queue/Wake/watch/turn path while alive, busy/idle/rejoin; no enqueue to stopped/closed hosts; existing explicit reopen/handoff ownership respected |
| Terminal Pi and Claude mod | Actual waiter/recv/push path, configured direct/notice/off behavior and PID/association gate, without silently changing config |
| Codex/manual/no active push | Existing queue/recv/manual or already-armed wait only; unknown process evidence suppresses timers and unsupported autonomous idle injection is not claimed |
| Shared text/package | Phase 1 scenarios, canonical resource/manifest/wsflow rendering and package guards; no repository-only authority leaks into shipped text |

Existing seams are source observations, not new-feature passes. Run the
project's relevant suites during implementation and retain actual historical
writer fixtures rather than substituting new types in a mock. PID reuse,
enqueue/stop races, transport queue retention and local-file forgery are explicit
limits, not solved by the new metadata. Optional store metadata remains T0 only
with demonstrated preservation; the selected contract authorizes no T2 or new
cross-version protocol.
