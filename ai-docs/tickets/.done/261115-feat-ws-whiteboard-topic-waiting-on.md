---
title: Whiteboard topic waiting-on dependencies with unblocked surfacing and reverse view
related:
  261115-feat-ws-whiteboard-topic-state-badge: coordinate; landed in develop 02a2e5722, owns the header badge and toolbar state filter this ticket extends without changing lifecycle values
  261114-feat-whiteboard-center-reference-navigation: coordinate; owns closed-topic reveal, which this ticket's topic chips leave to it
  261115-feat-ws-revive-topic-state-summaries: coordinate; revive summaries read folded topic state and should list waiting targets once this lands
sage-review-design: completed
sage-review-completeness: completed
sage-review-completeness-reviewed: 7509b4987c412d90
sage-review-design-reviewed: 7509b4987c412d90
completed: 2026-11-15
---

# Whiteboard topic waiting-on dependencies with unblocked surfacing and reverse view

## Background

Owner request (relayed by MAIN, 2026-11-15): "각 토픽 사이에 의존성을 거는
기능… 진행 중, 대기 중인 항목들이 대체 뭘 기다리고 있는건지 파악하고
싶은데" (a feature that puts dependencies between topics; the owner wants to
see what in-progress and waiting items are actually waiting for). Owner chose
option **(901)=a**.

MAIN's contract for (a):
- an optional `on` attribute on `<wb:state value="waiting" .../>` holding a
  space-separated list of `topic:<name>`, `ticket:<stem>`, `owner`,
  `exec:<N>` (consider making exec a free label);
- declarative: re-declaring `wb:state` replaces the whole list; no add/remove
  commands;
- rendered as chips on the topic;
- when a topic target closes or a ticket target reaches `.done`, the chip
  shows ✓, the topic surfaces as "진행 가능" (unblocked), and a lead nudge
  fires, reusing the existing overdue-nudge path;
- reverse view on the blocker topic ("이 토픽을 기다리는 곳 N");
- `owner` may be derived automatically when the topic has open policy/check
  items;
- T0: optional fields only, existing lines byte-compatible;
- coordinate with the topic-state badge (just merged) and topic_default_state;
- acceptance is the owner's visual check (UI-fast principle).

Today (develop `02a2e5722`, paths under `agents-plugin-tool/internal/`):
- `wb:state` grammar is `wswhiteboard/commands.go` ~22-40 (`value`, `owner`,
  `eta_minutes`); unknown attributes are refused atomically
  (`wsxmlcmd/wsxmlcmd.go` ~802-824). Values are `discussing`, `waiting`,
  `implementing`, `awaiting-confirmation`; only `implementing` takes owner
  and ETA (`wswhiteboard/topic_state.go` ~18-26, ~255-280).
- Topic state is folded from immutable system entries carrying
  `Entry.TopicState` raw JSON (`entries.go` ~26-43, `topic_state.go`
  ~57-130). A same-value declaration is a no-op only when valid; an
  implementing repeat with different owner/ETA is refused
  (`agents-plugin-tool/internal/wswhiteboard/topic_state.go#L290-L295`); the record
  validator refuses `set` with `from == value` and turns unknown ops into
  unsupported diagnostics (~188-196, ~242-249). `Entry` round-trips unknown
  members (`entry_json.go`); `State`/`TopicDoc` are typed structs.
- Badge: `assets/page.tmpl` ~42-48 renders `.topic-state > .stb` with
  `data-state`; `assets/ago.js` ~52-63 rebuilds `.topic-state` children when
  `[state, owner]` changes; toolbar state filter reads `data-state`
  (`wsdashboard/assets/cards.js` ~290-310).
- Nudges: daemon-owned scheduler `mcp/whiteboard_nudges.go` (30 s reconcile,
  pokes on board updates) -> `FolderStore.Notify` (`board_notice.go`
  ~230-283) -> `wsmailbox.PostSystem` to the owning lead. Receipt parsing
  accepts only `hourly`/`overdue` reasons (`board_notice.go` ~69-90).
- Closure: `wb:close` sets `ClosedTopics` (`store.go` ~708-750); render knows
  closure via `topicClosed()`; topic names resolve board-locally
  (`topics.go` ~53-65).
- Tickets: whiteboard has a host-supplied `TicketResolver`
  (`view.go` ~249-255); production resolves via `wsdoc.TicketStatus`
  (up to five `os.Stat` calls for a valid stem, stopping at the first matching
  file; `.done` -> `done`;
  `agents-plugin-tool/internal/wsdoc/tickets.go#L202-L221`).
- Open asks: `AskCount()` = open policy + open checks per topic
  (`view.go` ~357-390); `OpenAskCount()` board-wide (`store.go` ~160-174).
- No `topic_default_state` setting or contract exists at `02a2e5722`; the
  topic-state view tests forbid inventing a state for legacy topics.

## Decisions

- **(901)=a and MAIN's contract** as quoted in Background. Technical calls
  below are EXEC4's (MAIN delegated them, 2026-11-15).
- **Grammar.** `on` is an optional attribute of `wb:state`, valid only with
  `value="waiting"` (refused with any other value, as `owner`/`eta_minutes`
  are refused outside `implementing`). Its value is a space-separated list of
  targets: `topic:<name>` (resolved with the existing board-local topic
  resolution; an unknown topic or the declaring topic itself is refused; an
  already closed topic is accepted and shows resolved at once),
  `ticket:<stem>` (syntax `YYMMDD-category-name`; existence is not checked at
  apply, since the store has no ticket resolver - a missing stem renders as
  an unknown chip), `owner`, and `exec:<label>` where the label is free text
  matching `[A-Za-z0-9._-]{1,32}` (so `exec:3` still works). Duplicates are
  dropped keeping first occurrence; declared order is kept; at most 8
  targets; any invalid token refuses the whole transaction. Rejected: numeric
  `exec:<N>` bound to a session registry - no board-local executor identity
  exists (mailbox peers and hosted labels are not numbered), so binding would
  invent a registry for a label; add/remove commands - the contract is
  declarative.
- **Declaration semantics.** A `waiting` declaration's list is the whole
  list: with `on` it replaces the list, without `on` it clears it. A
  declaration whose value and normalized list both equal the current ones
  stays a no-op. Leaving `waiting` (any other value, or close) clears the
  list. Cycles between topics are allowed and simply render. Rejected:
  keeping the old list when `on` is omitted - contradicts whole-list
  declaration.
- **T0 encoding.** The list is an optional `waiting_on` member on a system
  entry: `{"v":1,"targets":[...]}` (normalized tokens). Entering `waiting`
  with `on` puts the member on the same state-set entry; a waiting->waiting
  list change appends a new system entry ("waiting on changed: ...") whose
  only machine payload is `waiting_on`, never a `TopicState` record. The fold
  takes the newest `waiting_on` while the folded state is `waiting`. Older
  daemons ignore the unknown member and show the plain system text; existing
  lines and records stay byte-identical. Rejected: a waiting->waiting `set`
  record (old validators refuse `from == value` and would turn it into an
  unsupported-state diagnostic); a new `TopicState` op (unknown ops become
  diagnostics in older readers); a field on typed `State`/`TopicDoc` (older
  writers reconstruct and drop it).
- **Target resolution.** `topic:` resolves when that topic is closed;
  reopening it un-resolves. `ticket:` resolves when `TicketStatus` returns
  `done`; `dropped` renders as a distinct dropped chip and also counts as
  resolved (nothing is left to wait for); a missing stem renders as unknown
  and does not resolve. `owner` resolves when the waiting topic has no open
  policy or check item (the existing `AskCount()` predicate; technical and
  announced items never count). `exec:` never resolves automatically; the
  agent re-declares. Rejected: treating `exec:` as resolved for readiness
  (it would surface topics as unblocked on no evidence).
- **Derived owner.** A waiting topic with open policy/check items shows an
  `owner` chip even when `on` does not list `owner`, styled as derived
  (render-only, never persisted); listing `owner` explicitly and deriving it
  render one chip. Rejected: persisting the derived target (it would go stale
  as asks settle).
- **Readiness ("진행 가능").** A waiting topic is ready when its declared list
  is non-empty and every declared target is resolved and no derived owner
  chip is open. A waiting topic with no declared list never surfaces
  readiness: a derived owner chip alone just disappears when the asks
  settle, because the owner's answers already reach the agent through the
  inbox. Rejected: surfacing readiness for derived-only waits (an empty
  declaration says nothing about what unblocks the topic).
  Readiness is derived presentation only: the lifecycle value stays
  `waiting`, `data-state` is unchanged, the toolbar's waiting filter still
  includes it, and the topic gains `data-waiting-ready` plus a "진행 가능"
  chip. A target that un-resolves drops readiness live. Rejected: a fifth
  lifecycle value (changes the state contract and the filter); auto-leaving
  `waiting` (the agent owns state declarations).
- **Chips placement.** A server-rendered `.topic-deps` row follows
  `.topic-state` in the topic header, outside `.topic-state`, so the badge
  reconciler's child rebuild never deletes it; each chip names its target
  (topic chips link to that topic, ticket chips show the stem) and carries ✓
  when resolved. Topic-link navigation needs implementation: topic sections
  have `data-topic` but no topic anchor ID, and the existing in-board fragment
  handler accepts only item/panel IDs
  (`agents-plugin-tool/internal/wswhiteboard/assets/page.tmpl#L48-L48`,
  `agents-plugin-tool/internal/wsdashboard/assets/dashboard.js#L4060-L4094`).
  Add a topic anchor id on each topic section and extend that handler to
  accept it: a target hidden by the toolbar state filter resets the filter to
  all before scrolling; a closed target that is not loaded in the page
  renders as plain chip text (no link) with its ✓, leaving closed-topic
  reveal to 261114-feat-whiteboard-center-reference-navigation. No new
  client timer. Rejected: chips inside `.topic-state` (needs a reconciler
  rewrite for no gain).
- **Reverse view.** A topic that some waiting topic on the same board lists
  as `topic:` target shows a "이 토픽을 기다리는 곳 N" chip whose disclosure
  lists those topics as fragment links; closed waiters and non-waiting topics
  are not counted. Rejected: cross-board or cross-session reverse lookup
  (topic names are board-local; it needs another identity contract).
- **Unblocked nudge.** The daemon nudge scheduler evaluates readiness during
  its existing reconcile and pokes; a topic's transition to ready posts one
  lead mail through the same eligibility gate, mailbox transport and
  enqueue-then-receipt ordering as overdue nudges, naming the topic and its
  resolved targets. Dedupe key is (topic dir, id of the entry holding the
  current `waiting_on`), so a re-declaration can nudge again and a
  re-block-then-resolve under the same declaration does not. Its receipt is
  a separate optional Entry member (`unblocked_receipt`) on its own system
  entry, never a new value in the existing receipt's reason list. Payload:
  `{"v":1,"reply_id":"<owning lead reply id>","topic":"<topic dir>","declaration":"<entry id holding waiting_on>"}`.
  Like existing receipts it is reply-id-specific: a receipt whose `reply_id`
  differs from the board's current owner (a copied or adopted board) does not
  dedupe, so the new owner gets its own nudge once. A malformed or
  unknown-version receipt member is ignored, never fatal to the board or to
  other receipts. Ticket
  targets are resolved in the scheduler through an injected resolver over
  the session root's ticket tree (`wsdoc.TicketStatus`), only for boards with
  waiting topics that have ticket targets, at the existing 30 s cadence.
  Completion is worktree-local: a ticket closed in another clone counts once
  it is merged into this worktree. Rejected: adding `unblocked` to the
  existing receipt reasons (older parsers reject the whole receipt and lose
  overdue/hourly dedupe); page-refresh watches as the trigger (no page may be
  open); a ticket-tree file watcher (new machinery for a 30 s-tolerant
  signal).
- **Playbook.** Document `on`, the target kinds, whole-list replacement and
  the meaning of "진행 가능" in the `wb:state` section of
  `agents-plugin/rsrc/whiteboard/whiteboard.md`, regenerated into the wsflow
  mirror and kept in step with `agents-plugin-pi/rsrc/whiteboard/whiteboard.md`.
  Wording tolerates older daemons that refuse `on` (the agent retries without
  it).
- **topic_default_state.** No such setting or contract exists at
  `02a2e5722`; dependency lists are only ever declared, never defaulted. A
  later default-state feature must not supply `on`.
- **Compatibility tier T0.** Optional `waiting_on` and `unblocked_receipt`
  Entry members, a new optional attribute, render-only chips; no folder schema
  bump, no MCP tool or argument change. Old plugin text with a new daemon:
  `on` is simply never sent. New plugin text with an old daemon: `on` is
  refused as an unknown attribute, and the playbook tells the agent to retry
  without it.
- **Acceptance.** Owner UI-fast principle (relayed by MAIN, 2026-11-15): the
  owner's on-screen check after merge and daemon swap; no screenshot matrix
  or capture-comparison round.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/skill-authoring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin-wsflow/rsrc/`, `agents-plugin-wsflow/skills/`, `agents-plugin-tool/internal/wsdoc/conventions/`)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin/hooks/`, `agents-plugin-wsflow/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)
- No change to the four lifecycle values, the badge markup inside
  `.topic-state`, or the toolbar filter order.
- Owner run rules: offline env `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`;
  never the full `./internal/wsdashboard` package, only `-run`; never the
  shared dashboard daemon or port 29417; end with an owned process scan.

## Prior Decisions

- 261115-feat-ws-whiteboard-topic-state-badge (2026-11-15, Decisions): "Preserve ordinary legacy absence and diagnostic authority rather than inventing a state." — bearing: constrains
- 02a2e5722 (2026-11-15, commit): "Trimmed acceptance per (899)=a and the owner UI-fast principle: scoped tests, wsi18n, one lite (clean); owner on-screen check is acceptance." — bearing: supports
- 0203cff95 (2026-11-15, commit): "Add an explicit absent-to-created symlinked cache regression and retain the actual /var versus /private/var pair in precise-timer test output" — bearing: constrains
- dd03b1d0 (2026-11-15, commit): "Failure after enqueue permits duplicates rather than loss; missing boards are never created." — bearing: constrains
- 733c7522 (2026-11-15, commit): "Retain raw payload and future Entry members in memory and deep-clone bytes directly; committed folder prefixes remain suffix-only" — bearing: supports
- 05096e96 (2026-11-15, commit): "Real archived historical schema-2 source remains unchanged rather than substituting current model types." — bearing: constrains
- e7c6e32b (2026-11-15, commit): "Reuse common topic rendering for full/embed, served regions, deferred topics and ClosedWindow." — bearing: constrains
- f5b6b502 (2026-11-15, commit): "Reuse the existing wbAgo initializer/refresh/adoption hook and its document-owned timer rather than introduce another asset or mutate dashboard refresh interfaces." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | Phase 1 spans agents-plugin-tool/internal/wswhiteboard/{commands.go,entries.go,entry_json.go,topic_state.go,view.go,board_notice.go,assets/page.tmpl}, internal/mcp/whiteboard_nudges.go, internal/wsi18n/tables/, and agents-plugin/rsrc/whiteboard/whiteboard.md plus wsflow/Pi copies. |
| scope.surface | public-interface | New optional on attribute on caller-facing wb:state and optional persisted waiting_on/unblocked_receipt Entry members; current grammar: agents-plugin-tool/internal/wswhiteboard/commands.go#L22-L40. |
| scope.new_public_symbol | yes | on is a new public wb:state attribute; waiting_on and unblocked_receipt are new serialized member names. Go member/type names are not specified. |
| scope.new_type_contract | yes | Decisions define waiting_on {v:1,targets:[...]} and whole-list replacement, readiness and receipt dedupe semantics; unblocked_receipt payload {v,reply_id,topic,declaration} is defined in Decisions. |
| scope.test_surface | existing | agents-plugin-tool/internal/wswhiteboard/{topic_state_history_test.go,topic_state_badge_test.go,topic_state_view_test.go,board_notice_test.go}, internal/mcp/whiteboard_nudges_test.go, internal/wsi18n/keys_test.go and internal/wsrsrc/{wsflow_mirror_test.go,pi_mirror_test.go}; no new test filename is mandated. |
| complexity.reuse_points | confirmed | Read Entry codec/clone, TopicStates, AskCount, TicketResolver, whiteboardTickets, Notify and scheduler. Receipt parser rejects the entire receipt for any unknown reason (agents-plugin-tool/internal/wswhiteboard/board_notice.go#L69-L91); TestBoardNoticeRawCloneAndReceiptValidation passed offline on 2026-11-15. |
| complexity.side_effect_risk | high | Readiness adds durable receipts and unsolicited lead mail under the existing folder lock; enqueue-before-receipt can duplicate after failure (agents-plugin-tool/internal/wswhiteboard/board_notice.go#L230-L288). |
| risk.correctness | high | Whole-list/no-op/clear semantics, close/reopen, derived owner, external ticket changes and declaration-scoped dedupe must agree between view and scheduler; current state fold is agents-plugin-tool/internal/wswhiteboard/topic_state.go#L94-L130. |
| risk.fit | moderate | Scheduler currently has no ticket-resolver field/constructor argument, and Notify has no resolver parameter; injection is Phase 1 work, not an existing seam (agents-plugin-tool/internal/mcp/whiteboard_nudges.go#L30-L49, #L186-L205; internal/wswhiteboard/board_notice.go#L236-L236). Host-supplied TicketResolver exists for views (internal/wswhiteboard/view.go#L249-L255). Topic fragment navigation also needs support. All internal/ paths in this cell are under agents-plugin-tool/. |
| risk.test | high | Existing historical-writer test passed offline on 2026-11-15 for committed opaque future members, pending bytes and separate typed-legacy loss, but does not exercise the proposed dependency/receipt payloads, injected scheduler resolver or new chips (agents-plugin-tool/internal/wswhiteboard/topic_state_history_test.go#L171-L329). New behavior coverage remains Phase 1 work; no dashboard test package was run. |
| risk.security_or_contract | high | Static T0 premise holds for existing folder entries: unknown members are retained but not interpreted by the Entry codec, and a payload-only system entry leaves TopicStates/hourly epochs unchanged unless its text starts with the legacy close literal (agents-plugin-tool/internal/wswhiteboard/entry_json.go#L10-L56, topic_state.go#L102-L130, board_notice.go#L43-L66). Preserve provenance, reply-id isolation and byte prefixes; planned fields still need their own historical-writer proof. |

## Phases

### Phase 1: Waiting-on targets, chips, readiness, reverse view and unblocked nudge

Scope: `wb:state` `on` parsing and normalization; `waiting_on` entry member
and fold; target resolution and derived owner in the view; `.topic-deps`
row, "진행 가능" chip, `data-waiting-ready`, reverse-view chip; scheduler
readiness evaluation with injected ticket resolver, unblocked mail and
`unblocked_receipt`; playbook text in all three package copies; i18n keys in
every `wsi18n` table.

Verification:
- `wswhiteboard`: parse/refusal cases (non-waiting value, unknown topic,
  self, bad stem, bad exec label, >8 targets, duplicate drop); whole-list
  replace and clear; unchanged declaration no-op; leaving waiting clears;
  waiting->waiting change appends a system entry with `waiting_on` and no
  `TopicState` record; historical-writer byte preservation
  (`topic_state_history_test.go` pattern).
- View: chips with ✓ for closed topic and done/dropped ticket, unknown
  ticket chip, derived owner chip, readiness attribute and chip, reverse
  count excluding closed/non-waiting waiters, escaping; topic anchor
  navigation including filter reset and unloaded-closed plain text; badge reconciler
  leaves `.topic-deps` intact (`topic_state_badge_test.go` pattern); state
  filter unchanged (`-run` only).
- Nudges: transition to ready posts one mail; re-declaration nudges again;
  re-block/resolve under one declaration does not; restart keeps the
  receipt; a receipt for another reply id does not dedupe; a malformed
  receipt member is ignored; derived-only waits never nudge; old
  overdue/hourly receipts still parse
  (`board_notice_test.go`, `whiteboard_nudges_test.go` patterns).
- `wsi18n` completeness tests; wsflow mirror drift tests.
- Acceptance: owner's on-screen check after merge and swap.

### Result (3fadb5362) - 2026-11-15

Implemented the Phase 1 contract in `3fadb5362` on
`impl/exec4/waiting-on/area-drift-dwarf`. The ticket remains in `ready/` by
owner instruction; the lead owns the one lite review, integration and the
owner's post-merge on-screen acceptance.

- Added strict waiting-only `on` normalization, whole-list replacement and
  clearing, dependency-only same-state entries, raw Entry codec/clone support,
  and a shared readiness projection in `wswhiteboard/waiting_on.go`.
- Added dependency/derived-owner/readiness chips, board-local reverse
  disclosures, escaped topic anchors and filter-aware navigation. Lifecycle
  values, badge children and state-filter order remain unchanged. Unloaded
  closed targets stay plain resolved text.
- Reused daemon reconciliation and the existing folder-lock/liveness/mailbox
  path with a lazy worktree-local ticket resolver. Separate reply-id-scoped
  unblocked receipts dedupe each declaration across restart and reblocking;
  enqueue-before-receipt failure semantics still permit duplicates after failure
  rather than silently losing the nudge.
- Updated every language table, canonical guidance and product-mode guidance
  tests; regenerated the canonical manifest and wsflow mirror, then copied
  the canonical whiteboard and manifest to Pi. Guidance includes old-daemon
  retry without `on`.

Decisions:
- Preserve the existing embed budget rather than raise it. Scoped verification
  found a 247842-byte stress embed: the new styles/anchors add overhead, and
  upstream explorer client strings had exhausted the old headroom. Extend
  the existing `ScriptExcept` pattern to multiple prefixes and omit unused
  `js.files.*` strings from static whiteboard pages. The unchanged budget
  assertion passes; the dashboard still receives its complete client table.
- Child agents, independent review and fresh-reader audit were omitted under
  the owner's binding override. The lead will run one lite review on the
  implementation range. No browser, screenshot, full suite, shared daemon,
  `develop` merge, push or ticket closure was performed.

Verification (all commands run from `agents-plugin-tool/` with
`GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`; full command output read):

```text
go test ./internal/wswhiteboard -run '^(TestWaitingOn.*|TestTopicState.*|TestBoardNotice.*|TestBoardNotify.*|TestRenderPageEmbedSizeBudget|TestDashboardTopicBodyUsesWholeCardFold|TestPagesLeaveOutTheTrackerStrings)$' -count=1 -timeout=180s
PASS (12.260s): normalization/refusal/no-op/clear, resolution and owner derivation, unchanged lifecycle/hourly epoch, raw provenance/clone, historical schema-2 committed/pending byte preservation, receipt/restart/fault/mailbox ordering, full/embed/region/closed-window rendering, escaping, reverse counts, badge reconciliation and unchanged embed budget.
go test ./internal/wsdashboard -run '^(TestWaitingOnTopic(FilterReveal|FragmentBridge)|TestBoardStateFilter|TestBoardTopicRevealUsesFoldOwner|TestBoardEntryFrameBridgeStaysNarrow)$' -count=1 -v
PASS (1.175s): production JavaScript through the Node DOM harness; filter reset waits for Overview restoration, ready remains waiting, foreign/missing targets cannot navigate, existing panel/item bridge and fold owner remain intact. No browser run.
go test ./internal/wsi18n -run '^(TestEvery.*|TestOtherTables.*|TestPlaceholdersMatchEnglish|TestPluralKeysComeInPairs|TestScriptExcept.*|TestSubsetScriptHoldsOnlyThePrefixKeys)$' -count=1 -v
PASS (1.084s): key usage/completeness, all language tables/placeholders and single/multiple/empty exclusion contracts.
WSRSRC_REGEN=1 go test ./internal/wsrsrc/... -count=1 -run '^TestGenerateRealManifest$'
PASS (0.418s).
WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run '^TestRegenerateWsflowRsrcMirror$'
PASS (0.253s); canonical whiteboard and manifest then copied to Pi.
go test ./internal/wsrsrc -run '^(TestWsflowRsrcMirrorUpToDate|TestPiMirrorUpToDate|TestShippedManifestUpToDate)$' -count=1 -v
PASS (0.245s).
go test ./internal/mcp -run '^(TestWaitingOnNudge.*|TestWhiteboardNudge.*|TestWhiteboardPlaybookStreamGuidanceProductModes|TestWhiteboardPlaybookStatesPointerAndEmbedDirWithoutNamingAHost|TestWhiteboardPlaybookReadAppendsCommandSyntax)$' -count=1 -timeout=120s
PASS (5.398s): lazy injected resolver, actual local ready-to-done ticket move without a page/update, restart/reblock/redeclaration, existing alive-lead gates/timers/mailbox delivery, and ws/wsflow playbook rendering.
```

An initial new codec assertion incorrectly expected whitespace to survive
`json.Marshal`; corrected it to compare the encoder's compact payload, without
weakening clone or committed-log byte checks. The initial embed-budget failure
was fixed in implementation as described above, not by changing its threshold.
`git diff --check` passed. Ticket guardrails passed with a Sage freshness
advisory, accepted because this update only appends implementation evidence and
preserves the approved plan and Decisions; no design re-review was run.
An owned descendant-process scan under Pi PID 30409
found zero remaining test/build/daemon processes; no process was signaled and
this worker launched no daemon. No unresolved implementation finding; lead
review and owner visual acceptance remain intentionally pending.
